import { Encrypter } from 'age-encryption'
import { sha256 } from '@noble/hashes/sha2.js'
import { chacha20poly1305 } from '@noble/ciphers/chacha.js'
import { base64, decrypt, digest, hex, unbase64 } from '../data/crypto'
import type { DeviceProfile } from '../data/crypto'

export const suite = 'DARE20-AGE-X25519-v1'
export const wireChunkSize = 65536
export type FileManifest = { suite: string; data_key: string; plaintext_root: string; plaintext_size: number; file_name: string; content_version?: 1; content_kind?: 'FILE' | 'TEXT'; sensitivity?: 'NORMAL' | 'SENSITIVE' }
export const textByteLimit = 256 * 1024
export const cipherSize = (size: number) => size + Math.ceil(size / wireChunkSize) * 32
export const safeFileName = (name: string) => name.replace(/[\\/<>:"|?*\u0000-\u001f\u007f]/g, '_').replace(/^\.+$/, '_').slice(0, 180) || 'File'
export async function hashBlob(blob: Blob) {
  const hash = sha256.create()
  for (let offset = 0; offset < blob.size; offset += 1 << 20) hash.update(new Uint8Array(await blob.slice(offset, offset + (1 << 20)).arrayBuffer()))
  return hex(hash.digest())
}

// DARE 2.0 framing matches minio/sio v0.5.1. AEAD primitives come from
// WebCrypto and noble; Go interoperability vectors guard this wire boundary.
export async function encryptDare(input: Blob, key: Uint8Array, randomNonce = crypto.getRandomValues(new Uint8Array(12)), cipher = 0) {
  if (key.length !== 32 || randomNonce.length !== 12 || ![0, 1].includes(cipher)) throw new Error('FILE_KEY_INVALID')
  const aes = cipher === 0 ? await crypto.subtle.importKey('raw', key, 'AES-GCM', false, ['encrypt']) : undefined
  const parts: BlobPart[] = []
  for (let offset = 0, sequence = 0; offset < input.size; offset += wireChunkSize, sequence++) {
    const plain = new Uint8Array(await input.slice(offset, offset + wireChunkSize).arrayBuffer())
    const header = new Uint8Array(16), view = new DataView(header.buffer)
    header[0] = 0x20; header[1] = cipher; view.setUint16(2, plain.length - 1, true); header.set(randomNonce, 4)
    header[4] = (header[4] & 0x7f) | (offset + plain.length === input.size ? 0x80 : 0)
    const nonce = header.slice(4); new DataView(nonce.buffer).setUint32(8, new DataView(nonce.buffer).getUint32(8, true) ^ sequence, true)
    const encrypted = aes ? new Uint8Array(await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData: header.slice(0, 4) }, aes, plain)) : chacha20poly1305(key, nonce, header.slice(0, 4)).encrypt(plain)
    parts.push(header, encrypted)
  }
  return new Blob(parts, { type: 'application/octet-stream' })
}

export async function decryptDare(input: Blob, manifest: FileManifest) {
  validateManifest(manifest)
  if (input.size !== cipherSize(manifest.plaintext_size)) throw new Error('FILE_TRUNCATED')
  const key = unbase64(manifest.data_key), aes = await crypto.subtle.importKey('raw', key, 'AES-GCM', false, ['decrypt'])
  const hash = sha256.create(), parts: BlobPart[] = []
  let offset = 0, sequence = 0, finalized = input.size === 0, reference: Uint8Array | undefined, cipher = -1, size = 0
  while (offset < input.size) {
    if (finalized || input.size - offset <= 32) throw new Error('FILE_FRAMING_INVALID')
    const header = new Uint8Array(await input.slice(offset, offset + 16).arrayBuffer()), view = new DataView(header.buffer)
    const length = view.getUint16(2, true) + 1, final = (header[4] & 0x80) !== 0
    if (header[0] !== 0x20 || ![0, 1].includes(header[1]) || (!final && length !== wireChunkSize) || offset + length + 32 > input.size) throw new Error('FILE_FRAMING_INVALID')
    const normalized = header.slice(4); normalized[0] &= 0x7f
    if (!reference) { reference = normalized; cipher = header[1] }
    if (cipher !== header[1] || reference.some((value, index) => value !== normalized[index])) throw new Error('FILE_NONCE_CHANGED')
    const nonce = header.slice(4); new DataView(nonce.buffer).setUint32(8, new DataView(nonce.buffer).getUint32(8, true) ^ sequence, true)
    const encrypted = new Uint8Array(await input.slice(offset + 16, offset + length + 32).arrayBuffer())
    const plain = cipher === 0 ? new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, additionalData: header.slice(0, 4) }, aes, encrypted)) : chacha20poly1305(key, nonce, header.slice(0, 4)).decrypt(encrypted)
    hash.update(plain); parts.push(plain); size += plain.length; offset += length + 32; sequence++; finalized = final
  }
  if (!finalized || size !== manifest.plaintext_size || hex(hash.digest()) !== manifest.plaintext_root) throw new Error('FILE_CHECKSUM_MISMATCH')
  return new Blob(parts, { type: 'application/octet-stream' })
}
export function validateManifest(value: unknown): asserts value is FileManifest {
  if (!value || typeof value !== 'object') throw new Error('FILE_MANIFEST_INVALID')
  const v = value as FileManifest
  if (v.suite !== suite || typeof v.data_key !== 'string' || unbase64(v.data_key).length !== 32 || !/^[a-f0-9]{64}$/.test(v.plaintext_root) || !Number.isSafeInteger(v.plaintext_size) || v.plaintext_size < 0 || v.plaintext_size > (1 << 30) || typeof v.file_name !== 'string' || v.file_name.length > 1024) throw new Error('FILE_MANIFEST_INVALID')
  if (v.content_kind === undefined) {
    if (v.content_version !== undefined || v.sensitivity !== undefined) throw new Error('FILE_MANIFEST_INVALID')
  } else if (v.content_version !== 1 || (v.content_kind !== 'FILE' && v.content_kind !== 'TEXT') || (v.content_kind === 'TEXT' ? !['NORMAL', 'SENSITIVE'].includes(v.sensitivity ?? '') || v.plaintext_size === 0 || v.plaintext_size > textByteLimit : v.sensitivity !== undefined)) throw new Error('FILE_MANIFEST_INVALID')
}
export function decodeText(bytes: Uint8Array) {
  if (!bytes.length || bytes.length > textByteLimit) throw new Error('TEXT_SIZE_INVALID')
  try { return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes) } catch { throw new Error('TEXT_UTF8_INVALID') }
}
export async function sealManifest(recipient: string, manifest: FileManifest) {
  const encrypter = new Encrypter(); encrypter.addRecipient(recipient)
  return base64(await encrypter.encrypt(JSON.stringify(manifest)))
}
export async function openManifest(profile: DeviceProfile, envelope: string) {
  const manifest = await decrypt(profile, envelope); validateManifest(manifest); return manifest
}
export async function prepareFile(file: File, recipients: { sender: string; receiver: string }, text?: { sensitivity: 'NORMAL' | 'SENSITIVE' }) {
  const key = crypto.getRandomValues(new Uint8Array(32))
  if (text) {
    if (!file.size || file.size > textByteLimit) throw new Error('TEXT_SIZE_INVALID')
    decodeText(new Uint8Array(await file.arrayBuffer()))
  }
  const manifest: FileManifest = { suite, data_key: base64(key), plaintext_root: await hashBlob(file), plaintext_size: file.size, file_name: text ? 'Text' : safeFileName(file.name), ...(text ? { content_version: 1, content_kind: 'TEXT', sensitivity: text.sensitivity } : {}) }
  validateManifest(manifest)
  const cipher = await encryptDare(file, key)
  const [recipientManifest, senderManifest, root] = await Promise.all([sealManifest(recipients.receiver, manifest), sealManifest(recipients.sender, manifest), hashBlob(cipher)])
  return { cipher, root, recipientManifest, senderManifest, name: manifest.file_name, plainSize: file.size }
}
export async function encodeFileFrame(index: number, bytes: Uint8Array) {
  const frame = new Uint8Array(36 + bytes.length); new DataView(frame.buffer).setUint32(0, index)
  frame.set(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), 4); frame.set(bytes, 36); return frame
}
export async function decodeFileFrame(buffer: ArrayBuffer, size: number) {
  const frame = new Uint8Array(buffer)
  if (frame.length < 36 || frame.length > wireChunkSize + 36) throw new Error('FILE_FRAME_INVALID')
  const index = new DataView(frame.buffer).getUint32(0), bytes = frame.slice(36)
  if (index >= Math.max(1, Math.ceil(size / wireChunkSize)) || bytes.length !== Math.min(wireChunkSize, size - index * wireChunkSize) || hex(frame.slice(4, 36)) !== await digest(bytes)) throw new Error('FILE_FRAME_INVALID')
  return { index, bytes }
}
