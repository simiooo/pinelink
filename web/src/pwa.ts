// Registration belongs to the application lifetime, outside React effect replay.
// A new worker waits for all old clients to close; it never reloads a transfer.
import { requestBackgroundSync } from './data/background-sync'

export function startPwa() {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return () => undefined
  let disposed = false
  let registration: ServiceWorkerRegistration | undefined
  let lastCheck = 0
  const check = () => {
    if (disposed || !registration || document.visibilityState === 'hidden' || Date.now() - lastCheck < 15 * 60 * 1000) return
    lastCheck = Date.now()
    void registration.update().catch(() => undefined)
  }
  const persistInstalledData = () => {
    if (!disposed) void navigator.storage?.persist?.().catch(() => false)
  }
  const register = () => {
    if (disposed) return
    void navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' }).then(value => {
      if (disposed) return
      registration = value
      lastCheck = Date.now()
      void navigator.serviceWorker.ready.then(() => { if (!disposed) void requestBackgroundSync(true) })
      if (window.matchMedia('(display-mode: standalone)').matches) persistInstalledData()
    }).catch(() => undefined)
  }
  if (document.readyState === 'complete') register()
  else window.addEventListener('load', register, { once: true })
  window.addEventListener('online', check)
  const offline = () => { void requestBackgroundSync(true) }
  window.addEventListener('offline', offline)
  window.addEventListener('pageshow', check)
  window.addEventListener('appinstalled', persistInstalledData)
  document.addEventListener('visibilitychange', check)
  const timer = setInterval(check, 60 * 60 * 1000)
  return () => {
    disposed = true
    clearInterval(timer)
    window.removeEventListener('load', register)
    window.removeEventListener('online', check)
    window.removeEventListener('offline', offline)
    window.removeEventListener('pageshow', check)
    window.removeEventListener('appinstalled', persistInstalledData)
    document.removeEventListener('visibilitychange', check)
  }
}
