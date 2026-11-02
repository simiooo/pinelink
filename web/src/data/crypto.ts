import { Decrypter, identityToRecipient } from 'age-encryption'

export const utf8 = new TextEncoder()
export const base64 = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes))
export const unbase64 = (value: string) => Uint8Array.from(atob(value), c => c.charCodeAt(0))
export const hex = (bytes: Uint8Array) => Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
export const digest = async (bytes: Uint8Array) => hex(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)))
export const rootOf = (key: string) => digest(unbase64(key))
export const nonce = () => base64(crypto.getRandomValues(new Uint8Array(32))).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')

export function canonical(value: unknown): string {
  if (typeof value === 'number' && (!Number.isSafeInteger(value))) throw new Error('UNSAFE_PROTOCOL_NUMBER')
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']'
  if (value !== null && typeof value === 'object') {
    const object = value as Record<string, unknown>
    return '{' + Object.keys(object).sort().filter(k => object[k] !== undefined).map(k => canonical(k) + ':' + canonical(object[k])).join(',') + '}'
  }
  return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'))
}
export const proof = (domain: string, value: object) => utf8.encode(`pinelink-replication-${domain}-v1\n${canonical({ ...value, signature: '' })}`)
export async function verify(key: string, bytes: Uint8Array, signature: string) {
  try {
    const publicKey = await crypto.subtle.importKey('raw', unbase64(key), 'Ed25519', false, ['verify'])
    return await crypto.subtle.verify('Ed25519', publicKey, unbase64(signature), bytes)
  } catch { return false }
}

export type DeviceProfile = {
  id: string; root: string; device: string; stream: string; signingKey: string; encryptionKey: string
  signingPrivate: CryptoKey; encryptionPrivate: CryptoKey
}
export async function newProfile(device?: string): Promise<DeviceProfile> {
  const sign = await crypto.subtle.generateKey('Ed25519', false, ['sign', 'verify']) as CryptoKeyPair
  const age = await crypto.subtle.generateKey('X25519', false, ['deriveBits']) as CryptoKeyPair
  const signingKey = base64(new Uint8Array(await crypto.subtle.exportKey('raw', sign.publicKey)))
  return { id: 'self', root: await rootOf(signingKey), device: device || `web-${crypto.randomUUID()}`, stream: crypto.randomUUID(), signingKey,
    encryptionKey: await identityToRecipient(age.privateKey), signingPrivate: sign.privateKey, encryptionPrivate: age.privateKey }
}
export async function decrypt(profile: DeviceProfile, ciphertext: string): Promise<Record<string, unknown>> {
  const bytes = unbase64(ciphertext)
  if (bytes.length > 65536) throw new Error('HISTORY_TOO_LARGE')
  const d = new Decrypter()
  d.addIdentity(profile.encryptionPrivate)
  const plaintext = await d.decrypt(bytes)
  if (plaintext.length > 32768) throw new Error('HISTORY_TOO_LARGE')
  return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(plaintext)) as Record<string, unknown>
}
