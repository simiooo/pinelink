import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { App, createAppRouter } from './main'
import { closeClient, initializeClient } from './data'
import { ClientWorker } from './data/ClientWorker'
import type { ClientRuntime } from './data/runtime'
import { API_URL, IDENTITY } from './api/client'
import { startPwa } from './pwa'

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
})

async function bootstrap() {
  let runtime: ClientRuntime | undefined
  try {
    runtime = await initializeClient(API_URL, IDENTITY.owner, import.meta.env.VITE_PINELINK_DEVICE ?? '', import.meta.env.VITE_PINELINK_HISTORY_KEY ?? '')
    IDENTITY.device = runtime.profile.device
  } catch { /* Existing request error states handle unavailable local storage. */ }
  if (disposed) return
  root = createRoot(document.getElementById('root')!)
  router = createAppRouter()
  root.render(<StrictMode><ClientWorker runtime={runtime}><App router={router} /></ClientWorker></StrictMode>)
}
void bootstrap()
