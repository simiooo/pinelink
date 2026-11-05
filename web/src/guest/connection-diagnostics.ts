import { identityToRecipient } from 'age-encryption'
import { rootOf, unbase64, utf8 } from '../data/crypto'
import type { DeviceProfile } from '../data/crypto'

export type ConnectionChannel = 'visitor' | 'account'
export type ConnectionStage = 'session' | 'challenge' | 'enroll' | 'sign' | 'connections' | 'transfers' | 'progress' | 'request' | 'local-data' | 'initialization'
export type ConnectionContext = { channel: ConnectionChannel; stage: ConnectionStage; method?: string; path?: string }
export type ConnectionFault = { channel: ConnectionChannel; stage: ConnectionStage; code: string; kind: 'network' | 'credentials' | 'storage' | 'browser' | 'limit' | 'protocol' | 'local'; status?: number; method?: string; endpoint?: string; at: number }
const contexts = new WeakMap<object, ConnectionContext>()

export async function connectionStage<T>(context: ConnectionContext, task: () => Promise<T>): Promise<T> {
  try { return await task() }
  catch (error) {
    if (error && typeof error === 'object' && !contexts.has(error)) contexts.set(error, context)
    throw error
  }
}

function endpoint(path?: string) {
  const clean = path?.split(/[?#]/, 1)[0]
  if (!clean) return undefined
  if (['/v1/guest/challenge', '/v1/guest/enroll', '/v1/guest/connections', '/v1/transfers'].includes(clean)) return clean
  if (/^\/v1\/p2p\/[^/]+\/progress$/.test(clean)) return '/v1/p2p/:id/progress'
  return '/v1/other'
}

export function requestStage(path: string): ConnectionStage {
  return path === '/v1/guest/challenge' ? 'challenge' : path === '/v1/guest/enroll' ? 'enroll' : path === '/v1/guest/connections' ? 'connections' : path === '/v1/transfers' ? 'transfers' : /\/progress$/.test(path) ? 'progress' : 'request'
}

export function connectionFault(error: unknown, fallback: ConnectionContext): ConnectionFault {
  const context = error && typeof error === 'object' ? contexts.get(error) ?? fallback : fallback
  const value = error as { code?: unknown; status?: unknown; name?: unknown; message?: unknown; inner?: { name?: string } } | undefined
  const message = typeof value?.message === 'string' ? value.message : ''
  const name = value?.inner?.name ?? value?.name
  const status = typeof value?.status === 'number' && value.status >= 100 && value.status <= 599 ? value.status : undefined
  let code = 'LOCAL_SYNC_ERROR', kind: ConnectionFault['kind'] = 'local'
  // Only codes and allowlisted exception classes enter the report, never raw
  // messages, request bodies, identifiers, credentials, file names or stacks.
  if (typeof value?.code === 'string' && /^[A-Z][A-Z0-9_]{1,63}$/.test(value.code)) code = value.code
  else if (/^(GUEST_IDENTITY_CHANGED|GUEST_CLOSED|ACCOUNT_SIGN_IN_REQUIRED|BROWSER_CRYPTO_UNAVAILABLE|BROWSER_CONTEXT_UNAVAILABLE|UPDATE_IN_PROGRESS)$/.test(message)) code = message
  const field = message.match(/Cannot read (?:properties|property).*reading ['"]([a-zA-Z_]+)['"]/)?.[1]
  if (field && ['peer', 'owner_id', 'device_id', 'invites', 'peers', 'candidates', 'completion', 'delivery', 'spool', 'state', 'phase', 'transfer', 'root', 'content_kind', 'stored'].includes(field)) code = 'LOCAL_FIELD_' + field.toUpperCase()
  if (/AbortSignal\.(any|timeout).*not a function/.test(message)) { code = 'ABORT_SIGNAL_UNSUPPORTED'; kind = 'browser' }
  else if (name === 'QuotaExceededError') { code = 'STORAGE_FULL'; kind = 'storage' }
  else if (['DatabaseClosedError', 'VersionError', 'UpgradeError', 'InvalidStateError', 'SecurityError', 'DataCloneError', 'TransactionInactiveError', 'NotFoundError'].includes(String(name))) { code = String(name).replace(/([a-z])([A-Z])/g, '$1_$2').toUpperCase(); kind = 'storage' }
  else if (name === 'NotSupportedError' || code.startsWith('BROWSER_')) { code = code === 'LOCAL_SYNC_ERROR' ? 'CRYPTO_UNSUPPORTED' : code; kind = 'browser' }
  else if (status === 401 || status === 403 || /IDENTITY|SIGNATURE|DEVICE_UNAUTHORIZED|ACCOUNT_SIGN_IN_REQUIRED/.test(code) || context.stage === 'sign') { if (code === 'LOCAL_SYNC_ERROR') code = 'DEVICE_KEY_UNAVAILABLE'; kind = 'credentials' }
  else if (status === 429 || /LIMIT|RATE/.test(code)) kind = 'limit'
  else if (name === 'TimeoutError' || name === 'AbortError' || /Failed to fetch|NetworkError|Load failed/i.test(message)) { code = name === 'TimeoutError' ? 'REQUEST_TIMEOUT' : 'NETWORK_ERROR'; kind = 'network' }
  else if (status && status >= 500) kind = 'network'
  else if (status) kind = 'protocol'
  return { channel: context.channel, stage: context.stage, code, kind, ...(status ? { status } : {}), ...(context.method ? { method: /^(GET|POST|PUT|DELETE|PATCH|HEAD)$/.test(context.method) ? context.method : 'OTHER' } : {}), ...(context.path ? { endpoint: endpoint(context.path) } : {}), at: Date.now() }
}

export async function checkStoredKeys(profile: DeviceProfile) {
  let signing = 'unavailable', encryption = 'unavailable', root = 'unavailable'
  try {
    const bytes = utf8.encode('pinelink-local-key-check-v1')
    const signature = await crypto.subtle.sign('Ed25519', profile.signingPrivate, bytes)
    const key = await crypto.subtle.importKey('raw', unbase64(profile.signingKey), 'Ed25519', false, ['verify'])
    signing = await crypto.subtle.verify('Ed25519', key, signature, bytes) ? 'match' : 'mismatch'
  } catch { /* Only the outcome is returned. Private keys stay inside Web Crypto. */ }
  try { root = await rootOf(profile.signingKey) === profile.root ? 'match' : 'mismatch' } catch { /* A damaged public descriptor stays distinguishable. */ }
  try { encryption = await identityToRecipient(profile.encryptionPrivate) === profile.encryptionKey ? 'match' : 'mismatch' } catch { /* Preserve unreadable identities for diagnosis. */ }
  return { signing, encryption, root, privateSigningKey: profile.signingPrivate?.type === 'private', signingAlgorithm: profile.signingPrivate?.algorithm?.name === 'Ed25519', signingUsage: profile.signingPrivate?.usages?.includes('sign') === true }
}

export function browserDiagnostics() {
  return { browser: navigator.userAgent.match(/(?:Chrome|CriOS)\/([0-9.]+)/)?.[1] ?? 'other', online: navigator.onLine, secure: globalThis.isSecureContext !== false, subtleCrypto: !!globalThis.crypto?.subtle, abortAny: typeof AbortSignal.any === 'function', abortTimeout: typeof AbortSignal.timeout === 'function', webLocks: !!navigator.locks, visible: !document.hidden }
}

export function connectionReason(fault: ConnectionFault | undefined, zh: boolean) {
  const kind = fault?.kind ?? 'network'
  const labels = zh ? { network: '连接暂不可用', credentials: '连接身份校验失败', storage: '本机存储异常', browser: '浏览器能力不可用', limit: '连接请求过于频繁', protocol: '服务响应异常', local: '本机同步异常' } : { network: 'Connection unavailable', credentials: 'Check device identity', storage: 'Check browser storage', browser: 'Browser capability unavailable', limit: 'Too many connection requests', protocol: 'Invalid service response', local: 'Local sync needs attention' }
  return labels[kind]
}
