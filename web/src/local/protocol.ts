import { Encrypter } from 'age-encryption'
import { base64, canonical, digest, nonce, rootOf, unbase64, utf8, verify } from '../data/crypto'
import type { DeviceProfile } from '../data/crypto'

export type LocalIdentity = { root: string; device: string; name: string; signing_key: string; encryption_key: string; capabilities?: string[] }
export type Signed<T> = T & { signature: string }
export type ConnectionPackage = Signed<{ v: 1; type: 'offer' | 'answer'; id: string; nonce: string; peer: LocalIdentity; sdp: string; reply: string }>
export type LocalReceipt = Signed<{ v: 1; id: string; sender: string; recipient: string; root: string; plaintext_root: string; plaintext_size: number; delivered_at: string }>
export type LocalState = Signed<{ id: string; sender: string; recipient: string; root: string; size: number; name: string; plain_size: number; content_kind?: 'TEXT'; state: 'PENDING' | 'ACCEPTED' | 'COMPLETED' | 'REJECTED' | 'CANCELLED' | 'EXPIRED' | 'FAILED'; version: number; revision: number; received: number; paused: boolean; receipt?: LocalReceipt }>
export const localID = (id: string) => 'local:' + id
export const isLocalID = (id: string) => id.startsWith('local:') && uuid(id.slice(6))
export const uuid = (id: unknown): id is string => typeof id === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(id)
export const proof = (domain: string, value: object) => utf8.encode('pinelink-local-browser-' + domain + '-v1\n' + canonical({ ...value, signature: '' }))
export const signed = async <T extends object>(profile: DeviceProfile, domain: string, value: T): Promise<Signed<T>> => ({ ...value, signature: base64(new Uint8Array(await crypto.subtle.sign('Ed25519', profile.signingPrivate, proof(domain, value)))) })
export const verified = (key: string, domain: string, value: { signature: string }) => verify(key, proof(domain, value), value.signature)
export const packageHash = (value: ConnectionPackage) => digest(utf8.encode(canonical(value)))
export const identity = (profile: DeviceProfile, name: string): LocalIdentity => ({ root: profile.root, device: profile.device, name, signing_key: profile.signingKey, encryption_key: profile.encryptionKey })
export const makePackage = (profile: DeviceProfile, peer: LocalIdentity, type: 'offer' | 'answer', id: string, sdp: string, reply = '') => signed(profile, 'connection', { v: 1 as const, type, id, nonce: nonce(), peer, sdp, reply })
// QR alphanumeric mode stores a signed SDP package more densely than mixed-case
// base64. The final colon protects an RFC 9285 trailing space from text trimming.
const alphabet='0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:'
function qrEncode(bytes:Uint8Array){let out='';for(let i=0;i<bytes.length;i+=2){let n=bytes[i]*256+(i+1<bytes.length?bytes[i+1]:0);if(i+1===bytes.length)n=bytes[i];out+=alphabet[n%45]+alphabet[Math.floor(n/45)%45];if(i+1<bytes.length)out+=alphabet[Math.floor(n/2025)]};return out}
function qrDecode(value:string){const out:number[]=[];if(value.length%3===1)throw new Error('LOCAL_CODE_INVALID');for(let i=0;i<value.length;i+=3){const count=Math.min(3,value.length-i),digits=[...value.slice(i,i+count)].map(c=>alphabet.indexOf(c));if(digits.some(d=>d<0))throw new Error('LOCAL_CODE_INVALID');const n=digits[0]+digits[1]*45+(count===3?digits[2]*2025:0);if(n>(count===3?65535:255))throw new Error('LOCAL_CODE_INVALID');if(count===3)out.push(n>>8,n&255);else out.push(n)}return new Uint8Array(out)}
export async function validateIdentity(value: LocalIdentity) {
  if (!value || typeof value.name !== 'string' || !value.name.trim() || value.name.length > 64 || /[\u0000-\u001f\u007f]/.test(value.name) || typeof value.device !== 'string' || value.device.length > 96 || !/^[a-f0-9]{64}$/.test(value.root) || typeof value.signing_key !== 'string' || unbase64(value.signing_key).length !== 32 || await rootOf(value.signing_key) !== value.root || typeof value.encryption_key !== 'string' || value.encryption_key.length > 128) throw new Error('LOCAL_CODE_INVALID')
  if (value.capabilities !== undefined && (!Array.isArray(value.capabilities) || value.capabilities.length > 16 || value.capabilities.some(capability => typeof capability !== 'string' || !/^[a-z0-9-]{1,32}$/.test(capability)) || new Set(value.capabilities).size !== value.capabilities.length)) throw new Error('LOCAL_CODE_INVALID')
  try { const e = new Encrypter(); e.addRecipient(value.encryption_key) } catch { throw new Error('LOCAL_CODE_INVALID') }
}
export async function validatePackage(value: ConnectionPackage) {
  if (!value || value.v !== 1 || !['offer', 'answer'].includes(value.type) || !uuid(value.id) || !/^[A-Za-z0-9_-]{43}$/.test(value.nonce) || typeof value.reply !== 'string' || (value.type === 'answer' ? !/^[a-f0-9]{64}$/.test(value.reply) : value.reply !== '') || typeof value.sdp !== 'string' || value.sdp.length > 16384 || value.sdp.split(/\r?\n/).filter(s => s.startsWith('m=')).length !== 1 || !/^m=application \d+ UDP\/DTLS\/SCTP webrtc-datachannel\r?$/m.test(value.sdp) || !/^a=fingerprint:sha-256 [0-9A-F:]+\r?$/im.test(value.sdp) || !/^a=ice-ufrag:.+$/m.test(value.sdp) || !/^a=ice-pwd:.+$/m.test(value.sdp)) throw new Error('LOCAL_CODE_INVALID')
  await validateIdentity(value.peer)
  if (!await verified(value.peer.signing_key, 'connection', value)) throw new Error('LOCAL_CODE_INVALID')
  return value
}
export async function encodePackage(value: ConnectionPackage) {
  const bytes = utf8.encode(JSON.stringify(value))
  const stream = new Blob([bytes]).stream().pipeThrough(new CompressionStream('deflate'))
  const compressed = new Uint8Array(await new Response(stream).arrayBuffer())
  if (compressed.length > 11996) throw new Error('LOCAL_CODE_INVALID')
  return 'PL2:' + qrEncode(compressed) + ':'
}
export async function decodePackage(code: string) {
  if (typeof code !== 'string' || code.length > 18000) throw new Error('LOCAL_CODE_INVALID')
  try {
    const value=code.trim(), bytes=value.startsWith('PL2:')&&value.endsWith(':')?qrDecode(value.slice(4,-1)):/^PL1\.[A-Za-z0-9_-]+$/.test(value)?unbase64(value.slice(4).replaceAll('-','+').replaceAll('_','/')):undefined
    if(!bytes)throw new Error('LOCAL_CODE_INVALID')
    const reader = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('deflate')).getReader()
    const parts: Uint8Array<ArrayBuffer>[] = []; let length = 0
    try { while (true) { const result = await reader.read(); if (result.done) break; length += result.value.length; if (length > 24000) throw new Error('LOCAL_CODE_INVALID'); parts.push(new Uint8Array(result.value)) } } finally { await reader.cancel().catch(() => undefined) }
    return await validatePackage(JSON.parse(await new Blob(parts).text()))
  } catch { throw new Error('LOCAL_CODE_INVALID') }
}
export async function comparison(offer: ConnectionPackage, answer: ConnectionPackage) {
  const hash = await digest(utf8.encode('pinelink-local-compare-v1\n' + await packageHash(offer) + '\n' + await packageHash(answer)))
  return (parseInt(hash.slice(0, 8), 16) % 1_000_000).toString().padStart(6, '0').replace(/^(...)/, '$1 ')
}
