import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import * as auth from '../auth/client'
import { newProfile } from '../data/crypto'
import type { ClientRuntime } from '../data/runtime'
import { guestWorkPending, setGuestWorkPending } from './continuity'
import { GuestRuntime } from './runtime'

const resources: GuestRuntime[] = []
beforeEach(() => { vi.stubGlobal('crypto', webcrypto); Object.defineProperty(navigator, 'onLine', { configurable: true, value: false }); Object.defineProperty(navigator, 'locks', { configurable: true, value: undefined }) })
afterEach(async () => { for (const resource of resources.reverse()) { await resource.close(); await resource.store.delete() }; resources.length = 0; setGuestWorkPending(0); vi.restoreAllMocks() })

it('opens account history in a separate store and never replaces visitor keys or work', async () => {
  const guest = await GuestRuntime.open('https://unit.invalid/' + crypto.randomUUID()); resources.push(guest)
  const profile = await newProfile(), account = { id: 'acct-test', email: '', name: '', methods: [] }
  const session = { account, device_id: profile.device, csrf: 'account-csrf', expires_at: '2099-01-01T00:00:00Z' }
  vi.spyOn(auth, 'authState').mockReturnValue({ session, offlineAccount: account, capabilities: { enabled: true, required: true } })
  const reader = { server: guest.server, owner: account.id, profile, access: { csrf: session.csrf }, store: { target: async () => ({ offset: 0 }) } } as unknown as ClientRuntime
  setGuestWorkPending(1)
  const accountRuntime = await GuestRuntime.openAccount(reader, guest.local); resources.push(accountRuntime)
  expect(accountRuntime.profile.root).toBe(profile.root); expect(accountRuntime.profile.root).not.toBe(guest.profile.root)
  expect(accountRuntime.store.name).not.toBe(guest.store.name)
  expect(await accountRuntime.store.profiles.get('self')).toBeUndefined()
  expect(guestWorkPending()).toBe(true)
  await accountRuntime.close()
  expect(guestWorkPending()).toBe(true); expect(guest.local.getSnapshot()).toBeDefined()
})

it('reopens the matching account store offline without creating a guest lease', async () => {
  const guest = await GuestRuntime.open('https://unit.invalid/' + crypto.randomUUID()); resources.push(guest)
  const profile = await newProfile(), account = { id: 'acct-offline', email: '', name: '', methods: [] }
  vi.spyOn(auth, 'authState').mockReturnValue({ session: undefined, offlineAccount: account, capabilities: { enabled: true, required: true } })
  const reader = { server: guest.server, owner: account.id, profile, access: { localOnly: true }, store: { target: async () => ({ offset: 0 }) } } as unknown as ClientRuntime
  const accountRuntime = await GuestRuntime.openAccount(reader, guest.local); resources.push(accountRuntime)
  expect(accountRuntime.getSnapshot().ready).toBe(true)
  expect(accountRuntime.getSnapshot().connected).toBe(false)
  expect(await accountRuntime.store.leases.get('self')).toBeUndefined()
  await expect(accountRuntime.ensureSession()).rejects.toThrow('ACCOUNT_SIGN_IN_REQUIRED')
})
