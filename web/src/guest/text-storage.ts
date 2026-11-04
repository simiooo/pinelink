import type { DeviceProfile } from '../data/crypto'
import { decodeText, decryptDare, openManifest } from './crypto'

// Only the ciphertext and the device-bound encrypted manifest are persisted.
// Callers must scope the record to the same service, account and device store.
export type StoredText = { cipher: Blob; manifest: string }
export async function readStoredText(profile: DeviceProfile, stored: StoredText) {
  const manifest = await openManifest(profile, stored.manifest)
  if (manifest.content_version !== 1 || manifest.content_kind !== 'TEXT' || !manifest.sensitivity) throw new Error('TEXT_MANIFEST_INVALID')
  const plain = await decryptDare(stored.cipher, manifest)
  return { value: decodeText(new Uint8Array(await plain.arrayBuffer())), sensitivity: manifest.sensitivity }
}
