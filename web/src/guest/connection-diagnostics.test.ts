import { webcrypto } from 'node:crypto'
import { beforeEach, expect, it, vi } from 'vitest'
import { newProfile } from '../data/crypto'
import { checkStoredKeys, connectionFault, connectionStage, requestStage } from './connection-diagnostics'

beforeEach(() => { vi.stubGlobal('crypto', webcrypto) })
it('keeps the first failing stage without changing the thrown error or copying its private details', async () => {
  const error = new TypeError('private-cookie secret@example.test filename-sensitive.txt')
  await expect(connectionStage({ channel: 'visitor', stage: 'connections', path: '/v1/guest/connections' }, () => connectionStage({ channel: 'visitor', stage: 'sign', method: 'GET', path: '/v1/guest/connections' }, async () => { throw error }))).rejects.toBe(error)
  const fault = connectionFault(error, { channel: 'account', stage: 'session' })
  expect(fault).toMatchObject({ channel: 'visitor', stage: 'sign', code: 'DEVICE_KEY_UNAVAILABLE', kind: 'credentials', method: 'GET', endpoint: '/v1/guest/connections' })
  expect(JSON.stringify(fault)).not.toMatch(/private-cookie|secret@example|sensitive\.txt/)
})
it('reports HTTP status and endpoint templates without transfer identifiers or query tokens', () => {
  const error = { code: 'DEVICE_UNAUTHORIZED', status: 401, message: 'token-private' }
  const fault = connectionFault(error, { channel: 'account', stage: 'progress', method: 'GET', path: '/v1/p2p/private-record/progress?token=private-token' })
  expect(fault).toMatchObject({ kind: 'credentials', code: 'DEVICE_UNAUTHORIZED', status: 401, endpoint: '/v1/p2p/:id/progress' })
  expect(JSON.stringify(fault)).not.toMatch(/private-record|private-token|token-private/)
})
it('distinguishes storage and missing browser APIs from a network failure', () => {
  const context = { channel: 'visitor', stage: 'connections' } as const
  expect(connectionFault(new DOMException('sensitive message', 'QuotaExceededError'), context)).toMatchObject({ code: 'STORAGE_FULL', kind: 'storage' })
  expect(connectionFault(new TypeError('AbortSignal.any is not a function'), context)).toMatchObject({ code: 'ABORT_SIGNAL_UNSUPPORTED', kind: 'browser' })
  expect(connectionFault(new TypeError('Failed to fetch'), context)).toMatchObject({ code: 'NETWORK_ERROR', kind: 'network' })
  expect(connectionFault(new TypeError("Cannot read properties of undefined (reading 'owner_id')"), context)).toMatchObject({ code: 'LOCAL_FIELD_OWNER_ID', kind: 'local' })
  expect(requestStage('/v1/guest/enroll')).toBe('enroll')
})
it('checks stored keys in place and distinguishes a mismatched private key without replacing it', async () => {
  const a = await newProfile(), b = await newProfile(), key = a.signingPrivate
  expect(await checkStoredKeys(a)).toMatchObject({ signing: 'match', encryption: 'match', root: 'match', privateSigningKey: true, signingAlgorithm: true, signingUsage: true })
  expect(await checkStoredKeys({ ...a, signingPrivate: b.signingPrivate })).toMatchObject({ signing: 'mismatch', encryption: 'match', root: 'match' })
  const broken = await checkStoredKeys({ ...a, signingPrivate: undefined! })
  expect(broken).toMatchObject({ signing: 'unavailable', root: 'match', privateSigningKey: false })
  expect(a.signingPrivate).toBe(key)
  expect(JSON.stringify(broken)).not.toContain(a.signingKey)
})
