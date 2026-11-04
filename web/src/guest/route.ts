import type { GuestJob } from './store'

export function displayedRoute(job?: GuestJob): 'idle' | 'connecting' | 'switching' | 'direct' | 'relay' {
  if (!job?.transfer) return 'idle'
  if (job.snapshot?.completion.delivered && job.snapshot.route?.kind === 'DIRECT') return 'direct'
  if (job.transport?.state === 'cloud' || job.snapshot && !['NOT_STARTED', 'NOT_REQUIRED'].includes(job.snapshot.spool.state)) return 'relay'
  if (job.transport?.state === 'connecting' || job.transport?.state === 'switching') return job.transport.state
  if (job.transport?.state === 'direct') return 'direct'
  if (['complete', 'paused'].includes(job.phase) && job.snapshot?.route?.kind === 'DIRECT') return 'direct'
  return 'connecting'
}
