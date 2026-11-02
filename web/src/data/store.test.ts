import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import { LocalStore } from './store'
import { canonical } from './crypto'
import type { DeviceProfile } from './crypto'
import { preparePage } from './wire'
import { ClientRuntime, DataError } from './runtime'
import type { Page } from './wire'
import type { Peer } from './store'

const stores: LocalStore[] = []
const fixtureBin = resolve('../.artifacts/historyfixture' + (process.platform === 'win32' ? '.exe' : ''))
beforeAll(() => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  execFileSync('go', ['build', '-o', fixtureBin, './cmd/historyfixture'], { cwd: resolve('../cli'), timeout: 120000 })
}, 120000)
afterEach(async () => { for (const store of stores.splice(0)) await store.delete() })
const database = () => { const d = new LocalStore('test-' + crypto.randomUUID()); stores.push(d); return d }

async function vector(profile: DeviceProfile) {
  const output = execFileSync(fixtureBin, { input: JSON.stringify({ root: profile.root, signing_key: profile.signingKey, encryption_key: profile.encryptionKey }), encoding: 'utf-8', timeout: 10000 })
  return { profile, ...JSON.parse(output) as { pin: string; peer: Omit<Peer, 'revoked'>; page: Page; unsafe_page: Page; completion_page: Page; bad_receipt_page: Page } }
}

describe('persistent signed replicas', () => {
  it('reopens the same nonextractable identity and consumes real Go age/signature vectors', async () => {
    const d = database()
    const profile = await d.profile()
    expect(profile.signingPrivate.extractable).toBe(false)
    expect(profile.encryptionPrivate.extractable).toBe(false)
    d.close()
    await d.open()
    expect((await d.profile()).root).toBe(profile.root)
    const v = await vector(profile)
    await d.peers.put({ ...v.peer, revoked: false })
    const peers = new Map((await d.peers.toArray()).map(p => [p.root, p]))
    const rows = await preparePage(v.profile, v.page, v.pin, peers)
    expect(rows[0].fact?.file_name).toBe('本地 <>&.fig')
    await d.apply(v.profile, v.page, rows)
    await d.apply(v.profile, v.page, rows)
    expect(await d.history.count()).toBe(2)
    expect((await d.target()).cursor).toBe(2)
    expect(canonical(rows[1].fact)).toContain('\\u003c')
    await expect(preparePage(v.profile,v.bad_receipt_page,v.pin,peers)).rejects.toThrow()
    await d.apply(v.profile,v.completion_page,await preparePage(v.profile,v.completion_page,v.pin,peers))
    expect((await d.history.where('kind').equals('TRANSFER_CHANGED').first())?.fact.state).toBe('COMPLETED')
  })
  it('rolls back inbox, facts, and cursor together and never rounds unsafe counters', async () => {
    const d = database(), v = await vector(await d.profile())
    await d.peers.put({ ...v.peer, revoked: false })
    const rows = await preparePage(v.profile, v.page, v.pin, new Map([[v.peer.root, { ...v.peer, revoked: false }]]))
    const fail = () => { throw new Error('injected cursor failure') }
    d.targets.hook('creating', fail)
    await expect(d.apply(v.profile, v.page, rows)).rejects.toThrow('injected cursor failure')
    d.targets.hook('creating').unsubscribe(fail)
    expect(await d.inbox.count()).toBe(0)
    expect(await d.history.count()).toBe(0)
    expect((await d.target()).cursor).toBe(0)
    await expect(preparePage(v.profile, v.unsafe_page, v.pin, new Map())).rejects.toThrow()
    await d.apply(v.profile, v.page, rows)
    d.close(); await d.open()
    expect((await d.target()).cursor).toBe(2)
  })
  it('checks trust again at commit and can reapply ignored data after confirmation', async () => {
    const d = database(), v = await vector(await d.profile())
    const trusted = { ...v.peer, revoked: false }
    await d.peers.put(trusted)
    const rows = await preparePage(v.profile, v.page, v.pin, new Map([[v.peer.root, trusted]]))
    await d.peers.put({ ...trusted, revoked: true })
    await d.apply(v.profile, v.page, rows)
    expect(await d.history.count()).toBe(1)
    await d.peers.put(trusted)
    await d.apply(v.profile, v.page, rows)
    expect(await d.history.count()).toBe(2)
    const corrupt = structuredClone(v.page); corrupt.items[0].event.ciphertext += 'A'
    await expect(preparePage(v.profile, corrupt, v.pin, new Map([[v.peer.root, trusted]]))).rejects.toThrow()
  })
  it('reads cached history offline but propagates explicit authorization failures', async () => {
    const d = database()
    const runtime = await ClientRuntime.open('https://unit.invalid', 'owner', '', '', d)
    await d.patchTarget({ bound: 'API_ONLY' })
    const path = '/v1/tasks?include_deleted=true'
    await d.cacheResponse(path, { tasks: [{ id: 'task', state: 'COMPLETED', version: 3 }] }, 10)
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    expect((await runtime.request<{ tasks: object[] }>('GET', path)).tasks).toHaveLength(1)
    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    const fetcher = globalThis.fetch
    globalThis.fetch = async () => new Response(JSON.stringify({ code: 'UNAUTHORIZED' }), { status: 401 })
    await expect(runtime.request('GET', path)).rejects.toBeInstanceOf(DataError)
    globalThis.fetch = fetcher
    await runtime.close()
  })
  it('persists successful controls and refuses late snapshots that would undo them', async () => {
    const d = database(), path = '/v1/tasks?include_deleted=true'
    await d.cacheResponse(path, { tasks: [{ id: 'task', state: 'RUNNING', version: 1 }] }, 10)
    await d.cacheMutation({ id: 'task', state: 'PAUSED', version: 2 })
    await d.cacheResponse(path, { tasks: [{ id: 'task', state: 'RUNNING', version: 1 }] }, 11)
    d.close(); await d.open()
    expect((await d.responses.get(path))?.value).toEqual({ tasks: [{ id: 'task', state: 'PAUSED', version: 2 }] })
  })
})
