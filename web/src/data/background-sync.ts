export const BACKGROUND_SYNC_TAG = 'pinelink-client-sync-v1'
type SyncRegistration = ServiceWorkerRegistration & { sync?: { register(tag: string): Promise<void> } }
let registering: Promise<void> | undefined
let lastRegistration = 0

// An enhancement only. Foreground/reopen recovery remains the portable path.
// Never persist signed HTTP requests: their timestamps and authorization expire.
export function requestBackgroundSync(force = false): Promise<void> {
  if (typeof window === 'undefined' || !('serviceWorker' in navigator)) return Promise.resolve()
  if (registering) return registering
  if (!force && Date.now() - lastRegistration < 30000) return Promise.resolve()
  registering = navigator.serviceWorker.getRegistration().then(async value => {
    const sync = (value as SyncRegistration | undefined)?.sync
    if (sync) { await sync.register(BACKGROUND_SYNC_TAG); lastRegistration = Date.now() }
  }).catch(() => undefined).finally(() => { registering = undefined })
  return registering
}
