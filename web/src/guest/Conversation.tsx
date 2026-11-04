import { useEffect, useState } from 'react'
import { FileText } from 'lucide-react'
import type { DeviceProfile } from '../data/crypto'
import { formatBytes } from '../format'
import type { Locale } from '../i18n'
import type { TaskRecord } from '../api/taskRecords'
import { MotionButton } from '../motion/components'
import { TextBubble } from './TextBubble'
import type { TextBubbleLabels } from './TextBubble'
import { readStoredText } from './text-storage'
import type { GuestJob } from './store'
import './Conversation.css'

type Labels = TextBubbleLabels & { unavailable: string; waiting: string; sent: string; received: string; stored: string; file: string }
function TextItem({ job, profile, labels, onJob }: { job: GuestJob; profile: DeviceProfile; labels: Labels; onJob: (job: GuestJob) => void }) {
  const [content, setContent] = useState<{ value: string; sensitivity: 'NORMAL' | 'SENSITIVE'; id: string; manifest: string; profile: DeviceProfile }>()
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let alive = true
    setContent(undefined); setFailed(false)
    const stored = job.text
    if (stored) void readStoredText(profile, stored).then(value => { if (alive) setContent({ ...value, id: job.id, manifest: stored.manifest, profile }) }).catch(() => { if (alive) setFailed(true) })
    return () => { alive = false }
  }, [job.id, job.text?.manifest, profile])
  if (!job.text) return <div className={`conversation-row conversation-row--${job.direction}`}><MotionButton className="conversation-pending" onClick={() => onJob(job)}>{labels.waiting}</MotionButton></div>
  if (!content || content.id !== job.id || content.manifest !== job.text.manifest || content.profile !== profile) return <div className={`conversation-row conversation-row--${job.direction}`}><span className="conversation-pending" role="status">{failed ? labels.unavailable : labels.waiting}</span></div>
  return <div className="conversation-entry"><TextBubble id={job.id} value={content.value} sensitive={content.sensitivity === 'SENSITIVE'} labels={labels} direction={job.direction} /><div className={`conversation-row conversation-row--${job.direction}`}><button type="button" className="conversation-status-button" onClick={() => onJob(job)}>{job.phase === 'complete' ? job.direction === 'send' ? labels.sent : labels.received : job.phase === 'stopped' || job.error ? labels.unavailable : labels.waiting}</button></div></div>
}

export function Conversation({ jobs, records = [], profile, labels, locale, onJob, onRecord }: { jobs: GuestJob[]; records?: TaskRecord[]; profile: DeviceProfile; labels: Labels; locale: Locale; onJob: (job: GuestJob) => void; onRecord?: (record: TaskRecord) => void }) {
  const items = [...jobs.map(job => ({ kind: 'job' as const, at: job.created, job })), ...records.filter(record => record.state !== 'DELETED').map(record => ({ kind: 'record' as const, at: Date.parse(record.updated_at) || 0, record }))].sort((a, b) => a.at - b.at)
  return <div className="conversation-list">
    {items.map(item => {
      if (item.kind === 'record') {
        const record = item.record, direction = record.kind === 'UPLOAD' ? 'send' : 'receive'
        return <div className={`conversation-row conversation-row--${direction}`} key={'account:' + record.id}><button type="button" className="conversation-file" onClick={() => onRecord?.(record)}><FileText size={22} /><span><strong>{record.fileName || labels.file}</strong><small>{formatBytes(record.total_bytes, locale)}</small></span></button><span className="conversation-file-status">{record.deliveredByDevice ? labels.sent : record.state === 'COMPLETED' ? direction === 'receive' ? labels.received : labels.stored : ['FAILED','CANCELLED'].includes(record.state) ? labels.unavailable : labels.waiting}</span></div>
      }
      const job = item.job
      return job.payloadKind === 'TEXT' ? <TextItem key={job.id} job={job} profile={profile} labels={labels} onJob={onJob} /> : <div className={`conversation-row conversation-row--${job.direction}`} key={job.id}><button type="button" className="conversation-file" onClick={() => onJob(job)}><FileText size={22} /><span><strong>{job.name}</strong><small>{formatBytes(job.plainSize, locale)}</small></span></button><span className="conversation-file-status">{job.phase === 'complete' ? job.direction === 'send' ? labels.sent : labels.received : job.phase === 'stopped' || job.error ? labels.unavailable : labels.waiting}</span></div>
    })}
  </div>
}
