import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, controlTask, getPull, getTasks, getTransfers } from '../api/client'
import { Courier } from '../components/Courier'
import { displayFileName, formatBytes, formatTime } from '../format'
import { useI18n } from '../i18n'
import { taskStatus } from '../i18n/status'
import type { Task } from '../api/client'

export { taskStatus } from '../i18n/status'

export function TasksPage() {
  const client = useQueryClient()
  const { t, locale } = useI18n()
  const [selected, setSelected] = useState<string>()
  const [confirmCancel, setConfirmCancel] = useState(false)
  const [notice, setNotice] = useState('')
  const tasks = useQuery({ queryKey: ['tasks'], queryFn: () => getTasks(), refetchInterval: 3000 })
  const transfers = useQuery({ queryKey: ['transfers'], queryFn: getTransfers, refetchInterval: 3000 })
  const list = tasks.data?.tasks ?? []
  const task = list.find(item => item.id === selected) ?? list.find(item => ['RUNNING', 'PAUSED', 'QUEUED'].includes(item.state)) ?? list[0]
  const transfer = transfers.data?.find(item => item.id === task?.reference_id || (task?.kind === 'UPLOAD' && item.entry_id === task.reference_id))
  const pull = useQuery({ queryKey: ['pull', task?.reference_id], queryFn: () => getPull(task!.reference_id), enabled: task?.kind === 'DOWNLOAD' && !transfer, retry: false })
  const mutation = useMutation({
    mutationFn: ({ id, action, version }: { id: string; action: 'pause' | 'resume' | 'cancel'; version: number }) => controlTask(id, action, version),
    onSuccess: result => {
      setConfirmCancel(false)
      setNotice(t(result.state === 'PAUSED' ? 'pauseSuccess' : result.state === 'RUNNING' ? 'resumeSuccess' : 'cancelSuccess'))
      client.setQueryData(['tasks'], (current: { tasks: Task[] } | undefined) => current && ({ tasks: current.tasks.map(item => item.id === result.id && item.version <= result.version ? result : item) }))
    },
    onError: error => setNotice(error instanceof ApiError && error.status === 409 ? t('stateChangedRefresh') : t('actionFailed')),
    onSettled: () => client.invalidateQueries({ queryKey: ['tasks'] }),
  })

  if (tasks.isPending) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1></div><div className="state-panel">{t('loadingTasks')}</div></section>
  if (tasks.isError) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1></div><div className="state-panel" role="alert">{t('tasksError')}</div></section>
  if (!task) return <section><div className="page-heading"><h1>{t('tasksTitle')}</h1><p>{t('noActiveTasksHint')}</p></div><div className="state-panel empty-state"><strong>{t('noActiveTasks')}</strong><p>{t('noActiveTasksHint')}</p></div></section>

  const percent = task.total_bytes > 0 ? Math.min(100, Math.round(task.progress_bytes / task.total_bytes * 100)) : task.state === 'COMPLETED' ? 100 : null
  const active = task.state === 'RUNNING' || task.state === 'PAUSED' || task.state === 'QUEUED'
  const fileName = displayFileName(transfer?.file_name ?? pull.data?.file_name, locale) ?? t(task.kind === 'DOWNLOAD' ? 'receivedFile' : 'sentFile')
  const peer = task.kind === 'DOWNLOAD' ? transfer?.sender_device_id : transfer?.recipient_device_id
  const status = taskStatus(task, t)
  const note = task.state === 'PAUSED' ? t('pausedNote') : task.state === 'QUEUED' ? t('queuedNote') : task.state === 'COMPLETED' ? t(task.kind === 'DOWNLOAD' ? 'downloadCompleteNote' : 'uploadCompleteNote') : t('activeNote', { direction: t(task.kind === 'DOWNLOAD' ? 'directionReceive' : 'directionSend') })
  return (
    <section className="tasks-page" aria-labelledby="tasks-title">
      <div className="page-heading"><h1 id="tasks-title">{t('tasksTitle')}</h1><p>{t('tasksSubtitle')}</p></div>
      {list.length > 1 && <label className="request-picker">{t('selectTask')}<select value={task.id} onChange={event => { setSelected(event.target.value); setConfirmCancel(false); setNotice('') }}>{list.map(item => <option value={item.id} key={item.id}>{taskStatus(item, t)} · {item.id.slice(0, 8)}</option>)}</select></label>}
      {notice && <p className="feedback" role="status">{notice}</p>}
      <article className="task-card">
        <div className="task-heading"><strong title={fileName}>{fileName}</strong><span className={`status-badge status-${task.state.toLowerCase()}`}>{status}</span></div>
        <div className="task-progress-layout">
          <Courier action={task.state === 'PAUSED' ? 'pause' : task.state === 'COMPLETED' ? (task.kind === 'UPLOAD' ? 'store' : 'complete') : task.state === 'RUNNING' ? 'run' : 'pause'} label={t('courierStatus', { status })} />
          <div className="task-progress">
            <strong className="task-percent">{percent == null ? t('waitingData') : `${percent}%`}</strong>
            {percent != null && <div className="progress-track" role="progressbar" aria-label={t('progressLabel')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><span style={{ width: `${percent}%` }} /></div>}
            <span>{formatBytes(task.progress_bytes, locale)} / {task.total_bytes > 0 ? formatBytes(task.total_bytes, locale) : t('sizePending')}</span>
            {peer && <span>{t(task.kind === 'DOWNLOAD' ? 'from' : 'to')} {peer}</span>}
            <small>{t('updatedAt', { time: formatTime(task.updated_at, locale) })}</small>
          </div>
        </div>
        <p className="task-note">{note}</p>
        {task.error_message && <p className="error-note" role="alert">{t('errorReason', { reason: task.error_message })}</p>}
      </article>
      {active && <div className="page-actions">
        <button className="primary-button" disabled={mutation.isPending} onClick={() => mutation.mutate({ id: task.id, version: task.version, action: task.state === 'PAUSED' ? 'resume' : 'pause' })}>{t(task.state === 'PAUSED' ? 'resume' : 'pause')}</button>
        <button className="secondary-button" disabled={mutation.isPending} onClick={() => {
          if (confirmCancel) mutation.mutate({ id: task.id, version: task.version, action: 'cancel' })
          else { setConfirmCancel(true); setNotice(t('cancelWarning')) }
        }}>{t(confirmCancel ? 'confirmCancel' : 'cancel')}</button>
      </div>}
    </section>
  )
}
