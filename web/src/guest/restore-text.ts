import type { DeviceProfile } from '../data/crypto'
import { hashBlob, openManifest, prepareFile } from './crypto'
import { readStoredText } from './text-storage'
import type { GuestStore } from './store'

// Recover type only from a text record or its authenticated manifest. A .txt
// filename alone never turns a file attachment into a message.
export async function restoreTextHistory(store: GuestStore, profile: DeviceProfile, localProfile: DeviceProfile) {
  for (const job of await store.jobs.toArray()) {
    if (job.payloadKind === 'TEXT' && job.text && !job.plain) continue
    if (!job.text && job.payloadKind !== 'TEXT' && job.transfer?.content_kind !== 'TEXT' && job.local?.state?.content_kind !== 'TEXT') continue
    const reader = job.authority === 'LOCAL_DEVICE' ? localProfile : profile
    try {
      let text = job.text
      const envelope = text?.manifest ?? job.prepared?.senderManifest ?? (job.direction === 'send' ? job.transfer?.sender_encrypted_manifest : job.local?.manifest ?? job.transfer?.encrypted_manifest)
      if (!envelope) throw new Error('TEXT_MANIFEST_INVALID')
      const manifest = await openManifest(reader, envelope)
      if (manifest.content_kind !== 'TEXT' || !manifest.sensitivity) throw new Error('TEXT_MANIFEST_INVALID')
      if (!text) {
        let cipher = job.prepared?.cipher
        if (!cipher && job.direction === 'receive') cipher = await store.receivedCipher(job).catch(() => undefined)
        if (cipher) text = { cipher, manifest: envelope }
        else if (job.plain && job.plain.size === manifest.plaintext_size && await hashBlob(job.plain) === manifest.plaintext_root) {
          const prepared = await prepareFile(new File([job.plain], 'Text'), { sender: reader.encryptionKey, receiver: reader.encryptionKey }, { sensitivity: manifest.sensitivity })
          text = { cipher: prepared.cipher, manifest: prepared.senderManifest }
        }
      }
      if (!text) throw new Error('TEXT_HISTORY_UNAVAILABLE')
      await readStoredText(reader, text)
      await store.jobs.update(job.id, { payloadKind: 'TEXT', text, plain: undefined, error: undefined })
    } catch (error) {
      await store.jobs.update(job.id, { payloadKind: 'TEXT', error: error instanceof Error ? error.message : 'TEXT_HISTORY_UNAVAILABLE' })
    }
  }
}
