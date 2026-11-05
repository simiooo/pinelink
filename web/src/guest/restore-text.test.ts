import 'fake-indexeddb/auto'
import { Blob as NodeBlob, File as NodeFile } from 'node:buffer'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { newProfile } from '../data/crypto'
import { prepareFile } from './crypto'
import { GuestStore } from './store'
import type { GuestJob } from './store'
import { restoreTextHistory } from './restore-text'
import { readStoredText } from './text-storage'

const stores: GuestStore[] = []
beforeEach(() => { vi.stubGlobal('crypto', webcrypto); vi.stubGlobal('Blob', NodeBlob); vi.stubGlobal('File', NodeFile) })
afterEach(async () => { for (const store of stores.splice(0)) await store.delete(); vi.unstubAllGlobals() })
const fixture = async () => { const store = new GuestStore('restore-' + crypto.randomUUID()); stores.push(store); return { store, profile: await store.profile(), local: await newProfile() } }
const job = (id: string) => ({ id, created: 1, direction: 'send', name: 'Text', plainSize: 5, phase: 'complete', received: 0, peer: { device_name: 'Phone' } }) as GuestJob

it('restores a missing type marker without changing the encrypted message, identity or completion', async () => {
  const { store, profile, local } = await fixture(), prepared = await prepareFile(new File(['# Secret'], 'Text'), { sender: profile.encryptionKey, receiver: local.encryptionKey }, { sensitivity: 'SENSITIVE' })
  await store.jobs.put({ ...job('old'), text: { cipher: prepared.cipher, manifest: prepared.senderManifest } })
  await restoreTextHistory(store, profile, local)
  const restored = (await store.jobs.get('old'))!
  expect(restored.payloadKind).toBe('TEXT'); expect(restored.phase).toBe('complete'); expect(restored.plain).toBeUndefined()
  expect(restored.text?.manifest).toBe(prepared.senderManifest)
  expect(await readStoredText(profile, restored.text!)).toEqual({ value: '# Secret', sensitivity: 'SENSITIVE' })
  expect((await store.profile()).root).toBe(profile.root)
})

it('uses the isolated local identity for a local message and keeps ordinary txt attachments as files', async () => {
  const { store, profile, local } = await fixture(), prepared = await prepareFile(new File(['local text'], 'Text'), { sender: local.encryptionKey, receiver: profile.encryptionKey }, { sensitivity: 'NORMAL' })
  await store.jobs.bulkPut([{ ...job('local'), authority: 'LOCAL_DEVICE', text: { cipher: prepared.cipher, manifest: prepared.senderManifest } }, { ...job('file'), name: 'message.txt', plain: new Blob(['ordinary attachment']) }])
  await restoreTextHistory(store, profile, local)
  expect((await store.jobs.get('local'))?.payloadKind).toBe('TEXT')
  expect((await store.jobs.get('file'))?.payloadKind).toBeUndefined()
  expect((await store.jobs.get('file'))?.plain?.size).toBe(19)
})

it('keeps unreadable text unavailable and continues repairing other records', async () => {
  const { store, profile, local } = await fixture(), prepared = await prepareFile(new File(['readable'], 'Text'), { sender: profile.encryptionKey, receiver: local.encryptionKey }, { sensitivity: 'NORMAL' })
  await store.jobs.bulkPut([{ ...job('broken'), text: { cipher: prepared.cipher, manifest: 'invalid' } }, { ...job('good'), text: { cipher: prepared.cipher, manifest: prepared.senderManifest } }])
  await restoreTextHistory(store, profile, local)
  expect((await store.jobs.get('broken'))?.error).toBeTruthy()
  expect((await store.jobs.get('broken'))?.payloadKind).toBe('TEXT')
  expect((await store.jobs.get('good'))?.error).toBeUndefined()
  expect((await store.jobs.get('good'))?.payloadKind).toBe('TEXT')
})
