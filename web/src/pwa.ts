import { requestBackgroundSync } from './data/background-sync'
import { freezeForUpdate, releaseUpdate, safeToUpdate, updateFrozen } from './pwa/guard'
import { pendingUpdate, updateMessageSource } from './pwa/messages'
export const appVersion = import.meta.env.VITE_PINELINK_VERSION || 'dev'
type Status = 'idle' | 'checking' | 'latest' | 'offline' | 'error' | 'available' | 'busy' | 'legacy' | 'applying'
let state: { status: Status; waiting: boolean; locked: boolean } = { status: 'idle', waiting: false, locked: false }
const listeners = new Set<() => void>()
const update = (patch: Partial<typeof state>) => { state = { ...state, ...patch }; for (const listener of listeners) listener() }
export const pwaSnapshot = () => state
export const subscribePwa = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
let registration: ServiceWorkerRegistration | undefined, checkNow: (() => Promise<void>) | undefined
export const dismissUpdateNotice = () => update({ status: 'idle' })
export const checkAppUpdate = () => checkNow?.() ?? Promise.resolve()

export async function applyAppUpdate() {
  if (!safeToUpdate()) { update({ status: 'busy' }); return }
  const worker = pendingUpdate(registration)
  if (!worker) { await checkAppUpdate(); return }
  update({ status: 'applying' })
  const session = crypto.randomUUID()
  const result = await new Promise<string>(resolve => {
    const channel = new MessageChannel()
    const timer = setTimeout(() => worker.postMessage({ type: 'PINELINK_CANCEL_UPDATE', session }), 12000)
    channel.port1.onmessage = event => { clearTimeout(timer); channel.port1.close(); resolve(event.data?.status ?? 'error') }
    worker.postMessage({ type: 'PINELINK_APPLY_UPDATE', session }, [channel.port2])
  }).catch(() => 'error')
  if (result !== 'applied') { releaseUpdate(); update({ status: result === 'legacy' ? 'legacy' : result === 'busy' ? 'busy' : 'error', locked: false }) }
}

export function startPwa() {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return () => undefined
  let disposed = false, lastCheck = 0, checking: Promise<void> | undefined, prepare: string | undefined, deadline: ReturnType<typeof setTimeout> | undefined
  const refreshWaiting = () => {
    if (disposed) return false
    if (pendingUpdate(registration)) { update({ status: 'available', waiting: true }); return true }
    if (state.waiting && !state.locked) update({ status: 'idle', waiting: false })
    return false
  }
  const unlock = () => { clearTimeout(deadline); prepare = undefined; releaseUpdate(); update({ locked: false }) }
  const message = (event: MessageEvent) => {
    const source = updateMessageSource(event, location.origin)
    if (!source) return
    const { phase, session } = event.data
    if (typeof session !== 'string' || session.length > 64) return
    if (phase === 'CHECK') event.ports[0]?.postMessage({ ready: safeToUpdate() })
    else if (phase === 'PREPARE') {
      const ready = freezeForUpdate()
      if (ready) { prepare = session; update({ status: 'applying', locked: true }); deadline = setTimeout(() => { if (source.state === 'redundant') { unlock(); update({ status: 'error' }) } else source.postMessage({ type: 'PINELINK_CANCEL_UPDATE', session }) }, 15000) }
      event.ports[0]?.postMessage({ ready })
    } else if (phase === 'CANCEL' && prepare === session) { const previous = state.status; unlock(); if (previous === 'applying') update({ status: 'available' }) }
  }
  const changed = () => { if (prepare && updateFrozen()) { clearTimeout(deadline); location.reload() } else refreshWaiting() }
  const blockInteraction = (event: Event) => { if (updateFrozen()) { event.preventDefault(); event.stopImmediatePropagation() } }
  const check = (manual = false) => {
    if (disposed || checking) return checking ?? Promise.resolve()
    if (!manual && (document.hidden || Date.now() - lastCheck < 60000)) return Promise.resolve()
    if (navigator.onLine === false) { if (!refreshWaiting() && manual) update({ status: 'offline' }); return Promise.resolve() }
    lastCheck = Date.now(); if (manual) update({ status: 'checking' })
    checking = (async () => {
      try {
        registration ??= await navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' })
        await registration.update()
        const installing = registration.installing
        if (installing) await new Promise<void>((resolve, reject) => {
          const timer = setTimeout(() => { installing.removeEventListener('statechange', changed); reject(new Error('UPDATE_TIMEOUT')) }, 15000)
          const changed = () => { if (['installed', 'redundant'].includes(installing.state)) { clearTimeout(timer); installing.removeEventListener('statechange', changed); resolve() } }
          installing.addEventListener('statechange', changed); changed()
        })
        if (disposed) return
        if (installing?.state === 'redundant' && !pendingUpdate(registration)) throw new Error('UPDATE_INSTALL_FAILED')
        if (!refreshWaiting() && manual) update({ status: 'latest', waiting: false })
      } catch { if (manual && !disposed) update({ status: navigator.onLine === false ? 'offline' : 'error' }) }
    })().finally(() => { checking = undefined })
    return checking
  }
  checkNow = () => check(true)
  const wake = () => { void check() }
  const persistInstalledData = () => { void navigator.storage?.persist?.().catch(() => false) }
  const register = () => {
    void navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' }).then(value => {
      if (disposed) return
      registration = value; refreshWaiting()
      value.addEventListener('updatefound', () => { const worker = value.installing; worker?.addEventListener('statechange', () => { if (worker.state === 'installed') refreshWaiting() }) })
      void navigator.serviceWorker.ready.then(() => { if (!disposed) void requestBackgroundSync(true) })
      if (window.matchMedia('(display-mode: standalone)').matches) persistInstalledData()
      void check()
    }).catch(() => undefined)
  }
  navigator.serviceWorker.addEventListener('message', message)
  navigator.serviceWorker.addEventListener('controllerchange', changed)
  const events = ['click', 'keydown', 'beforeinput', 'submit', 'drop']
  for (const event of events) document.addEventListener(event, blockInteraction, { capture: true, passive: false })
  if (document.readyState === 'complete') register(); else window.addEventListener('load', register, { once: true })
  window.addEventListener('online', wake); window.addEventListener('pageshow', wake); document.addEventListener('visibilitychange', wake); window.addEventListener('appinstalled', persistInstalledData)
  const timer = setInterval(wake, 5 * 60 * 1000)
  return () => {
    disposed = true; checkNow = undefined; registration = undefined; clearInterval(timer); unlock()
    navigator.serviceWorker.removeEventListener('message', message); navigator.serviceWorker.removeEventListener('controllerchange', changed)
    for (const event of events) document.removeEventListener(event, blockInteraction, true)
    window.removeEventListener('load', register); window.removeEventListener('online', wake); window.removeEventListener('pageshow', wake); document.removeEventListener('visibilitychange', wake); window.removeEventListener('appinstalled', persistInstalledData)
  }
}
