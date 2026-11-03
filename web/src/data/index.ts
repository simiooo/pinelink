import { ClientRuntime } from './runtime'
import type { RuntimeAccess } from './runtime'

let active: ClientRuntime | undefined
let failure: unknown
let pending: Promise<ClientRuntime> | undefined
let configuration: string | undefined
let closing: Promise<void> | undefined
export const clientRuntime = () => {
  if (failure) throw failure
  return active
}
export async function initializeClient(server: string, owner: string, device: string, pin: string, scope = import.meta.env.VITE_PINELINK_SCOPE ?? 'dev-scope', access: RuntimeAccess = {}) {
  if (closing) throw new Error('CLIENT_CLOSING')
  const key = JSON.stringify([new URL(server, location.href).href.replace(/\/$/, ''), owner, device, pin, scope, access.localOnly, access.csrf])
  if (configuration && configuration !== key) throw new Error('CLIENT_CONFIGURATION_CHANGED')
  if (active) return active
  if (pending) return pending
  configuration = key
  failure = undefined
  pending = ClientRuntime.open(server, owner, device, pin, undefined, access).then(async runtime => {
    try { await runtime.prime(scope) } catch (error) { await runtime.close(); throw error }
    active = runtime
    return runtime
  }).catch(error => {
    failure = error
    configuration = undefined
    throw error
  }).finally(() => { pending = undefined })
  return pending
}
export function closeClient(): Promise<void> {
  if (closing) return closing
  closing = (async () => {
    const runtime = active ?? await pending?.catch(() => undefined)
    await runtime?.close()
    active = undefined
    failure = undefined
    configuration = undefined
  })().finally(() => { closing = undefined })
  return closing
}
