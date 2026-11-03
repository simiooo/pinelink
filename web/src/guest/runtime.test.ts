import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { GuestRuntime } from './runtime'
import { GuestStore } from './store'
import type { GuestJob } from './store'
import type { GuestTransfer, ProgressSnapshot } from './types'

const resources: GuestRuntime[] = []
const deferred = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
const response = (body: unknown) => new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
beforeEach(() => { vi.stubGlobal('crypto', webcrypto); Object.defineProperty(navigator, 'onLine', { configurable: true, value: true }); Object.defineProperty(navigator, 'locks', { configurable: true, value: undefined }) })
afterEach(async () => { for (const runtime of resources.splice(0)) { await runtime.close(); await runtime.store.delete() }; vi.restoreAllMocks() })
const runtime = async () => { const r = await GuestRuntime.open('https://unit.invalid/' + crypto.randomUUID()); resources.push(r); return r }
const enrollment = (r: GuestRuntime) => ({ guest: { owner_id: 'guest-fixture', device_id: r.profile.device, device_name: 'Test device', expires_at: new Date(Date.now() + 86400000).toISOString() }, csrf: 'test-csrf' })
const challenge = () => response({ challenge: 'test-challenge', unix_time: Math.floor(Date.now() / 1000) })

it('keeps a committed pause and accepted transfer when an earlier poll finishes late', async () => {
  const r = await runtime(), entered = deferred<void>(), stale = deferred<Response>()
  const transfer: GuestTransfer = { id: crypto.randomUUID(), state: 'ACCEPTED', version: 2, file_name: 'file.bin', size_bytes: 100, content_root: '0'.repeat(64), encryption_suite: 'DARE20-AGE-X25519-v1', sender_owner_id: 'guest-peer', sender_device_id: 'peer-device', recipient_owner_id: 'guest-fixture', recipient_device_id: r.profile.device, expires_at: new Date(Date.now() + 600000).toISOString(), received_bytes: 0 }
  const snapshot: ProgressSnapshot = { transfer_id: transfer.id, state: 'ACCEPTED', state_version: 2, control_revision: 1, progress_revision: 0, delivery: { state: 'RUNNING', acknowledged_bytes: 0, total_bytes: 100, revision: 0 }, spool: { state: 'OPEN', acknowledged_bytes: 0, total_bytes: 100 }, completion: { delivered: false, stored: false }, capabilities: { pause: true, resume: false, cancel: false, restore: false } }
  const job: GuestJob = { id: transfer.id, created: Date.now(), direction: 'receive', name: 'file.bin', plainSize: 68, received: 0, phase: 'running', transfer, snapshot, peer: { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: transfer.expires_at } }
  const calls: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    calls.push(url)
    if (url.endsWith('/challenge')) return challenge()
    if (url.endsWith('/enroll')) return response(enrollment(r))
    if (url.endsWith('/connections')) return response({ invites: [], candidates: [], peers: [] })
    if (url.endsWith('/transfers')) return response({ transfers: [{ ...transfer, state: 'PENDING', version: 1 }] })
    if (url.endsWith('/tasks')) return response({ tasks: [{ id: 'task', kind: 'DOWNLOAD', reference_id: transfer.id, version: 1 }] })
    if (url.endsWith('/pause')) return response({ version: 2, state: 'PAUSED' })
    if (url.endsWith('/progress')) { entered.resolve(); return stale.promise }
    throw new Error('unexpected request ' + url)
  }))
  await r.ensureSession(); await r.store.jobs.add(job)
  const poll = r.poll(); await entered.promise
  const pause = r.control(job.id, 'pause')
  await vi.waitFor(async () => expect((await r.store.jobs.get(job.id))?.phase).toBe('paused'))
  stale.resolve(response(snapshot)); await Promise.all([poll, pause])
  expect(await r.store.jobs.get(job.id)).toMatchObject({ phase: 'paused', controlRevision: 2, transfer: { version: 2, state: 'ACCEPTED' } })
  expect(calls.some(url => url.endsWith('/relay-ticket'))).toBe(false)
})
it('single-flights enrollment and discards a response arriving after close', async () => {
  const r = await runtime(), entered = deferred<void>(), late = deferred<Response>(), name = r.store.name, root = r.profile.root
  let enrollments = 0
  vi.stubGlobal('fetch', vi.fn(async (url: string) => { if (url.endsWith('/challenge')) return challenge(); enrollments++; entered.resolve(); return late.promise }))
  const first = r.ensureSession(), second = r.ensureSession(); await entered.promise
  const closing = r.close(); late.resolve(response(enrollment(r)))
  await Promise.allSettled([first, second]); await closing
  expect(enrollments).toBe(1)
  const reopened = new GuestStore(name)
  try { expect((await reopened.profile()).root).toBe(root); expect(await reopened.leases.get('self')).toBeUndefined() } finally { reopened.close() }
})
it('refreshes a version advanced by cloud sealing, but never retries consent for changed content', async () => {
  const r = await runtime(), versions: number[] = [], transfer: GuestTransfer = { id: crypto.randomUUID(), state: 'PENDING', version: 1, file_name: 'file.bin', size_bytes: 100, content_root: '0'.repeat(64), encryption_suite: 'DARE20-AGE-X25519-v1', sender_owner_id: 'guest-peer', sender_device_id: 'peer-device', recipient_owner_id: 'guest-fixture', recipient_device_id: r.profile.device, expires_at: new Date(Date.now() + 600000).toISOString(), received_bytes: 0 }
  const job: GuestJob = { id: transfer.id, created: Date.now(), direction: 'receive', name: 'file.bin', plainSize: 68, received: 0, phase: 'waiting', transfer, peer: { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: transfer.expires_at } }
  let reads = 0, replaced = false
  vi.stubGlobal('fetch', vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/challenge')) return challenge()
    if (url.endsWith('/enroll')) return response(enrollment(r))
    if (url.endsWith('/reject')) { versions.push(JSON.parse(String(options?.body)).expected_version); return versions.length === 1 ? new Response(JSON.stringify({ code: 'STATE_CONFLICT' }), { status: 409 }) : response({ ...transfer, version: 4, state: 'REJECTED' }) }
    return response({ ...transfer, version: ++reads + 1, ...(replaced ? { content_root: '1'.repeat(64) } : {}) })
  }))
  await r.ensureSession(); await r.store.jobs.add(job); await r.reject(job.id)
  expect(versions).toEqual([2, 3]); expect((await r.store.jobs.get(job.id))?.phase).toBe('stopped')
  await r.store.jobs.put(job); replaced = true
  await expect(r.reject(job.id)).rejects.toThrow('FILE_JOB_CHANGED'); expect(versions).toEqual([2, 3])
})
