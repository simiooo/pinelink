/// <reference lib="webworker" />
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { BACKGROUND_SYNC_TAG } from './data/background-sync'
import { runWorkerSync } from './data/worker-sync'

declare const self: ServiceWorkerGlobalScope & { __WB_MANIFEST: { url: string; revision?: string | null }[] }
cleanupOutdatedCaches()
precacheAndRoute(self.__WB_MANIFEST)
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), {
  allowlist: [/^\/(?:notifications|tasks|history|settings)?\/?(?:\?.*)?$/],
  denylist: [/^\/(?:api|v1)(?:\/|$)/, /^\/version\.json$/],
}))
// No skipWaiting/clientsClaim or forced reload: installed updates wait for the
// existing app windows to close. API/content never enter a Workbox cache.
let active: Promise<void> | undefined
self.addEventListener('sync', raw => {
  const event = raw as ExtendableEvent & { tag: string }
  if (event.tag !== BACKGROUND_SYNC_TAG) return
  active ??= runWorkerSync({
    server: import.meta.env.VITE_PINELINK_API_URL ?? 'http://127.0.0.1:8080',
    owner: import.meta.env.VITE_PINELINK_OWNER ?? 'dev-owner',
    device: import.meta.env.VITE_PINELINK_DEVICE ?? '',
    pin: import.meta.env.VITE_PINELINK_HISTORY_KEY ?? '',
  }).finally(() => { active = undefined })
  event.waitUntil(active)
})
