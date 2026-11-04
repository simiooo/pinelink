import Dexie from 'dexie'
import type { Table } from 'dexie'
import { base64, digest, nonce, utf8 } from '../data/crypto'
import { LocalStore, storeName } from '../data/store'
import { API_URL, IDENTITY } from '../api/client'
import { closeClient } from '../data'
import { guestWorkPending } from '../guest/continuity'

export type Account = { id: string; email: string; name: string; methods: string[] }
export type Session = { account: Account; device_id: string; expires_at: string; csrf: string }
export type Capabilities = { enabled: boolean; required: boolean; google?: boolean; password?: boolean; email?: boolean; guests?: boolean; guest_max_file_bytes?: number; local_only?: boolean }
export type Challenge = { id: string; purpose: 'signup' | 'reset'; generation: number; expires_at: string; resend_at: string; delivery_state: string }
export type Pending = { id: string; privateKey: CryptoKey; verifier: string; expires: number; email?: string; challenge?: Challenge; resetGrant?: string }
class AuthStore extends Dexie {
  pending!: Table<Pending, string>
  links!: Table<{ id: string; token: string; expires: number }, string>
  constructor() { super('pinelink-auth:' + new URL(API_URL, location.href).href.replace(/\/$/, '')); this.version(1).stores({ pending: 'id,expires', links: 'id,expires' }) }
}
const store = new AuthStore()
const prefix = 'pinelink.account:' + new URL(API_URL, location.href).href.replace(/\/$/, '')
const pendingKey = prefix + ':request'
let session: Session | undefined
let capabilities: Capabilities = { enabled: import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime', required: import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime' }
let offlineAccount: Account | undefined
let revision = 0
const listeners = new Set<() => void>()
const emit = () => { revision++; for (const listener of listeners) listener() }
export const subscribeAuth = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
export const authRevision = () => revision
export const authState = () => ({ session, capabilities, offlineAccount })
export class AuthError extends Error { constructor(public code: string, public status = 0) { super(code) } }
const cached = <T>(key: string): T | undefined => { try { return JSON.parse(localStorage.getItem(key) ?? 'null') ?? undefined } catch { return undefined } }
const save = (key: string, value: unknown) => { try { if (value === undefined) localStorage.removeItem(key); else localStorage.setItem(key, JSON.stringify(value)) } catch { /* Durable private keys still use IndexedDB. */ } }
export async function authFetch<T>(path: string, input?: unknown): Promise<T> {
  const response = await fetch(API_URL + path, { method: input === undefined ? 'GET' : 'POST', credentials: 'include', redirect: 'error', cache: 'no-store', signal: AbortSignal.timeout(12000), headers: { 'Content-Type': 'application/json' }, body: input === undefined ? undefined : JSON.stringify(input) })
  const data = await response.json().catch(() => ({})) as T & { code?: string }
  if (!response.ok) throw new AuthError(data.code ?? 'AUTH_UNAVAILABLE', response.status)
  return data
}
export async function prepareAuth(logoutBlocked = false) {
  capabilities = cached<Capabilities>(prefix + ':capabilities') ?? capabilities
  offlineAccount = logoutBlocked ? undefined : cached<Account>(prefix)
  try {
    if(navigator.onLine===false)throw new Error('LOCAL_ONLY')
    capabilities = await authFetch<Capabilities>('/v1/auth/capabilities'); save(prefix + ':capabilities', capabilities)
    if (capabilities.enabled && !logoutBlocked) {
      try { session = await authFetch<Session>('/v1/auth/session'); offlineAccount = session.account; save(prefix, offlineAccount) }
      catch (error) { if (!(error instanceof AuthError) || error.status >= 500) throw error; session = undefined }
    }
  } catch { session = undefined;capabilities={...capabilities,guests:true,local_only:true} }
  // This private journal has no replication/outbox path. Erase expired proofs.
  await Promise.all([store.pending.where('expires').below(Date.now()).delete(), store.links.where('expires').below(Date.now()).delete()]).catch(() => undefined)
  if (/^\/a\/[A-Za-z0-9_-]{22}$/.test(location.pathname)) {
    await store.links.put({ id: 'current', token: location.pathname.slice(3), expires: Date.now() + 15 * 60000 })
    history.replaceState(null, '', '/verify-link')
  }
  if (location.pathname === '/auth/return') {
    const id = new URLSearchParams(location.search).get('request_id')
    history.replaceState(null, '', '/auth/return')
    if (id) {
      try { const pending = await store.pending.get(id); if (!pending) throw new AuthError('AUTH_WRONG_DEVICE'); await finish(pending) }
      catch (error) { save(prefix + ':returnError', error instanceof AuthError ? error.code : 'AUTH_UNAVAILABLE') }
    } else save(prefix + ':returnError', 'AUTH_WRONG_DEVICE')
  }
  emit()
  return authState()
}
export function authReturnError() { const code = cached<string>(prefix + ':returnError'); save(prefix + ':returnError', undefined); return code }
export async function currentPending() { const id = sessionStorage.getItem(pendingKey); return id ? store.pending.get(id) : undefined }
export async function createPending() {
  await completePendingLogout()
  const key = await crypto.subtle.generateKey('Ed25519', false, ['sign', 'verify']) as CryptoKeyPair
  const verifier = nonce(), challenge = base64(new Uint8Array(await crypto.subtle.digest('SHA-256', utf8.encode(verifier)))).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
  const continuation = sessionStorage.getItem(prefix + ':continue') ?? 'home'
  const data = await authFetch<{ request: { id: string; expires_at: string } }>('/v1/auth/requests', { proof_key: base64(new Uint8Array(await crypto.subtle.exportKey('raw', key.publicKey))), challenge, client_kind: 'web', continuation, device_name: 'Web browser' })
  const pending: Pending = { id: data.request.id, privateKey: key.privateKey, verifier, expires: Date.parse(data.request.expires_at) }
  await store.pending.put(pending); sessionStorage.setItem(pendingKey, pending.id); return pending
}
export async function action<T>(pending: Pending, name: string, input: Record<string, unknown> = {}): Promise<T> {
  if (pending.expires < Date.now()) throw new AuthError('AUTH_EXPIRED')
  const path = `/v1/auth/requests/${pending.id}/${name}`, body = JSON.stringify({ ...input, verifier: pending.verifier })
  const signature = base64(new Uint8Array(await crypto.subtle.sign('Ed25519', pending.privateKey, utf8.encode('pinelink-auth-v1\nPOST\n' + path + '\n' + await digest(utf8.encode(body))))))
  const response = await fetch(API_URL + path, { method: 'POST', body, headers: { 'Content-Type': 'application/json', 'X-Pinelink-Auth-Signature': signature }, credentials: 'include', redirect: 'error', cache: 'no-store', signal: AbortSignal.timeout(12000) })
  const data = await response.json().catch(() => ({})) as T & { code?: string }
  if (!response.ok) throw new AuthError(data.code ?? 'AUTH_UNAVAILABLE', response.status)
  return data
}
const finishing = new Map<string, Promise<void>>()
export function finish(pending: Pending): Promise<void> {
  const existing = finishing.get(pending.id); if (existing) return existing
  const work = async () => {
    const state = await action<{ account?: Account; request: { status: string } }>(pending, 'state')
    if (!state.account || !['READY', 'CLAIMED'].includes(state.request.status)) throw new AuthError('AUTH_NOT_READY')
    const account = state.account, db = new LocalStore(await storeName(new URL(API_URL, location.href).href.replace(/\/$/, ''), account.id, ''))
    try {
      const profile = await db.profile('')
      const proof = `pinelink-enrollment-v1\n${pending.id}\n${account.id}\n${profile.device}\n${profile.signingKey}\n${profile.encryptionKey}`
      // Stop the former namespace before a cookie can authorize the next one.
      authChannel?.postMessage('changing'); await closeClient()
      const result = await action<Session & { continuation: string }>(pending, 'claim', { device_id: profile.device, signing_key: profile.signingKey, encryption_key: profile.encryptionKey, device_signature: base64(new Uint8Array(await crypto.subtle.sign('Ed25519', profile.signingPrivate, utf8.encode(proof)))) })
      session = result; offlineAccount = result.account; save(prefix, result.account); await store.pending.delete(pending.id); sessionStorage.removeItem(pendingKey)
      authChannel?.postMessage('changed'); emit()
      location.replace(result.continuation === 'home' ? '/' : '/' + result.continuation)
    } finally { db.close() }
  }
  const promise = (navigator.locks ? navigator.locks.request(prefix + ':claim:' + pending.id, work) : work()).finally(() => finishing.delete(pending.id))
  finishing.set(pending.id, promise); return promise
}
export async function login(email: string, password: string) { const pending = await createPending(); await action(pending, 'password-login', { email, password }); await finish(pending) }
export async function googleLogin() { const pending = await createPending(); const result = await action<{ url: string }>(pending, 'google-start'); const target = new URL(result.url); if (target.origin !== location.origin || target.pathname !== '/api/v1/auth/google/open') throw new AuthError('AUTH_UNAVAILABLE'); location.assign(target.href) }
export async function startEmail(purpose: 'signup' | 'reset', email: string, password = '') {
  const pending = await createPending(), challenge = await action<Challenge>(pending, purpose === 'signup' ? 'password-signup' : 'reset-start', { email, password })
  pending.challenge = challenge; pending.email = email; await store.pending.put(pending); return pending
}
export async function updateChallenge(pending: Pending, resend = false) {
  if (!pending.challenge) throw new AuthError('AUTH_EXPIRED')
  const challenge = await action<Challenge>(pending, resend ? 'email-resend' : 'email-status', { challenge_id: pending.challenge.id, generation: pending.challenge.generation })
  return store.transaction('rw', store.pending, async () => {
    const current = await store.pending.get(pending.id); if (!current?.challenge) throw new AuthError('AUTH_EXPIRED')
    if (current.challenge.generation > challenge.generation) return current
    if (current.challenge.generation === challenge.generation && ['sent', 'failed'].includes(current.challenge.delivery_state) && ['pending', 'retry'].includes(challenge.delivery_state)) return current
    const updated = { ...current, challenge }; await store.pending.put(updated); return updated
  })
}
export async function verifyCode(pending: Pending, code: string) {
  const result = await action<{ state: string; reset_grant?: string }>(pending, 'email-verify', { challenge_id: pending.challenge?.id, generation: pending.challenge?.generation, code })
  if (result.state === 'ready') await finish(pending)
  if (result.state === 'reset') { pending.resetGrant = result.reset_grant; await store.pending.put(pending) }
  return result.state
}
export async function resetPassword(pending: Pending, password: string) { await action(pending, 'reset-complete', { reset_grant: pending.resetGrant, password }); await store.pending.delete(pending.id); sessionStorage.removeItem(pendingKey); save(prefix, undefined); session = undefined; offlineAccount = undefined; authChannel?.postMessage('changed'); emit() }
export async function inspectLink() { const link = await store.links.get('current'); if (!link || link.expires < Date.now()) throw new AuthError('LINK_EXPIRED'); const info = await authFetch<{ purpose: 'signup' | 'reset'; request_id: string }>('/v1/auth/link/inspect', { token: link.token }); const pending = await store.pending.get(info.request_id); return { ...info, sameDevice: !!pending, token: link.token } }
export async function confirmLink(info: Awaited<ReturnType<typeof inspectLink>>, password: string) {
  const pending = await store.pending.get(info.request_id) ?? await createPending()
  const result = await action<{ state: string; reset_grant?: string }>(pending, 'email-verify', { link: info.token, new_password: info.purpose === 'signup' && !info.sameDevice ? password : '' })
  await store.links.delete('current')
  if (result.state === 'ready') await finish(pending)
  if (result.state === 'reset') { pending.resetGrant = result.reset_grant; await store.pending.put(pending); await resetPassword(pending, password) }
  return result.state
}
export async function signOut() {
  const csrf = session?.csrf; session = undefined; offlineAccount = undefined; save(prefix, undefined); emit(); authChannel?.postMessage('changing'); await closeClient()
  let revoked = false
  try { const response = await fetch(API_URL + '/v1/auth/logout', { method: 'POST', credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(8000), headers: { 'X-Pinelink-CSRF': csrf ?? '' } }); revoked = response.ok } catch { /* Local sign-out has already stopped cloud work. */ }
  if (!revoked) save(prefix + ':logoutPending', true)
  authChannel?.postMessage('changed'); location.replace(revoked ? '/login' : '/login?error=SIGNOUT_PENDING')
}
export async function completePendingLogout() {
  if (!cached<boolean>(prefix + ':logoutPending')) return
  // The tab must not restore an account while its deliberate revocation is pending.
  let previous: Session | undefined
  try { previous = await authFetch<Session>('/v1/auth/session') } catch (error) { if (!(error instanceof AuthError) || error.status !== 401) throw new AuthError('SIGNOUT_PENDING') }
  if (previous) { const response = await fetch(API_URL + '/v1/auth/logout', { method: 'POST', credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(8000), headers: { 'X-Pinelink-CSRF': previous.csrf } }); if (!response.ok) throw new AuthError('SIGNOUT_PENDING') }
  save(prefix + ':logoutPending', undefined)
}
export function continueOffline() { if (!session) save(prefix + ':localOnly', true); location.assign('/') }
export function chooseSignIn(continuation = 'home') { sessionStorage.setItem(prefix + ':continue', continuation); location.assign('/login') }
export function accountHeaders() { return session ? { 'X-Pinelink-CSRF': session.csrf } : {} }
let expiring = false
export function sessionExpired(expected: string) {
  if (!session || session.csrf !== expected || expiring) return
  expiring = true; session = undefined; emit()
  void closeClient().then(() => { authChannel?.postMessage('expired'); location.assign('/login?error=AUTH_EXPIRED') })
}
export const legacyIdentity = () => { IDENTITY.owner = import.meta.env.VITE_PINELINK_OWNER ?? 'dev-owner' }
const authChannel = typeof window.BroadcastChannel !== 'undefined' ? new window.BroadcastChannel(prefix) : undefined
authChannel?.addEventListener('message', event => {
  if (event.data === 'changing') void closeClient()
  else if (event.data === 'changed') {
    if (!guestWorkPending()) { location.reload(); return }
    // Updating account state must not replace the independent visitor worker.
    void authFetch<Session>('/v1/auth/session').then(value => { session = value; offlineAccount = value.account; save(prefix, value.account); emit() }).catch(() => { session = undefined; offlineAccount = undefined; emit() })
  } else if (event.data === 'expired') void closeClient().then(() => { session = undefined; emit(); if (!guestWorkPending()) location.assign('/login?error=AUTH_EXPIRED') })
})
