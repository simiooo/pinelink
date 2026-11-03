import type { Task } from '../api/client'
import type { Translate } from './index'

const statusKeys = { RUNNING: 'statusRunning', PAUSED: 'statusPaused', QUEUED: 'statusQueued', COMPLETED: 'statusCompleted', CANCELLED: 'statusCancelled', FAILED: 'statusFailed', DELETED: 'statusDeleted' } as const

export function taskStatus(task: Task, t: Translate) {
  if (task.kind === 'UPLOAD' && task.state === 'COMPLETED') return t('cloudStored')
  const direction = t(task.kind === 'DOWNLOAD' ? 'directionReceive' : 'directionSend')
  const key = statusKeys[task.state as keyof typeof statusKeys]
  return key ? t(key, { direction }) : t('statusUnknown')
}
export function taskError(task: Task, t: Translate) {
  const messages = { TRANSFER_CANCELLED: 'errTransferCancelled', TRANSFER_REJECTED: 'errTransferRejected', DEVICE_REVOKED: 'errDeviceRevoked', FILE_EXPIRED: 'errFileExpired', TASK_CANCELLED: 'errTaskCancelled', TASK_DELETED: 'errTaskDeleted', TASK_FAILED: 'errTaskFailed' } as const
  if (task.error_code) return t(messages[task.error_code as keyof typeof messages] ?? 'errUnknown')
  return task.error_message ?? ''
}
