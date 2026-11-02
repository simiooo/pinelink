import { useEffect } from 'react'
import type { ReactNode } from 'react'
import type { ClientRuntime } from './runtime'

// React owns the background subscription. The session owns the database and
// foreground requests, which remain usable across StrictMode effect replay.
export function ClientWorker({ runtime, children }: { runtime?: ClientRuntime; children: ReactNode }) {
  useEffect(() => {
    runtime?.start()
    return () => runtime?.stop()
  }, [runtime])
  return children
}
