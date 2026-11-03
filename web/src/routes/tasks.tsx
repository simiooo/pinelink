import { NativeSelect } from '../components/ui/native-select'
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, controlTask, getPull, getTasks, getTransfers } from '../api/client'
import type { Task } from '../api/client'
import { displayFileName, formatBytes, formatTime } from '../format'
import { useI18n } from '../i18n'
import { taskStatus } from '../i18n/status'
import { MotionButton, MotionReveal, MotionSpinner } from '../motion/components'
import { TransferGraph } from '../motion/graph'
import { MotionSurface } from '../motion/surface'
import { useOnline } from '../motion/react'
import { confirmedPercent } from '../motion/tokens'

export { taskStatus } from '../i18n/status'

export function TasksPage() {
  const client = useQueryClient(), { t, locale } = useI18n(), online = useOnline()
  const [selected, setSelected] = useState<string>(), [details, setDetails] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false), [notice, setNotice] = useState('')
  const tasks = useQuery({ queryKey: ['tasks'], queryFn: () => getTasks(), refetchInterval: 3000 })
  const transfers = useQuery({ queryKey: ['transfers'], queryFn: getTransfers, refetchInterval: 3000 })
  const list = tasks.data?.tasks ?? []
  const task = list.find(item => item.id === selected) ?? list.find(item => ['RUNNING', 'PAUSED', 'QUEUED'].includes(item.state)) ?? list[0]
  const transfer = transfers.data?.find(item => item.id === task?.reference_id || (task?.kind === 'UPLOAD' && item.entry_id === task.reference_id))
  const pull = useQuery({ queryKey: ['pull', task?.reference_id], queryFn: () => getPull(task!.reference_id), enabled: task?.kind === 'DOWNLOAD' && !transfer, retry: false })
  const visible = useRef(task), alive = useRef(true)
  visible.current = task
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const mutation = useMutation({
    mutationFn: ({ id, action, version }: { id: string; action: 'pause' | 'resume' | 'cancel'; version: number }) => controlTask(id, action, version),
    onSuccess: result => {
      client.setQueryData(['tasks'], (current: { tasks: Task[] } | undefined) => current && ({ ...current, tasks: current.tasks.map(item => item.id === result.id && item.version <= result.version ? result : item) }))
      if (alive.current && visible.current?.id === result.id && visible.current.version <= result.version) {
        setConfirmCancel(false); setNotice(t(result.state === 'PAUSED' ? 'pauseSuccess' : result.state === 'RUNNING' ? 'resumeSuccess' : 'cancelSuccess'))
      }
    },
    onError: (error, variables) => {
      if (alive.current && visible.current?.id === variables.id) setNotice(error instanceof ApiError && error.status === 409 ? t('stateChangedRefresh') : t('actionFailed'))
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['tasks'] }),
  })
  if (tasks.isPending) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1></div><MotionReveal className="state-panel" role="status" identity="loading"><MotionSpinner /> {t('loadingTasks')}</MotionReveal></section>
  if (tasks.isError) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1></div><MotionReveal className="state-panel" role="alert" identity="error">{t('tasksError')}</MotionReveal></section>
  if (!task) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1></div><MotionReveal className="state-panel empty-state" identity="empty"><strong>{t('noActiveTasks')}</strong></MotionReveal></section>

  const fraction = task.total_bytes > 0 ? Math.min(1, Math.max(0, task.progress_bytes / task.total_bytes)) : task.state === 'COMPLETED' ? 1 : 0
  const percent = task.total_bytes > 0 ? confirmedPercent(fraction) : task.state === 'COMPLETED' ? 100 : null
  const active = ['RUNNING', 'PAUSED', 'QUEUED'].includes(task.state)
  const cloud = task.kind === 'UPLOAD' || !transfer || Boolean(transfer.delivery_mode && !transfer.delivery_mode.startsWith('P2P'))
  const fileName = displayFileName(transfer?.file_name ?? pull.data?.file_name, locale) ?? t(task.kind === 'DOWNLOAD' ? 'receivedFile' : 'sentFile')
  const peer = task.kind === 'DOWNLOAD' ? transfer?.sender_device_id : transfer?.recipient_device_id
  const status = task.kind === 'UPLOAD' && task.state === 'COMPLETED' ? t('cloudStored') : task.state === 'RUNNING' && fraction === 1 ? t('awaitingConfirmation') : taskStatus(task, t)
  const control = (action: 'pause' | 'resume' | 'cancel') => mutation.mutateAsync({ id: task.id, version: task.version, action })
  const feedback = notice && <MotionReveal className="feedback" role="status" identity={notice}>{notice}</MotionReveal>
  return <section className="tasks-page" aria-labelledby="tasks-title">
    <div className="page-heading"><h1 id="tasks-title">{t('tasksTitle')}</h1></div>
    {list.length > 1 && <label className="request-picker">{t('selectTask')}<NativeSelect value={task.id} onChange={event => { setSelected(event.target.value); setConfirmCancel(false); setNotice(''); setDetails(false) }}>{list.map(item => <option value={item.id} key={item.id}>{taskStatus(item, t)} · {item.id.slice(0, 8)}</option>)}</NativeSelect></label>}
    {!details && feedback}
    <TransferGraph key={task.id} value={fraction} running={task.state === 'RUNNING' && online && tasks.data?.live !== false} completed={task.state === 'COMPLETED'}
      incoming={task.kind === 'DOWNLOAD'} cloud={cloud} localLabel={t('thisDevice')} peerLabel={cloud ? t('cloudNode') : peer ?? t('peerDevice')}
      statusLabel={status} progressLabel={t('progressLabel')} detailsLabel={t('transferDetails')} onDetails={() => setDetails(true)} />
    <div className="task-graph-caption"><MotionButton onClick={() => setDetails(true)}>{status}</MotionButton></div>
    <MotionSurface open={details} onClose={() => { setDetails(false); setConfirmCancel(false) }} title={fileName} closeLabel={t('close')}>
      <div className="task-details">
        {feedback}
        <strong className="task-percent">{percent == null ? t('waitingData') : `${percent}%`}</strong>
        <dl className="metadata-list">
          <div><dt>{t('transferState')}</dt><dd>{status}</dd></div>
          <div><dt>{t('transferred')}</dt><dd>{formatBytes(task.progress_bytes, locale)} / {task.total_bytes > 0 ? formatBytes(task.total_bytes, locale) : t('sizePending')}</dd></div>
          {peer && <div><dt>{t('peerDevice')}</dt><dd>{peer}</dd></div>}
          <div><dt>{t('time')}</dt><dd>{formatTime(task.updated_at, locale)}</dd></div>
        </dl>
        {task.error_message && <MotionReveal role="alert" className="error-note" identity={task.error_message}>{t('errorReason', { reason: task.error_message })}</MotionReveal>}
        {confirmCancel && <MotionReveal className="detail-note" identity="confirm">{t('cancelWarning')}</MotionReveal>}
        {active && <div className="page-actions">
          <MotionButton className="primary-button" disabled={mutation.isPending || !online} onClick={() => control(task.state === 'PAUSED' ? 'resume' : 'pause')}>{t(task.state === 'PAUSED' ? 'resume' : 'pause')}</MotionButton>
          <MotionButton disabled={mutation.isPending || !online} onClick={() => {
            if (confirmCancel) return control('cancel')
            setConfirmCancel(true)
          }}>{t(confirmCancel ? 'confirmCancel' : 'cancel')}</MotionButton>
        </div>}
      </div>
    </MotionSurface>
  </section>
}
