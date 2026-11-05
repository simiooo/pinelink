import { AuthError } from '../auth/client'
import { decodePackage } from '../local/protocol'
import { GuestError } from './runtime'
import { parseConnectionInput } from './connection-input'
import type { GuestCandidate } from './types'

export const unreachable = (error: unknown) => error instanceof TypeError || error instanceof DOMException && ['TimeoutError', 'NetworkError'].includes(error.name) || (error instanceof GuestError || error instanceof AuthError) && error.status >= 500

export async function joinConnection(value: string, origin: string, actions: {
  online: boolean; joinOnline: (value: string) => Promise<GuestCandidate>; joinLocal: (value: string) => Promise<unknown>
}) {
  const input = parseConnectionInput(value, origin)
  if (input.kind === 'online') {
    if (!actions.online) throw new Error('CONNECTION_SCAN_REQUIRED')
    try { return { kind: 'online' as const, candidate: await actions.joinOnline(input.value) } }
    catch (error) { if (unreachable(error)) throw new Error('CONNECTION_SCAN_REQUIRED'); throw error }
  }
  const packet = await decodePackage(input.value)
  const route = packet.rendezvous
  if (route && route.origin !== origin) throw new Error('INVALID_REQUEST')
  if (route && actions.online && Date.parse(route.expires_at) > Date.now()) {
    try { return { kind: 'online' as const, candidate: await actions.joinOnline(route.token) } }
    catch (error) { if (!unreachable(error)) throw error }
  }
  await actions.joinLocal(input.value)
  return { kind: 'local' as const }
}
