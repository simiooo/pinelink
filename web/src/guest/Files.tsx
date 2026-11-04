import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, ArrowDown, ArrowUp, Check, Clock3, Cloud, FileText, Pause } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { ApiError, controlTask, getPendingNotifications, IDENTITY, respondTransfer } from '../api/client'
import type { Transfer, Task } from '../api/client'
import { useTaskRecords } from '../api/taskRecords'
import type { TaskRecord } from '../api/taskRecords'
import { ClientRuntime, DataError } from '../data/runtime'
import { useAccountReader } from './account'
import { displayFileName, formatBytes } from '../format'
import { useI18n } from '../i18n'
import { taskError, taskStatus } from '../i18n/status'
import { MotionButton, MotionReveal, MotionSpinner, MotionTooltip } from '../motion/components'
import { Tabs, TabsList, TabsTrigger } from '../components/ui/tabs'
import { useGuestCopy } from './copy'
import type { GuestJob } from './store'
import type { FileFilter } from './navigation'

export function useAccountWorkspace() {
  const { reader, ready, owner, device } = useAccountReader()
  const records = useTaskRecords(true, ready, { runtime: reader, owner, device })
  const notifications = useQuery({ queryKey: ['notifications', owner, device], queryFn: () => reader ? reader.request<{ transfers: Transfer[] | null }>('GET', `/v1/notifications?owner_id=${encodeURIComponent(owner)}&device_id=${encodeURIComponent(device)}&state=PENDING`) : getPendingNotifications(), refetchInterval: 5000, enabled: ready })
  return { records, notifications, ready, reader, recordKey: ['task-records', owner, device, true] }
}
const taskActive = (record: TaskRecord) => ['QUEUED', 'RUNNING', 'PAUSED'].includes(record.state)
const guestActive = (job: GuestJob) => !['complete', 'stopped'].includes(job.phase)
export function guestJobStatus(job: GuestJob, c: ReturnType<typeof useGuestCopy>) {
  if(job.local?.disconnected&&!['complete','stopped'].includes(job.phase))return c.waitingConnection
  return job.phase === 'complete' ? job.direction === 'receive' && job.plain ? c.saved : c.complete : job.phase === 'paused' ? c.paused : job.phase === 'verifying' ? c.verifying : job.phase === 'stopped' ? job.error === 'FILE_EXPIRED' || job.snapshot?.state === 'EXPIRED' ? c.fileExpired : c.stopped : job.phase === 'preparing' ? c.preparing : job.error ? c.unavailable : job.transfer?.state === 'PENDING' ? c.waitingFile : c.running
}
function FileTile({ name, status, Badge, onClick }: { name: string; status: string; Badge: LucideIcon; onClick: () => void }) {
  return <MotionTooltip label={status}><MotionButton className="guest-history-file" aria-label={name + ' · ' + status} onClick={onClick}><span className="guest-history-icon"><FileText size={42} strokeWidth={1.2} /><span className="guest-file-badge" aria-hidden="true"><Badge size={11} /></span></span><span title={name}>{name}</span></MotionButton></MotionTooltip>
}
export function FilesCollection({ jobs, records, filter, onFilter, onJob, onRecord, error, loading }: { jobs: GuestJob[]; records: TaskRecord[]; filter: FileFilter; onFilter: (filter: FileFilter) => void; onJob: (job: GuestJob) => void; onRecord: (record: TaskRecord) => void; error?: boolean; loading?: boolean }) {
  const c = useGuestCopy(), { t } = useI18n()
  const guest = jobs.filter(job => filter === 'all' || filter === 'active' && guestActive(job) || filter === 'saved' && !!job.plain || filter === 'sent' && job.direction === 'send' && job.phase === 'complete' || filter === 'issues' && (job.phase === 'stopped' || !!job.error))
  const account = records.filter(record => filter === 'all' ? record.state !== 'DELETED' : filter === 'active' ? taskActive(record) : filter === 'sent' ? record.state !== 'DELETED' && record.deliveredByDevice : filter === 'issues' ? ['FAILED', 'CANCELLED', 'DELETED'].includes(record.state) : false)
  return <div className="workspace-files">
    <Tabs value={filter} onValueChange={value => onFilter(value as FileFilter)}><TabsList variant="line" className="workspace-file-tabs">{(['all', 'active', 'saved', 'sent', 'issues'] as const).map(value => <TabsTrigger key={value} value={value}><span className="workspace-tab-label">{value === 'saved' ? c.savedFilter : value === 'sent' ? c.sentFilter : c[value]}</span></TabsTrigger>)}</TabsList></Tabs>
    <div className="guest-history" aria-label={c.files}>
      {guest.map(job => { const status = (job.direction === 'send' && job.phase === 'complete' ? c.sentFilter + ' · ' : '') + (job.plain ? '' : c.once + ' · ') + guestJobStatus(job, c), Badge = job.error ? AlertCircle : job.phase === 'complete' ? Check : job.phase === 'paused' ? Pause : job.phase === 'stopped' ? Clock3 : job.direction === 'receive' ? ArrowDown : ArrowUp
        return <FileTile key={'visitor:' + job.id} name={job.name} status={status} Badge={Badge} onClick={() => onJob(job)} /> })}
      {account.map(record => { const status = (record.deliveredByDevice ? c.sentFilter + ' · ' + c.complete + ' · ' : '') + taskStatus(record, t), Badge = record.deliveredByDevice ? Check : record.state === 'COMPLETED' ? Cloud : record.state === 'PAUSED' ? Pause : ['FAILED', 'CANCELLED', 'DELETED'].includes(record.state) ? AlertCircle : Clock3, name = record.encryptedMetadata ? t('encryptedFile') : record.fileName || t(record.kind === 'DOWNLOAD' ? 'receivedFile' : 'sentFile')
        return <FileTile key={'account:' + record.id} name={name} status={c.sourceAccount + ' · ' + status} Badge={Badge} onClick={() => onRecord(record)} /> })}
    </div>
    {!guest.length && !account.length && (loading ? <div className="guest-muted" role="status" aria-label={t('historyLoading')}><MotionSpinner /></div> : !error && <p className="guest-muted" role="status">{jobs.length || records.length ? c.emptyFilter : c.empty}</p>)}
    {error && <p className="guest-inline-error" role="alert">{c.accountFilesError}</p>}
  </div>
}
export function AccountTaskDetails({ record, queryKey, reader }: { record: TaskRecord; queryKey: readonly unknown[]; reader?: ClientRuntime }) {
  const c = useGuestCopy(), { t, locale } = useI18n(), client = useQueryClient()
  const [confirm, setConfirm] = useState<'cancel' | 'delete'>(), [notice, setNotice] = useState('')
  const visible = useRef(record), alive = useRef(true); visible.current = record
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  useEffect(() => { setConfirm(undefined); setNotice('') }, [record.id])
  const mutation = useMutation({ mutationFn: ({ target, action }: { target: TaskRecord; action: 'pause' | 'resume' | 'cancel' | 'delete' }) => reader ? reader.request<Task>('POST', `/v1/tasks/${encodeURIComponent(target.id)}/${action}`, { owner_id: reader.owner, device_id: reader.profile.device, expected_version: target.version }) : controlTask(target.id, action, target.version), onSuccess: (result: Task) => {
    client.setQueryData<TaskRecord[]>(queryKey, old => old?.map(item => item.id === result.id && item.version <= result.version ? { ...item, ...result } : item))
    const committed = client.getQueryData<TaskRecord[]>(queryKey)?.find(item => item.id === result.id)
    if (alive.current && committed?.version === result.version && visible.current.id === result.id && visible.current.version <= result.version) { setConfirm(undefined); setNotice(t(result.state === 'PAUSED' ? 'pauseSuccess' : result.state === 'RUNNING' ? 'resumeSuccess' : result.state === 'DELETED' ? 'deletedSuccess' : 'cancelSuccess')) }
  }, onError: error => { if (alive.current) setNotice((error instanceof ApiError || error instanceof DataError) && error.status === 409 ? t('stateChangedRefresh') : t('actionFailed')) }, onSettled: () => Promise.all([client.invalidateQueries({ queryKey: ['task-records'] }), client.invalidateQueries({ queryKey: ['tasks'] })]) })
  const act = (action: 'pause' | 'resume' | 'cancel' | 'delete') => { if ((action === 'cancel' || action === 'delete') && confirm !== action) { setConfirm(action); return }; mutation.mutate({ target: record, action }) }
  return <div className="guest-job-detail"><div className="guest-file-row"><FileText size={40} /><span>{record.encryptedMetadata ? t('encryptedFile') : record.fileName || t('receivedFile')}</span></div><div className="guest-job-meta"><span>{taskStatus(record, t)}</span><span>{formatBytes(record.progress_bytes, locale)} / {formatBytes(record.total_bytes, locale)}</span></div>
    {taskError(record, t) && <p role="alert" className="guest-muted">{taskError(record, t)}</p>}{notice && <MotionReveal role="status" identity={notice}>{notice}</MotionReveal>}
    {taskActive(record) ? <div className="page-actions"><MotionButton className="primary-button" disabled={mutation.isPending || navigator.onLine === false} onClick={() => act(record.state === 'PAUSED' ? 'resume' : 'pause')}>{record.state === 'PAUSED' ? c.resume : c.pause}</MotionButton><MotionButton disabled={mutation.isPending || navigator.onLine === false} onClick={() => act('cancel')}>{confirm === 'cancel' ? c.confirm : c.cancel}</MotionButton></div> : record.state !== 'DELETED' && <MotionButton className="secondary-button guest-wide" disabled={mutation.isPending || navigator.onLine === false} onClick={() => act('delete')}>{confirm === 'delete' ? c.confirm : t('deleteTask')}</MotionButton>}
  </div>
}
export function AccountRequest({ request, onDone, reader }: { request: Transfer; onDone: () => void; reader?: ClientRuntime }) {
  const c = useGuestCopy(), { t, locale } = useI18n(), client = useQueryClient(), [error, setError] = useState('')
  const mutation = useMutation({ mutationFn: (action: 'accept' | 'reject') => reader ? reader.request<Transfer>('POST', `/v1/transfers/${encodeURIComponent(request.id)}/${action}`, { owner_id: reader.owner, device_id: reader.profile.device, expected_version: request.version }) : respondTransfer(request.id, action, IDENTITY.owner, IDENTITY.device, request.version), onSuccess: () => onDone(), onError: error => setError((error instanceof ApiError || error instanceof DataError) && error.status === 409 ? t('notificationStateChanged') : (error instanceof ApiError || error instanceof DataError) && error.status === 410 ? t('requestExpired') : t('actionFailed')), onSettled: () => Promise.all([client.invalidateQueries({ queryKey: ['notifications'] }), client.invalidateQueries({ queryKey: ['task-records'] })]) })
  return <div className="guest-job-detail"><div className="guest-file-row"><FileText size={40} /><span>{displayFileName(request.file_name, locale) ?? t('encryptedFile')}</span></div><div className="guest-job-meta"><span>{request.sender_device_id}</span><span>{formatBytes(request.size_bytes ?? 0, locale)}</span></div><dl className="guest-progress-detail"><div><dt>{t('senderAccount')}</dt><dd>{request.sender_owner_id}</dd></div></dl>{error && <p role="alert">{error}</p>}<div className="page-actions"><MotionButton className="secondary-button" disabled={mutation.isPending} onClick={() => mutation.mutate('reject')}>{c.decline}</MotionButton><MotionButton className="primary-button" busy={mutation.isPending} onClick={() => mutation.mutate('accept')}>{c.receive}</MotionButton></div></div>
}
