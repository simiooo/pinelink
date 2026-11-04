import { Blob as NodeBlob, File as NodeFile } from 'node:buffer'
import { webcrypto } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { newProfile } from '../data/crypto'
import { decodeText, decryptDare, openManifest, prepareFile, suite, textByteLimit, validateManifest } from './crypto'
import { readStoredText } from './text-storage'

const base = { suite, data_key: btoa(String.fromCharCode(...new Uint8Array(32))), plaintext_root: 'a'.repeat(64), plaintext_size: 1, file_name: 'Text' }
afterEach(() => vi.unstubAllGlobals())
describe('versioned text payload boundaries', () => {
  it('accepts legacy files and explicitly marked text without treating a filename as a kind', () => {
    expect(() => validateManifest(base)).not.toThrow()
    expect(() => validateManifest({ ...base, content_version: 1, content_kind: 'TEXT', sensitivity: 'SENSITIVE' })).not.toThrow()
    for (const invalid of [{ content_kind: 'TEXT' }, { content_version: 2, content_kind: 'TEXT', sensitivity: 'NORMAL' }, { content_version: 1, content_kind: 'TEXT', sensitivity: 'OTHER' }, { content_version: 1, content_kind: 'FILE', sensitivity: 'SENSITIVE' }, { content_version: 1, content_kind: 'OTHER' }]) expect(() => validateManifest({ ...base, ...invalid })).toThrow('FILE_MANIFEST_INVALID')
  })
  it('preserves UTF-8 bytes and rejects empty, oversized and malformed text', () => {
    const body = '标题\n  保留空格  \n'; expect(decodeText(new TextEncoder().encode(body))).toBe(body)
    expect(decodeText(new Uint8Array([0xef, 0xbb, 0xbf, 0x61]))).toBe('\ufeffa')
    expect(decodeText(new Uint8Array(textByteLimit).fill(65))).toHaveLength(textByteLimit)
    expect(() => decodeText(new Uint8Array())).toThrow('TEXT_SIZE_INVALID')
    expect(() => decodeText(new Uint8Array(textByteLimit + 1))).toThrow('TEXT_SIZE_INVALID')
    expect(() => decodeText(new Uint8Array([0xc3, 0x28]))).toThrow('TEXT_UTF8_INVALID')
  })
  it('seals the type and sensitive flag with both device envelopes and preserves the exact text', async () => {
    vi.stubGlobal('crypto', webcrypto); vi.stubGlobal('Blob', NodeBlob); vi.stubGlobal('File', NodeFile)
    const sender = await newProfile(), receiver = await newProfile(), body = '标题\n  a & b  \n'
    const prepared = await prepareFile(new File([body], 'do-not-expose.txt'), { sender: sender.encryptionKey, receiver: receiver.encryptionKey }, { sensitivity: 'SENSITIVE' })
    expect(prepared.name).toBe('Text')
    const manifest = await openManifest(receiver, prepared.recipientManifest), own = await openManifest(sender, prepared.senderManifest)
    expect(manifest).toMatchObject({ content_version: 1, content_kind: 'TEXT', sensitivity: 'SENSITIVE', file_name: 'Text' })
    expect(own).toEqual(manifest)
    expect(decodeText(new Uint8Array(await (await decryptDare(prepared.cipher, manifest)).arrayBuffer()))).toBe(body)
    expect(await readStoredText(sender, { cipher: prepared.cipher, manifest: prepared.senderManifest })).toEqual({ value: body, sensitivity: 'SENSITIVE' })
    expect(await readStoredText(receiver, { cipher: prepared.cipher, manifest: prepared.recipientManifest })).toEqual({ value: body, sensitivity: 'SENSITIVE' })
    await expect(readStoredText(await newProfile(), { cipher: prepared.cipher, manifest: prepared.recipientManifest })).rejects.toThrow()
    await expect(readStoredText(receiver, { cipher: prepared.cipher.slice(0, -1), manifest: prepared.recipientManifest })).rejects.toThrow()
  })
})
