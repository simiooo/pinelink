/// <reference lib="webworker" />
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { BACKGROUND_SYNC_TAG } from './data/background-sync'
import { runWorkerSync } from './data/worker-sync'
import { coordinateUpdate } from './pwa/update-coordinator'

declare const self: ServiceWorkerGlobalScope & { __WB_MANIFEST: { url: string; revision?: string | null }[] }
cleanupOutdatedCaches()
self.addEventListener('activate', event => { event.waitUntil(self.clients.claim()) })
precacheAndRoute(self.__WB_MANIFEST)
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), {
  allowlist: [/^\/(?:files|notifications|tasks|history|local-files|settings|connect|login|register|forgot|verify|reset|account|terms|privacy)?\/?(?:\?.*)?$/],
  denylist: [/^\/(?:api|v1|a|auth)(?:\/|$)/, /^\/version\.json$/, /[?&](?:state|code|error)=/],
}))
// Activation requires every open app page to be idle and prepared. No install-
// time skipWaiting, cache reset, or automatic reload of an active transfer.
let updating: Promise<string> | undefined
let updateAbort: AbortController | undefined, updateSession: string | undefined
self.addEventListener('message', event => {
  if (!event.source || !('id' in event.source)) return
  const source = event.source as Client
  if (new URL(source.url).origin !== self.location.origin) return
  if (event.data?.type === 'PINELINK_CANCEL_UPDATE' && event.data.session === updateSession) { updateAbort?.abort(); return }
  if (event.data?.type !== 'PINELINK_APPLY_UPDATE' || !event.ports[0] || typeof event.data.session !== 'string' || event.data.session.length > 64) return
  if (!updating) {
    updateAbort = new AbortController(); updateSession = event.data.session
    const timeout = setTimeout(() => updateAbort?.abort(), 10000)
    updating = coordinateUpdate(async () => (await self.clients.matchAll({ type: 'window', includeUncontrolled: true })).filter(client => new URL(client.url).origin === self.location.origin && !new URL(client.url).pathname.startsWith('/designs/')), () => { void self.skipWaiting(); return Promise.resolve() }, updateSession!, updateAbort.signal).finally(() => { clearTimeout(timeout); updating = undefined; updateAbort = undefined; updateSession = undefined })
  }
  event.waitUntil(updating.then(status => event.ports[0].postMessage({ status })))
})
let active: Promise<void> | undefined
self.addEventListener('sync', raw => {
  const event = raw as ExtendableEvent & { tag: string }
  if (event.tag !== BACKGROUND_SYNC_TAG) return
  active ??= runWorkerSync({
    server: import.meta.env.VITE_PINELINK_API_URL ?? 'http://127.0.0.1:8080',
    owner: import.meta.env.VITE_PINELINK_OWNER ?? 'dev-owner',
    device: import.meta.env.VITE_PINELINK_DEVICE ?? '',
    pin: import.meta.env.VITE_PINELINK_HISTORY_KEY ?? '',
    runtimeProfile: import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime',
  }).finally(() => { active = undefined })
  event.waitUntil(active)
})
