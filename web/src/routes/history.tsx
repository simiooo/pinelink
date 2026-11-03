import { NativeSelect } from '../components/ui/native-select'
import { MotionButton, MotionReveal } from '../motion/components'
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, controlTask } from '../api/client'
import { useTaskRecords } from '../api/taskRecords'
import type { TaskRecord } from '../api/taskRecords'
import type { Task } from '../api/client'
import { formatBytes, formatTime } from '../format'
import { useI18n } from '../i18n'
import type { Translate } from '../i18n'
import { taskStatus } from '../i18n/status'

export function taskError(task: TaskRecord, t: Translate) {
  const messages: Record<string, Parameters<Translate>[0]> = {
    TRANSFER_CANCELLED: 'errTransferCancelled',
    TRANSFER_REJECTED: 'errTransferRejected',
    DEVICE_REVOKED: 'errDeviceRevoked',
    FILE_EXPIRED: 'errFileExpired',
    TASK_CANCELLED: 'errTaskCancelled',
    TASK_DELETED: 'errTaskDeleted',
    TASK_FAILED: 'errTaskFailed',
  }
  if (task.error_code) return t(messages[task.error_code] ?? 'errUnknown')
  return task.error_message ?? ''
}

export function HistoryPage() {
  const client = useQueryClient()
  const { t, locale } = useI18n()
  const records = useTaskRecords(true)
  const [selected, setSelected] = useState<string>()
  const [state, setState] = useState('')
  const [kind, setKind] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [notice, setNotice] = useState('')
  const filtered = (records.data ?? []).filter(task => (!state || task.state === state) && (!kind || task.kind === kind))
  const task = filtered.find(item => item.id === selected) ?? filtered[0]
  const mutation = useMutation({
    mutationFn: (target: Pick<Task, 'id' | 'version'>) => controlTask(target.id, 'delete', target.version),
    onSuccess: () => { setNotice(t('deletedSuccess')); setConfirm(false) },
    onError: error => setNotice(error instanceof ApiError && error.status === 409 ? t('stateChangedRefresh') : t('deleteFailed')),
    onSettled: () => Promise.all([client.invalidateQueries({ queryKey: ['task-records'] }), client.invalidateQueries({ queryKey: ['tasks'] })]),
  })
  const fileName = (record: TaskRecord) => record.encryptedMetadata ? t('encryptedFile') : record.fileName || t(record.kind === 'DOWNLOAD' ? 'receivedFile' : 'sentFile')
  return (
    <section aria-labelledby="history-title">
      <div className="page-heading"><h1 id="history-title">{t('historyTitle')}</h1><p>{t('historySubtitle')}</p></div>
      <div className="history-filters">
        <label>{t('stateFilter')}<NativeSelect value={state} onChange={event => { setState(event.target.value); setSelected(undefined); setConfirm(false) }}><option value="">{t('allStates')}</option><option value="COMPLETED">{t('completedState')}</option><option value="PAUSED">{t('pausedState')}</option><option value="CANCELLED">{t('cancelledState')}</option><option value="FAILED">{t('failedState')}</option><option value="DELETED">{t('deletedState')}</option><option value="RUNNING">{t('runningState')}</option></NativeSelect></label>
        <label>{t('kindFilter')}<NativeSelect value={kind} onChange={event => { setKind(event.target.value); setSelected(undefined); setConfirm(false) }}><option value="">{t('allKinds')}</option><option value="DOWNLOAD">{t('directionReceive')}</option><option value="UPLOAD">{t('directionSend')}</option></NativeSelect></label>
        {filtered.length > 1 && <label>{t('recordFilter')}<NativeSelect value={task?.id ?? ''} onChange={event => { setSelected(event.target.value); setConfirm(false); setNotice('') }}>{filtered.map(item => <option key={item.id} value={item.id}>{fileName(item)} · {taskStatus(item, t)}</option>)}</NativeSelect></label>}
      </div>
      {notice && <MotionReveal className="feedback" role="status" identity={notice}>{notice}</MotionReveal>}
      {records.isPending && <div className="state-panel">{t('historyLoading')}</div>}
      {records.isError && <div className="state-panel" role="alert">{t('historyError')}</div>}
      {!records.isPending && !records.isError && !task && <div className="state-panel empty-state"><strong>{t('noHistory')}</strong><p>{t('noHistoryHint')}</p></div>}
      {task && <>
        <div className="task-heading history-title"><strong>{fileName(task)}</strong><span className={`status-badge status-${task.state.toLowerCase()}`}>{taskStatus(task, t)}</span></div>
        <article className="history-card">
          <dl className="metadata-list">
            <div><dt>{t('direction')}</dt><dd>{t(task.kind === 'DOWNLOAD' ? 'directionReceive' : 'directionSend')}</dd></div>
            {task.peer && <div><dt>{t('peerDevice')}</dt><dd>{task.peer}</dd></div>}
            <div><dt>{t('transferred')}</dt><dd>{formatBytes(task.progress_bytes, locale)} / {task.total_bytes > 0 ? formatBytes(task.total_bytes, locale) : t('sizeUnknown')}</dd></div>
            <div><dt>{t('time')}</dt><dd>{formatTime(task.updated_at, locale)}</dd></div>
          </dl>
          {taskError(task, t) && <p className="error-note" role="note">{taskError(task, t)}</p>}
        </article>
        {task.state !== 'RUNNING' && task.state !== 'DELETED' && <MotionButton className="secondary-button history-delete" disabled={mutation.isPending} onClick={() => {
          if (confirm) return mutation.mutateAsync(task)
          else { setConfirm(true); setNotice(t('deleteWarning')) }
        }}>{t(confirm ? 'confirmDelete' : 'deleteTask')}</MotionButton>}
        <p className="detail-note">{t('deleteNote')}</p>
      </>}
    </section>
  )
}
