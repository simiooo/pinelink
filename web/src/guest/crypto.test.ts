import { Blob as NodeBlob, File as NodeFile } from 'node:buffer'
import { webcrypto } from 'node:crypto'
import { execFileSync, spawn } from 'node:child_process'
import { resolve } from 'node:path'
import { createInterface } from 'node:readline'
import { beforeAll, beforeEach, expect, it, vi } from 'vitest'
import { newProfile, unbase64, base64, hex } from '../data/crypto'
import { cipherSize, decodeFileFrame, decryptDare, encodeFileFrame, encryptDare, hashBlob, openManifest, prepareFile, safeFileName } from './crypto'

const bin = resolve('../.artifacts/guest-filefixture')
beforeAll(() => { execFileSync('go', ['build', '-o', bin, './cmd/historyfixture'], { cwd: resolve('../cli'), timeout: 120000 }) }, 120000)
beforeEach(() => { vi.stubGlobal('crypto', webcrypto); vi.stubGlobal('Blob', NodeBlob); vi.stubGlobal('File', NodeFile) })
it('matches Go DARE 2.0 across both ciphers, packet boundaries, and age envelopes in both directions', async () => {
  const profile = await newProfile(), child = spawn(bin, [], { stdio: ['pipe', 'pipe', 'pipe'] }), lines = createInterface({ input: child.stdout })
  const iterator = lines[Symbol.asyncIterator](), stderr: string[] = []
  child.stderr.on('data', value => stderr.push(String(value)))
  try {
    child.stdin.write(JSON.stringify({ root: profile.root, signing_key: profile.signingKey, encryption_key: profile.encryptionKey }) + '\n')
    const first = await iterator.next(); if (!first.value) throw new Error(stderr.join(''))
    const data = JSON.parse(first.value) as { file_vectors: { size: number; cipher: number; root: string }[]; file_cipher: string; file_manifest: string; peer: { encryption_key: string } }
    expect(data.file_vectors).toHaveLength(14)
    const key = Uint8Array.from({ length: 32 }, (_, index) => index), nonce = Uint8Array.from({ length: 12 }, (_, index) => index)
    for (const vector of data.file_vectors) {
      const bytes = Uint8Array.from({ length: vector.size }, (_, index) => (index * 29 + 7) & 255)
      const cipher = await encryptDare(new Blob([bytes]), key, nonce, vector.cipher)
      expect(cipher.size).toBe(cipherSize(vector.size)); expect(await hashBlob(cipher)).toBe(vector.root)
      const plain = await decryptDare(cipher, { suite: 'DARE20-AGE-X25519-v1', data_key: base64(key), plaintext_root: await hashBlob(new Blob([bytes])), plaintext_size: vector.size, file_name: 'fixture.bin' })
      expect(await hashBlob(plain)).toBe(await hashBlob(new Blob([bytes])))
    }
    const manifest = await openManifest(profile, data.file_manifest), file = await decryptDare(new Blob([unbase64(data.file_cipher)]), manifest)
    expect(await file.text()).toBe('Pinelink browser / Go interoperability\n')
    const prepared = await prepareFile(new File([new Uint8Array(131073).fill(19)], '浏览器 <>&.bin'), { sender: profile.encryptionKey, receiver: data.peer.encryption_key })
    child.stdin.end(JSON.stringify({ manifest: prepared.recipientManifest, cipher: Buffer.from(await prepared.cipher.arrayBuffer()).toString('base64') }) + '\n')
    const second = await iterator.next(); if (!second.value) throw new Error(stderr.join(''))
    expect(JSON.parse(second.value)).toMatchObject({ verified: true, size: 131073 })
    const ownManifest = await openManifest(profile, prepared.senderManifest); expect(ownManifest.file_name).toBe('浏览器 __&.bin')
  } finally { lines.close(); child.kill() }
}, 30000)
it('rejects truncation, final-bit changes, forged empty files and changed wire frames', async () => {
  const profile = await newProfile(), prepared = await prepareFile(new File(['a'.repeat(65537)], 'input.bin'), { sender: profile.encryptionKey, receiver: profile.encryptionKey }), manifest = await openManifest(profile, prepared.recipientManifest)
  await expect(decryptDare(prepared.cipher.slice(0, -1), manifest)).rejects.toThrow()
  const changed = new Uint8Array(await prepared.cipher.arrayBuffer()); changed[4] ^= 0x80
  await expect(decryptDare(new Blob([changed]), manifest)).rejects.toThrow()
  await expect(decryptDare(new Blob([]), manifest)).rejects.toThrow()
  const frame = await encodeFileFrame(0, new Uint8Array([1, 2, 3]))
  expect(hex((await decodeFileFrame(frame.buffer, 3)).bytes)).toBe('010203')
  frame[36] ^= 1; await expect(decodeFileFrame(frame.buffer, 3)).rejects.toThrow('FILE_FRAME_INVALID')
  expect(safeFileName('../secret/file\n.txt')).toBe('.._secret_file_.txt')
})
