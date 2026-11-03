import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { Blob as NodeBlob } from 'node:buffer'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { GuestRuntime } from './runtime'
import { GuestStore } from './store'
import { LocalStore } from '../data/store'
import { digest, utf8 } from '../data/crypto'
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

it('upgrades a v3 visitor database without replacing keys, saved files, partial chunks or replication peers', async () => {
  const server = 'https://unit.invalid/upgrade-' + crypto.randomUUID(), name = 'pinelink-visitor:' + await digest(utf8.encode(server)), legacy = new LocalStore(name)
  legacy.version(3).stores({ leases: 'id', jobs: 'id,created,transfer.id,direction', chunks: '[job+index],job' })
  const profile = await legacy.profile()
  await legacy.table('jobs').put({ id: 'saved', created: 1, direction: 'receive', phase: 'complete', name: 'Saved.pdf', plain: new NodeBlob(['saved']), plainSize: 5, received: 5, peer: { pair_id: 'old-pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer', encryption_key: profile.encryptionKey, signing_key: profile.signingKey, expires_at: '2099-01-01T00:00:00Z' } })
  await legacy.table('chunks').put({ job: 'partial', index: 0, bytes: new NodeBlob(['partial']), hash: 'retained' })
  await legacy.peers.put({ root: 'authority', signing_key: 'sign', encryption_key: 'encrypt', revoked: false }); legacy.close()
  const upgraded = await GuestRuntime.open(server); resources.push(upgraded)
  expect(upgraded.profile.root).toBe(profile.root); expect(upgraded.profile.device).toBe(profile.device)
  expect((await upgraded.store.jobs.get('saved'))?.plain?.size).toBe(5)
  expect((await upgraded.store.chunks.get(['partial', 0]))?.bytes.size).toBe(7)
  expect(await upgraded.store.peers.get('authority')).toMatchObject({ revoked: false })
  expect(await upgraded.store.guestPeers.count()).toBe(0)
})

it('keeps peer aliases and the device roster across an offline restart without changing remote names', async () => {
  const r = await runtime(), peer = { pair_id: 'pair-local', owner_id: 'guest-peer', device_id: 'remote', device_name: 'Remote device', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: new Date(Date.now() + 86400000).toISOString() }
  await r.store.guestPeers.put(peer); await r.renamePeer(peer.pair_id, '  Design Mac  ')
  await expect(r.renamePeer(peer.pair_id, ' '.repeat(5))).rejects.toThrow('DEVICE_NAME_INVALID')
  const server = r.server; await r.close(); resources.splice(resources.indexOf(r), 1)
  Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
  const reopened = await GuestRuntime.open(server); resources.push(reopened)
  expect(reopened.getSnapshot().connections.peers[0]).toMatchObject({ device_name: 'Remote device', local_name: 'Design Mac' })
  await expect(reopened.removePeer(peer.pair_id)).rejects.toThrow('CONNECTION_OFFLINE')
  expect(await reopened.store.guestPeers.get(peer.pair_id)).toBeDefined()
})

it('a roster response started before removal cannot resurrect the removed connection', async () => {
  const r = await runtime(), entered = deferred<void>(), roster = deferred<Response>()
  const peer = { pair_id: 'pair-removed', owner_id: 'guest-peer', device_id: 'remote', device_name: 'Remote', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: new Date(Date.now() + 86400000).toISOString() }
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/challenge')) return challenge()
    if (url.endsWith('/enroll')) return response(enrollment(r))
    if (init?.method === 'DELETE') return response({ removed: true })
    if (url.endsWith('/connections')) { entered.resolve(); return roster.promise }
    if (url.endsWith('/transfers')) return response({ transfers: [] })
    throw Error('unexpected request')
  }))
  await r.ensureSession(); await r.store.guestPeers.put(peer); const polling = r.poll(); await entered.promise
  const removal = r.removePeer(peer.pair_id)
  await vi.waitFor(async () => expect(await r.store.peerRemovals.get(peer.pair_id)).toBeDefined())
  roster.resolve(response({ invites: [], candidates: [], peers: [peer] })); await Promise.all([polling, removal])
  expect(await r.store.guestPeers.get(peer.pair_id)).toBeUndefined(); expect(r.getSnapshot().connections.peers).toHaveLength(0)
})

it('distinguishes an expired file from an expired connection code', async () => {
  const r = await runtime()
  vi.stubGlobal('fetch', vi.fn(async (url: string) => url.endsWith('/challenge') ? challenge() : url.endsWith('/enroll') ? response(enrollment(r)) : new Response(JSON.stringify({ code: 'EXPIRED' }), { status: 410 })))
  await r.ensureSession()
  await expect(r.request('POST', '/v1/transfers/file/accept', {})).rejects.toThrow('FILE_EXPIRED')
  await expect(r.request('POST', '/v1/guest/join', { code: 'ABCDEFGH' })).rejects.toThrow(/^EXPIRED$/)
})

it('does not drop a resume clicked while the earlier pause is finishing its poll', async () => {
  const r = await runtime(), entered = deferred<void>(), read = deferred<Response>(), actions: string[] = []
  const transfer: GuestTransfer = { id: crypto.randomUUID(), state: 'ACCEPTED', version: 2, file_name: 'file.bin', size_bytes: 100, content_root: '0'.repeat(64), encryption_suite: 'DARE20-AGE-X25519-v1', sender_owner_id: 'guest-peer', sender_device_id: 'peer-device', recipient_owner_id: 'guest-fixture', recipient_device_id: r.profile.device, expires_at: new Date(Date.now() + 600000).toISOString(), received_bytes: 0 }
  const job: GuestJob = { id: transfer.id, created: Date.now(), direction: 'receive', name: 'file.bin', plainSize: 68, received: 0, phase: 'running', transfer, retryAt: Date.now() + 3600000, peer: { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: transfer.expires_at } }
  job.snapshot = { transfer_id: transfer.id, state: 'ACCEPTED', state_version: 2, control_revision: 1, progress_revision: 0, delivery: { state: 'RUNNING', acknowledged_bytes: 0, total_bytes: 100, revision: 0 }, spool: { state: 'OPEN', acknowledged_bytes: 0, total_bytes: 100 }, completion: { delivered: false, stored: false }, capabilities: { pause: true, resume: false, cancel: false, restore: false } }
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url.endsWith('/challenge')) return challenge()
    if (url.endsWith('/enroll')) return response(enrollment(r))
    if (url.endsWith('/connections')) { entered.resolve(); return read.promise }
    if (url.endsWith('/transfers')) return response({ transfers: [] })
    if (url.endsWith('/tasks')) return response({ tasks: [{ id: 'task', kind: 'DOWNLOAD', reference_id: transfer.id, version: actions.length + 1 }] })
    if (url.endsWith('/pause') || url.endsWith('/resume')) { actions.push(url.split('/').at(-1)!); return response({ version: actions.length + 1, state: url.endsWith('/pause') ? 'PAUSED' : 'RUNNING', progress_bytes: 50, total_bytes: 100, progress_revision: 1 }) }
    throw Error('unexpected request')
  }))
  await r.ensureSession(); await r.store.jobs.add(job)
  const paused = r.control(job.id, 'pause'); await entered.promise
  const resumed = r.control(job.id, 'resume')
  try { await vi.waitFor(() => expect(actions).toEqual(['pause', 'resume'])) }
  finally { read.resolve(response({ invites: [], candidates: [], peers: [] })) }
  await Promise.all([paused, resumed])
  expect((await r.store.jobs.get(job.id))?.phase).toBe('running')
  expect((await r.store.jobs.get(job.id))?.controlRevision).toBe(3)
})

it('publishes the committed pause byte boundary atomically and ignores an earlier poll', async () => {
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
    if (url.endsWith('/pause')) return response({ version: 2, state: 'PAUSED', progress_bytes: 50, total_bytes: 100, progress_revision: 1 })
    if (url.endsWith('/progress')) { entered.resolve(); return stale.promise }
    throw new Error('unexpected request ' + url)
  }))
  await r.ensureSession(); await r.store.jobs.add(job)
  const poll = r.poll(); await entered.promise
  const pause = r.control(job.id, 'pause')
  await vi.waitFor(async () => expect((await r.store.jobs.get(job.id))?.phase).toBe('paused'))
  expect((await r.store.jobs.get(job.id))?.snapshot?.delivery.acknowledged_bytes).toBe(50)
  stale.resolve(response(snapshot)); await Promise.all([poll, pause])
  expect(await r.store.jobs.get(job.id)).toMatchObject({ phase: 'paused', controlRevision: 2, transfer: { version: 2, state: 'ACCEPTED' } })
  expect((await r.store.jobs.get(job.id))?.snapshot?.delivery.acknowledged_bytes).toBe(50)
  expect(calls.some(url => url.endsWith('/relay-ticket'))).toBe(false)
})
it('late resume responses neither rewind newer acknowledgements nor revive a completed file', async () => {
  for (const complete of [false, true]) {
    const r = await runtime(), entered = deferred<void>(), late = deferred<Response>()
    const transfer: GuestTransfer = { id: crypto.randomUUID(), state: 'ACCEPTED', version: 2, file_name: 'file.bin', size_bytes: 100, content_root: '0'.repeat(64), encryption_suite: 'DARE20-AGE-X25519-v1', sender_owner_id: 'guest-peer', sender_device_id: 'peer-device', recipient_owner_id: 'guest-fixture', recipient_device_id: r.profile.device, expires_at: new Date(Date.now() + 600000).toISOString(), received_bytes: 50 }
    const snapshot: ProgressSnapshot = { transfer_id: transfer.id, state: 'ACCEPTED', state_version: 2, control_revision: 2, progress_revision: 1, delivery: { state: 'PAUSED', acknowledged_bytes: 50, total_bytes: 100, revision: 1 }, spool: { state: 'OPEN', acknowledged_bytes: 0, total_bytes: 100 }, completion: { delivered: false, stored: false }, capabilities: { pause: false, resume: true, cancel: false, restore: false } }
    const job: GuestJob = { id: transfer.id, created: Date.now(), direction: 'receive', name: 'file.bin', plainSize: 68, received: 50, phase: 'paused', transfer, snapshot, controlRevision: 2, retryAt: Date.now() + 3600000, peer: { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer', encryption_key: r.profile.encryptionKey, signing_key: r.profile.signingKey, expires_at: transfer.expires_at } }
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (url.endsWith('/challenge')) return challenge()
      if (url.endsWith('/enroll')) return response(enrollment(r))
      if (url.endsWith('/tasks')) return response({ tasks: [{ id: 'task', kind: 'DOWNLOAD', reference_id: transfer.id, version: 2 }] })
      if (url.endsWith('/resume')) { entered.resolve(); return late.promise }
      if (url.endsWith('/connections')) return response({ invites: [], candidates: [], peers: [] })
      if (url.endsWith('/transfers')) return response({ transfers: [] })
      throw Error('unexpected request')
    }))
    await r.ensureSession(); await r.store.jobs.add(job)
    const resume = r.control(job.id, 'resume'); await entered.promise
    if (complete) await r.store.jobs.update(job.id, { phase: 'complete', transfer: { ...transfer, state: 'COMPLETED', version: 3 } })
    else await r.store.jobs.update(job.id, { phase: 'running', controlRevision: 3, snapshot: { ...snapshot, control_revision: 3, progress_revision: 2, delivery: { ...snapshot.delivery, state: 'RUNNING', acknowledged_bytes: 75, revision: 2 } } })
    late.resolve(response({ version: 3, state: 'RUNNING', progress_bytes: 50, total_bytes: 100, progress_revision: 1 })); await resume
    expect((await r.store.jobs.get(job.id))?.phase).toBe(complete ? 'complete' : 'running')
    expect((await r.store.jobs.get(job.id))?.snapshot?.delivery.acknowledged_bytes).toBe(complete ? 50 : 75)
    if (complete) expect((await r.store.jobs.get(job.id))?.transfer?.state).toBe('COMPLETED')
  }
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
