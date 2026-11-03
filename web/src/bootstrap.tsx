import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { App, createAppRouter } from './main'
import { closeClient, initializeClient } from './data'
import { ClientWorker } from './data/ClientWorker'
import type { ClientRuntime } from './data/runtime'
import { API_URL, IDENTITY } from './api/client'
import { startPwa } from './pwa'
import { loadServerPin } from './data/service-profile'
import { completePendingLogout, prepareAuth, sessionExpired, signOut } from './auth/client'
import { dataAccess } from './data/access'
import { captureGuestLink } from './guest/Home'
import { getGuestRuntime, closeGuestRuntime } from './guest/runtime'

let root: Root | undefined
let router: ReturnType<typeof createAppRouter> | undefined
let disposed = false
const stopPwa = startPwa()
import.meta.hot?.dispose(() => {
  disposed = true
  stopPwa()
  root?.unmount()
  router?.history.destroy()
  void closeClient()
  void closeGuestRuntime()
})

async function bootstrap() {
  captureGuestLink()
  let runtime: ClientRuntime | undefined
  try {
    let logoutBlocked = false
    try { await completePendingLogout() } catch { logoutBlocked = true }
    const state = await prepareAuth(logoutBlocked)
    if (state.capabilities.guests) await getGuestRuntime().catch(() => undefined)
    const managed = state.capabilities.enabled
    dataAccess.managed = managed
    const account = state.session?.account ?? state.offlineAccount
    const localOnly = managed && !state.session
    if (managed && account) { IDENTITY.owner = account.id; IDENTITY.scope = account.id }
    const pin = localOnly ? '' : await loadServerPin(API_URL, import.meta.env.VITE_PINELINK_HISTORY_KEY ?? '', import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime')
    if (disposed) return
    runtime = await initializeClient(API_URL, IDENTITY.owner, managed && account ? '' : import.meta.env.VITE_PINELINK_DEVICE ?? '', pin, IDENTITY.scope, { localOnly, csrf: state.session?.csrf, onUnauthorized: () => sessionExpired(state.session?.csrf ?? '') })
    if (state.session && runtime.profile.device !== state.session.device_id) { runtime = undefined; await closeClient(); await signOut(); return }
    IDENTITY.device = runtime.profile.device
  } catch { /* Existing request error states handle unavailable local storage. */ }
  if (disposed) return
  root = createRoot(document.getElementById('root')!)
  router = createAppRouter()
  root.render(<StrictMode><ClientWorker runtime={runtime}><App router={router} /></ClientWorker></StrictMode>)
}
void bootstrap()
