import { webcrypto } from 'node:crypto'
import { afterEach, expect, it, vi } from 'vitest'
import { decodeFileFrame, encodeFileFrame, wireChunkSize } from './crypto'
import { FrameAssembler, fragmentFrame } from './direct'

vi.stubGlobal('crypto', webcrypto)
afterEach(() => vi.restoreAllMocks())

it('reassembles bounded SCTP packets into the original authenticated full and final file frames', async () => {
  const size = wireChunkSize + 17, assembler = new FrameAssembler(size)
  for (const [index, length] of [[0, wireChunkSize], [1, 17]]) {
    const bytes = new Uint8Array(length).fill(index + 17), frame = await encodeFileFrame(index, bytes), parts = fragmentFrame(frame)
    expect(parts.every(part => part.length <= 16384)).toBe(true)
    let complete: ArrayBuffer | undefined
    for (const part of parts) complete = assembler.push(part.buffer)
    expect(complete).toEqual(frame.buffer)
    expect((await decodeFileFrame(complete!, size)).bytes).toEqual(bytes)
  }
})
it('transmits a real empty frame rather than assuming zero bytes means a receipt', async () => {
  const frame = await encodeFileFrame(0, new Uint8Array()), assembler = new FrameAssembler(0)
  const result = assembler.push(fragmentFrame(frame)[0].buffer)
  expect((await decodeFileFrame(result!, 0)).bytes.byteLength).toBe(0)
  expect(() => assembler.push(fragmentFrame(frame)[0].buffer)).toThrow('FILE_FRAME_INVALID')
})
it.each(['offset', 'overlap', 'index', 'too-large', 'inner-index'])('rejects malformed %s fragments without growing an unbounded queue', async kind => {
  const frame = await encodeFileFrame(0, new Uint8Array(wireChunkSize)), parts = fragmentFrame(frame), assembler = new FrameAssembler(wireChunkSize)
  if (kind === 'offset') new DataView(parts[0].buffer).setUint32(4, 1)
  if (kind === 'index') new DataView(parts[0].buffer).setUint32(0, 1)
  if (kind === 'too-large') parts[0] = new Uint8Array(16385)
  if (kind === 'inner-index') new DataView(parts[0].buffer).setUint32(8, 1)
  expect(() => {
    for (const part of parts) { assembler.push(part.buffer); if (kind === 'overlap') assembler.push(part.buffer) }
  }).toThrow('FILE_FRAME_INVALID')
})
