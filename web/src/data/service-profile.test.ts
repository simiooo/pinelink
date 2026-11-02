import 'fake-indexeddb/auto'
import Dexie from 'dexie'
import { afterEach, expect, it, vi } from 'vitest'
import { loadServerPin } from './service-profile'

const a = btoa(String.fromCharCode(...new Uint8Array(32).fill(1)))
const b = btoa(String.fromCharCode(...new Uint8Array(32).fill(2)))
const response = (key: string, api_url = '/api') => new Response(JSON.stringify({ version: 1, api_url, history_public_key: key }), { headers: { 'Content-Type': 'application/json' } })
afterEach(async () => { await Dexie.delete('pinelink.service-profiles'); vi.unstubAllGlobals() })

it('pins one key atomically across competing clients and reuses it offline without localStorage', async () => {
  vi.stubGlobal('localStorage', undefined)
  const fetch = vi.fn().mockResolvedValueOnce(response(a)).mockResolvedValueOnce(response(b))
  vi.stubGlobal('fetch', fetch)
  const keys = await Promise.all([loadServerPin('/api', '', true), loadServerPin('/api', '', true)])
  expect(keys[0]).toBe(keys[1])
  expect([a, b]).toContain(keys[0])
  expect(fetch).toHaveBeenCalledWith('/service-config.json', expect.objectContaining({ credentials: 'same-origin', redirect: 'error', cache: 'no-store' }))
  fetch.mockRejectedValue(new Error('offline'))
  expect(await loadServerPin('/api', '', true)).toBe(keys[0])
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('refuses an origin mismatch and an unverified first profile without downgrading to API-only mode', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(a, 'https://other.invalid/api')))
  await expect(loadServerPin('/api', '', true)).rejects.toThrow('INVALID_SERVICE_PROFILE')
  await expect(loadServerPin('https://other.invalid/api', '', true)).rejects.toThrow('SERVICE_PROFILE_ORIGIN_MISMATCH')
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
  await expect(loadServerPin('/api', '', true)).rejects.toThrow('offline')
})

it('preserves an explicitly configured key and legacy preview behavior', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  expect(await loadServerPin('/api', a, true)).toBe(a)
  expect(await loadServerPin('/api')).toBe('')
  expect(fetch).not.toHaveBeenCalled()
})

it('fails closed on a corrupted persisted pin', async () => {
  const database = new Dexie('pinelink.service-profiles')
  database.version(1).stores({ pins: 'server' })
  await database.table('pins').put({ server: new URL('/api', location.href).href, key: 'corrupted' })
  database.close()
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  await expect(loadServerPin('/api', '', true)).rejects.toThrow('INVALID_SERVICE_PIN')
  expect(fetch).not.toHaveBeenCalled()
})
