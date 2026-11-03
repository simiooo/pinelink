import 'fake-indexeddb/auto'
import Dexie from 'dexie'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ClientRuntime } from './runtime'
import { LocalStore } from './store'
import { observeCache } from './cache-observer'
import { newProfile } from './crypto'
import { runWorkerSync } from './worker-sync'
import { storeName } from './store'

const resources: ClientRuntime[] = []
beforeEach(() => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
  Object.defineProperty(navigator, 'locks', { value: undefined, configurable: true })
})
afterEach(async () => {
  const closing = resources.splice(0)
  for (const r of closing) await r.close()
  for (const [name] of new Map(closing.map(r => [r.store.name, r.store]))) await Dexie.delete(name)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
async function runtime(store = new LocalStore('offline-' + crypto.randomUUID())) {
  const r = await ClientRuntime.open('https://unit.invalid', 'owner', '', '', store)
  resources.push(r)
  await r.store.patchTarget({ bound: 'API_ONLY' })
  return r
}

it('cached running snapshots remain readable without pretending that network activity is fresh', async () => {
  const r = await runtime(), path = '/v1/tasks?owner_id=owner&include_deleted=false'
  const snapshot = { tasks: [{ id: 'running', state: 'RUNNING', version: 1, progress_bytes: 42, total_bytes: 100 }] }
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(snapshot))))
  await r.request('GET', path)
  expect(r.isNetworkFresh(path)).toBe(true)
  vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('offline') }))
  expect(await r.request('GET', path)).toEqual(snapshot)
  expect(r.isNetworkFresh(path)).toBe(false)
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(snapshot))))
  await r.request('GET', path)
  expect(r.isNetworkFresh(path)).toBe(true)
})

it('refreshes every resource despite a permanently failing first resource, including after reopening', async () => {
  const r = await runtime()
  const paths = Array.from({ length: 7 }, (_, i) => `/v1/files/${i}?owner_id=owner`)
  for (const path of paths) await r.store.cacheResponse(path, { version: 1 }, await r.store.beginRead())
  const fetcher = vi.fn(async (url: string) => {
    const index = paths.indexOf(new URL(url).pathname + new URL(url).search)
    if (index === 0) return new Response(JSON.stringify({ code: 'TEMPORARY' }), { status: 503 })
    if (index === 1) return new Response(JSON.stringify({ code: 'FILE_EXPIRED' }), { status: 410 })
    return new Response(JSON.stringify({ version: 2 }))
  })
  vi.stubGlobal('fetch', fetcher)
  await expect(r.sync(true)).rejects.toThrow('TEMPORARY')
  await expect(r.sync(true)).rejects.toThrow('TEMPORARY')
  for (const path of paths.slice(2)) expect((await r.store.responses.get(path))?.value).toEqual({ version: 2 })
  expect((await r.store.responses.get(paths[1]))?.invalid?.status).toBe(410)
  const name = r.store.name, root = r.profile.root
  await r.close()
  const reopened = await runtime(new LocalStore(name))
  expect(reopened.profile.root).toBe(root)
  Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
  await expect(reopened.request('GET', paths[1])).rejects.toMatchObject({ status: 410, code: 'FILE_EXPIRED' })
  expect(await reopened.request('GET', paths[6])).toEqual({ version: 2 })
})

it('does not use a known authorization denial as an offline cached success', async () => {
  const r = await runtime(), path = '/v1/files/private?owner_id=owner'
  await r.store.cacheResponse(path, { version: 1, file_name: 'Private.fig' }, await r.store.beginRead())
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ code: 'FORBIDDEN' }), { status: 403 })))
  await expect(r.request('GET', path)).rejects.toMatchObject({ status: 403 })
  Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
  await expect(r.request('GET', path)).rejects.toMatchObject({ status: 403 })
})

it('attempts a reachable local service even when the Internet hint says offline', async () => {
  const r = await runtime()
  Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ id: 'task', version: 2, state: 'PAUSED' })))
  vi.stubGlobal('fetch', fetcher)
  expect(await r.request('POST', '/v1/tasks/task/pause', { expected_version: 1 })).toMatchObject({ state: 'PAUSED' })
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(fetcher.mock.calls[0]).toBeDefined()
})

it('migrates an existing v1 cache and keeps its identity, cursor, and refresh eligibility', async () => {
  const name = 'migration-' + crypto.randomUUID(), old = new Dexie(name)
  old.version(1).stores({ profiles: 'id', targets: 'id', peers: 'root', inbox: 'id,[event.source_root+event.stream_id]', history: 'id,aggregate,source,kind', responses: 'path,updated' })
  const profile = await newProfile()
  await old.table('profiles').put(profile)
  await old.table('targets').put({ id: 'cloud', cursor: 42, bound: 'API_ONLY' })
  await old.table('responses').put({ path: '/v1/tasks', value: { tasks: [] }, updated: 7 })
  old.close()
  const r = await runtime(new LocalStore(name))
  expect(r.profile.root).toBe(profile.root)
  expect(r.profile.stream).toBe(profile.stream)
  expect((await r.store.target()).cursor).toBe(42)
  expect((await r.store.reserveRefreshes(4)).map(row => row.path)).toEqual(['/v1/tasks'])
  const root = r.profile.root
  await r.close()
  expect((await runtime(new LocalStore(name))).profile.root).toBe(root)
})

it('resumes a bounded background sweep after worker restarts without losing its identity or remaining resources', async () => {
  const configuration = { server: 'https://worker.invalid', owner: crypto.randomUUID(), device: '', pin: '' }
  const store = new LocalStore(await storeName(configuration.server, configuration.owner, ''))
  const r = await runtime(store)
  const paths = Array.from({ length: 14 }, (_, i) => `/v1/files/${i}`)
  for (const path of paths) await store.cacheResponse(path, { version: 1 }, await store.beginRead())
  const identity = r.profile.root
  await r.close()
  try {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ version: 2 }))))
  await expect(runWorkerSync(configuration)).rejects.toThrow('BACKGROUND_SYNC_MORE_PENDING')
  await store.open()
  const cutoff = (await store.target()).backgroundThrough
  expect(cutoff).toBeGreaterThan(0)
  store.close()
  await runWorkerSync(configuration)
  await store.open()
  expect((await store.profile()).root).toBe(identity)
  expect((await store.target()).backgroundThrough).toBe(0)
  for (const path of paths) expect((await store.responses.get(path))?.value).toEqual({ version: 2 })
  } finally { store.close() }
})

it('does not replace a missing identity when invoked as a background worker', async () => {
  const configuration = { server: 'https://worker.invalid', owner: crypto.randomUUID(), device: '', pin: '' }
  const name = await storeName(configuration.server, configuration.owner, ''), store = new LocalStore(name)
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  try {
    await runWorkerSync(configuration)
    expect(await store.profiles.count()).toBe(0)
    expect(fetcher).not.toHaveBeenCalled()
  } finally { store.close(); await Dexie.delete(name) }
})

it('primes unvisited page resources without presenting unfetched data as an offline success', async () => {
  const r = await runtime()
  await r.prime('personal')
  const quota = '/v1/quota?scope_id=personal'
  expect((await r.store.responses.get(quota))?.pending).toBe(1)
  Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
  vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Network unavailable') }))
  await expect(r.request('GET', quota)).rejects.toThrow('Network unavailable')
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ quota_bytes: 1000, used_bytes: 10 }))))
  await r.sync(true)
  await r.sync(true)
  const row = await r.store.responses.get(quota)
  expect(row?.pending).toBeUndefined()
  expect(await r.request('GET', quota)).toMatchObject({ quota_bytes: 1000 })
})

it('notifies semantic cache changes across connections without looping on identical refreshes', async () => {
  const r = await runtime(), other = new LocalStore(r.store.name), changed = vi.fn()
  const stop = observeCache(r.store, changed)
  try {
    // Give the asynchronous initial live query a chance to establish observation.
    await new Promise(resolve => setTimeout(resolve, 30))
    await other.cacheResponse('/v1/devices', { devices: [{ id: 'other', status: 'ACTIVE' }] }, await other.beginRead())
    await vi.waitFor(() => expect(changed).toHaveBeenCalledTimes(1))
    await other.cacheResponse('/v1/devices', { devices: [{ id: 'other', status: 'ACTIVE' }] }, await other.beginRead())
    await new Promise(resolve => setTimeout(resolve, 30))
    expect(changed).toHaveBeenCalledTimes(1)
    stop()
    await other.cacheResponse('/v1/devices', { devices: [] }, await other.beginRead())
    await new Promise(resolve => setTimeout(resolve, 30))
    expect(changed).toHaveBeenCalledTimes(1)
  } finally { stop(); other.close() }
})
