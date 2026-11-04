import { useEffect, useState } from 'react'
import { FileText } from 'lucide-react'
import type { DeviceProfile } from '../data/crypto'
import { formatBytes } from '../format'
import type { Locale } from '../i18n'
import { MotionButton } from '../motion/components'
import { TextBubble } from './TextBubble'
import type { TextBubbleLabels } from './TextBubble'
import { readStoredText } from './text-storage'
import type { GuestJob } from './store'
import './Conversation.css'

type Labels = TextBubbleLabels & { unavailable: string; waiting: string; sent: string; received: string }
function TextItem({ job, profile, labels }: { job: GuestJob; profile: DeviceProfile; labels: Labels }) {
  const [content, setContent] = useState<{ value: string; sensitivity: 'NORMAL' | 'SENSITIVE'; id: string; manifest: string; profile: DeviceProfile }>()
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let alive = true
    setContent(undefined); setFailed(false)
    const stored = job.text
    if (stored) void readStoredText(profile, stored).then(value => { if (alive) setContent({ ...value, id: job.id, manifest: stored.manifest, profile }) }).catch(() => { if (alive) setFailed(true) })
    return () => { alive = false }
  }, [job.id, job.text?.manifest, profile])
  if (!job.text || !content || content.id !== job.id || content.manifest !== job.text.manifest || content.profile !== profile) return <div className={`conversation-row conversation-row--${job.direction}`}><span className="conversation-pending" role="status">{failed ? labels.unavailable : labels.waiting}</span></div>
  return <TextBubble id={job.id} value={content.value} sensitive={content.sensitivity === 'SENSITIVE'} labels={labels} direction={job.direction} />
}

export function Conversation({ jobs, profile, labels, locale, onJob }: { jobs: GuestJob[]; profile: DeviceProfile; labels: Labels; locale: Locale; onJob: (job: GuestJob) => void }) {
  return <div className="conversation-list">
    {[...jobs].sort((a, b) => a.created - b.created).map(job => job.payloadKind === 'TEXT' ? <TextItem key={job.id} job={job} profile={profile} labels={labels} /> :
      <div className={`conversation-row conversation-row--${job.direction}`} key={job.id}>
        <MotionButton className="conversation-file" onClick={() => onJob(job)}><FileText size={22} /><span><strong>{job.name}</strong><small>{formatBytes(job.plainSize, locale)}</small></span></MotionButton>
        <span className="conversation-file-status">{job.phase === 'complete' ? job.direction === 'send' ? labels.sent : labels.received : labels.waiting}</span>
      </div>)}
  </div>
}
