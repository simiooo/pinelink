import { Blob as NodeBlob } from 'node:buffer'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { newProfile } from '../data/crypto'
import { encodePackage, identity, makePackage } from '../local/protocol'
import { GuestError } from './runtime'
import { joinConnection } from './automatic-connection'

const origin = 'https://pinelink.example', token = 'a'.repeat(22)
const sdp = 'v=0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=ice-ufrag:abc\r\na=ice-pwd:abcdefghijklmnopqrstuv\r\na=fingerprint:sha-256 ' + Array(32).fill('AB').join(':') + '\r\n'
const candidate = { id: 'candidate', invite_id: 'invite', device_name: 'Phone', device_id: 'other', incoming: false, status: 'WAITING', expires_at: '2099-01-01T00:00:00Z' }
const actions = () => ({ online: true, joinOnline: vi.fn().mockResolvedValue(candidate), joinLocal: vi.fn().mockResolvedValue(undefined) })
const invitation = async (server = origin, expires_at = '2099-01-01T00:00:00Z') => {
  const profile = await newProfile()
  return encodePackage(await makePackage(profile, identity(profile, 'Phone'), 'offer', crypto.randomUUID(), sdp, '', { origin: server, token, expires_at }))
}
beforeEach(() => { vi.stubGlobal('crypto', webcrypto); vi.stubGlobal('Blob', NodeBlob) })
afterEach(() => vi.unstubAllGlobals())

it('prefers the reachable service from a verified universal invitation', async () => {
  const a = actions()
  expect(await joinConnection(await invitation(), origin, a)).toEqual({ kind: 'online', candidate })
  expect(a.joinOnline).toHaveBeenCalledWith(token); expect(a.joinLocal).not.toHaveBeenCalled()
})
it('uses the same invitation locally when offline or the service cannot be reached', async () => {
  const code = await invitation()
  for (const reason of [undefined, new TypeError('Failed to fetch'), new DOMException('timeout', 'TimeoutError'), new GuestError('UNAVAILABLE', 503)]) {
    const a = actions(); a.online = !!reason
    if (reason) a.joinOnline.mockRejectedValue(reason)
    expect(await joinConnection(code, origin, a)).toEqual({ kind: 'local' })
    expect(a.joinLocal).toHaveBeenCalledWith(code)
    if (!reason) expect(a.joinOnline).not.toHaveBeenCalled()
  }
})
it('does not turn denial or a revoked invitation into a different connection', async () => {
  for (const status of [401, 403, 404, 409, 429]) {
    const a = actions(), reason = new GuestError('DENIED', status); a.joinOnline.mockRejectedValue(reason)
    await expect(joinConnection(await invitation(), origin, a)).rejects.toBe(reason)
    expect(a.joinLocal).not.toHaveBeenCalled()
  }
})
it('keeps services isolated and never sends a code to a supplied remote origin', async () => {
  const a = actions()
  await expect(joinConnection(await invitation('https://other.example'), origin, a)).rejects.toThrow('INVALID_REQUEST')
  expect(a.joinOnline).not.toHaveBeenCalled(); expect(a.joinLocal).not.toHaveBeenCalled()
})
it('requests the complete QR when a short resolver-only code cannot be resolved', async () => {
  const a = actions(); a.online = false
  await expect(joinConnection('ABCD EFGH', origin, a)).rejects.toThrow('CONNECTION_SCAN_REQUIRED')
  a.online = true; a.joinOnline.mockRejectedValue(new TypeError('Failed to fetch'))
  await expect(joinConnection('ABCD EFGH', origin, a)).rejects.toThrow('CONNECTION_SCAN_REQUIRED')
  expect(a.joinLocal).not.toHaveBeenCalled()
})
it('falls back locally after the signed resolver token expires without calling it', async () => {
  const a = actions(), code = await invitation(origin, '2020-01-01T00:00:00Z')
  await joinConnection(code, origin, a)
  expect(a.joinOnline).not.toHaveBeenCalled(); expect(a.joinLocal).toHaveBeenCalledWith(code)
})
