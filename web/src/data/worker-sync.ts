import { ClientRuntime } from './runtime'
import { LocalStore, storeName } from './store'
import { loadServerPin } from './service-profile'

export type WorkerConfiguration = { server: string; owner: string; device: string; pin: string; runtimeProfile?: boolean }

// Resume the same verified journal and identity. A worker must never generate a
// replacement device when the application's local identity is unavailable.
export async function runWorkerSync(configuration: WorkerConfiguration) {
  const server = new URL(configuration.server, location.href).href.replace(/\/$/, '')
  let csrf: string | undefined
  if (configuration.runtimeProfile) {
    const response = await fetch(server + '/v1/auth/capabilities', { credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(5000) })
    if (!response.ok) return
    const capabilities = await response.json() as { enabled: boolean }
    if (capabilities.enabled) {
      const current = await fetch(server + '/v1/auth/session', { credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(5000) })
      if (!current.ok) return
      const session = await current.json() as { account: { id: string }; csrf: string }
      configuration = { ...configuration, owner: session.account.id, device: '' }; csrf = session.csrf
    }
  }
  const store = new LocalStore(await storeName(server, configuration.owner, configuration.device))
  let runtime: ClientRuntime | undefined
  try {
    if (!await store.profiles.get('self')) return
    const pin = await loadServerPin(server, configuration.pin, configuration.runtimeProfile)
    runtime = await ClientRuntime.open(server, configuration.owner, configuration.device, pin, store, { csrf })
    const target = await store.target()
    const through = target.backgroundThrough || await store.beginRead()
    if (!target.backgroundThrough) await store.patchTarget({ backgroundThrough: through })
    // Bound one background event, retaining the sweep position across OS retry.
    // Foreground and worker share the same Web Lock, cursors, and cache fencing.
    const deadline = Date.now() + 20000
    for (let pass = 0; pass < 3; pass++) {
      await runtime.sync(true)
      const pending = (await store.target()).more || await store.responses.where('checked').below(through).count() > 0 || await store.responses.where('retryPending').equals(1).count() > 0
      if (!pending) { await store.patchTarget({ backgroundThrough: 0 }); return }
      if (Date.now() >= deadline) break
    }
    throw new Error('BACKGROUND_SYNC_MORE_PENDING')
  } finally {
    if (runtime) await runtime.close()
    else store.close()
  }
}
