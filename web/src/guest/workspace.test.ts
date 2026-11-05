import { afterEach, expect, it, vi } from 'vitest'
import type { ClientRuntime } from '../data/runtime'
import type { GuestJob } from './store'
import type { GuestPeer } from './types'

const auth = vi.hoisted(() => ({ session: undefined as unknown, listeners: new Set<() => void>() }))
vi.mock('../auth/client', () => ({ authState: () => ({ session: auth.session }), subscribeAuth: (listener: () => void) => { auth.listeners.add(listener); return () => auth.listeners.delete(listener) } }))
import { GuestRuntime } from './runtime'
import { WorkspaceRuntime } from './workspace'
import { safeToUpdate } from '../pwa/guard'

const peer = (id: string): GuestPeer => ({ pair_id: id, owner_id: id, device_id: id, device_name: id, encryption_key: 'key', signing_key: 'key', expires_at: '2099-01-01T00:00:00Z', online: true })
const job = (id: string): GuestJob => ({ id, created: 1, direction: 'send', name: id, plainSize: 1, received: 0, phase: 'waiting', peer: peer(id) })
const fixture = (id: string) => {
  const listeners = new Set<() => void>()
  const state = { ready: true, connected: true, busy: false, offline: false, checkedAt: Date.now(), jobs: [job(id)], connections: { invites: [], candidates: [], peers: [peer(id)] }, invite: { id, code: 'ABCD1234', expires_at: '2099-01-01T00:00:00Z', consumed: false } }
  return { state, runtime: { profile: { device: id }, local: { close: vi.fn() }, getSnapshot: () => state, now: () => Date.now(), subscribe: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn) }, connect: vi.fn(async () => state.invite), join: vi.fn(), send: vi.fn(async () => id), accept: vi.fn(async () => {}), reject: vi.fn(async () => {}), control: vi.fn(async () => {}), retry: vi.fn(async () => {}), dismiss: vi.fn(async () => {}), download: vi.fn(async () => {}), renamePeer: vi.fn(async () => {}), removePeer: vi.fn(async () => {}), respondCandidate: vi.fn(async () => {}), ensureSession: vi.fn(async () => {}), reconnect: vi.fn(async () => {}), poll: vi.fn(async () => {}), close: vi.fn(async () => {}) } }
}
const reader = { server: 'https://unit.invalid', owner: 'account', profile: { device: 'account' }, access: { csrf: 'csrf' } } as unknown as ClientRuntime
afterEach(() => { auth.session = undefined; auth.listeners.clear(); vi.restoreAllMocks() })

it('retries visitor and account connections independently after sign-in', async () => {
  const guest = fixture('guest'), account = fixture('account')
  vi.spyOn(GuestRuntime, 'openAccount').mockResolvedValue(account.runtime as unknown as GuestRuntime)
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  auth.session = { account: { id: 'account' }, device_id: 'account', csrf: 'csrf' }
  await workspace.setAccount(reader)
  guest.runtime.reconnect.mockRejectedValue(new TypeError('Failed to fetch'))
  await workspace.reconnect()
  expect(guest.runtime.reconnect).toHaveBeenCalledOnce()
  expect(account.runtime.reconnect).toHaveBeenCalledOnce()
  expect(workspace.getSnapshot().connections.peers).toHaveLength(2)
  expect(guest.runtime.close).not.toHaveBeenCalled()
  workspace.dispose()
})

it('retries account initialization after a transient failure instead of getting stuck unavailable', async () => {
  const guest = fixture('guest'), account = fixture('account')
  const open = vi.spyOn(GuestRuntime, 'openAccount').mockRejectedValueOnce(new TypeError('Failed to fetch')).mockResolvedValue(account.runtime as unknown as GuestRuntime)
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  auth.session = { account: { id: 'account' }, device_id: 'account', csrf: 'csrf' }
  await expect(workspace.setAccount(reader)).rejects.toThrow('Failed to fetch')
  expect(workspace.getSnapshot().error).toBe('Failed to fetch')
  await workspace.reconnect()
  expect(open).toHaveBeenCalledTimes(2)
  expect(workspace.hasAccount).toBe(true)
  expect(workspace.getSnapshot().error).toBeUndefined()
  expect(account.runtime.reconnect).toHaveBeenCalledOnce()
  workspace.dispose()
})

it('does not mistake an expired presence check for a remote disconnect or affect a working local link', async () => {
  const guest = fixture('guest'), account = fixture('account')
  guest.state.checkedAt = Date.now() - 20000
  guest.state.connections.peers.push({ ...peer('local'), authority: 'LOCAL_DEVICE' })
  vi.spyOn(GuestRuntime, 'openAccount').mockResolvedValue(account.runtime as unknown as GuestRuntime)
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  auth.session = { account: { id: 'account' }, device_id: 'account', csrf: 'csrf' }
  await workspace.setAccount(reader)
  expect(workspace.getSnapshot().connections.peers.map(p => [p.pair_id, p.online])).toEqual([['guest', undefined], ['local', true], ['account', true]])
  workspace.dispose()
})

it('keeps guest and account peers, jobs and actions in their own runtime', async () => {
  const guest = fixture('guest'), account = fixture('account')
  vi.spyOn(GuestRuntime, 'openAccount').mockResolvedValue(account.runtime as unknown as GuestRuntime)
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  await workspace.connect()
  auth.session = { account: { id: 'account' }, device_id: 'account', csrf: 'csrf' }
  await workspace.setAccount(reader)
  await workspace.setAccount({ ...reader } as ClientRuntime)
  expect(GuestRuntime.openAccount).toHaveBeenCalledOnce(); expect(account.runtime.close).not.toHaveBeenCalled()
  expect(workspace.getSnapshot().connections.peers.map(p => [p.pair_id, p.authority])).toEqual([['guest', undefined], ['account', 'ACCOUNT']])
  expect(workspace.getSnapshot().jobs.map(j => [j.id, j.authority])).toEqual([['guest', undefined], ['account', 'ACCOUNT']])
  expect(workspace.getSnapshot().invite?.id).toBe('guest')
  await workspace.connect(); expect(workspace.getSnapshot().invite?.id).toBe('account')
  await workspace.send(new File(['x'], 'x'), { ...peer('account'), authority: 'ACCOUNT' }, 100)
  await workspace.send(new File(['x'], 'x'), peer('guest'), 100)
  await workspace.accept('account'); await workspace.accept('guest')
  expect(account.runtime.send).toHaveBeenCalledOnce(); expect(guest.runtime.send).toHaveBeenCalledOnce()
  expect(account.runtime.accept).toHaveBeenCalledWith('account'); expect(guest.runtime.accept).toHaveBeenCalledWith('guest')
  expect(workspace.accountProfile?.device).toBe('account')
  auth.session = undefined; auth.listeners.forEach(listener => listener())
  expect(workspace.getSnapshot().jobs.map(j => j.id)).toEqual(['guest'])
  expect(account.runtime.close).toHaveBeenCalledOnce(); expect(guest.runtime.close).not.toHaveBeenCalled(); expect(guest.runtime.local.close).not.toHaveBeenCalled()
  workspace.dispose()
})

it('closes an account runtime that finishes opening after sign-out', async () => {
  const guest = fixture('guest'), account = fixture('late')
  let resolve!: (value: GuestRuntime) => void
  vi.spyOn(GuestRuntime, 'openAccount').mockReturnValue(new Promise(r => { resolve = r }))
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  const opening = workspace.setAccount(reader)
  await workspace.setAccount(undefined)
  resolve(account.runtime as unknown as GuestRuntime); await opening
  expect(workspace.hasAccount).toBe(false); expect(account.runtime.close).toHaveBeenCalledOnce()
  expect(workspace.getSnapshot().connections.peers.map(p => p.pair_id)).toEqual(['guest'])
  workspace.dispose()
})

it('blocks an update for account transfers as well as visitor work', async () => {
  const guest = fixture('guest'), account = fixture('account'); guest.state.jobs = []
  vi.spyOn(GuestRuntime, 'openAccount').mockResolvedValue(account.runtime as unknown as GuestRuntime)
  const workspace = new WorkspaceRuntime(guest.runtime as unknown as GuestRuntime)
  expect(safeToUpdate()).toBe(true)
  auth.session = { account: { id: 'account' }, device_id: 'account', csrf: 'csrf' }
  await workspace.setAccount(reader)
  expect(safeToUpdate()).toBe(false)
  await workspace.setAccount(undefined)
  expect(safeToUpdate()).toBe(true)
  workspace.dispose()
})
