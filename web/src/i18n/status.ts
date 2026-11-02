import type { Task } from '../api/client'
import type { Translate } from './index'

const statusKeys = { RUNNING: 'statusRunning', PAUSED: 'statusPaused', QUEUED: 'statusQueued', COMPLETED: 'statusCompleted', CANCELLED: 'statusCancelled', FAILED: 'statusFailed', DELETED: 'statusDeleted' } as const

export function taskStatus(task: Task, t: Translate) {
  const direction = t(task.kind === 'DOWNLOAD' ? 'directionReceive' : 'directionSend')
  const key = statusKeys[task.state as keyof typeof statusKeys]
  return key ? t(key, { direction }) : t('statusUnknown')
}
