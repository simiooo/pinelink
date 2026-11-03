import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { expect, it, vi } from 'vitest'
import Dexie from 'dexie'
import { ClientRuntime, DataError } from '../data/runtime'
import { LocalStore, storeName } from '../data/store'

it('an offline account reads its own cache without registering or sending cloud work', async () => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher)
  const name = 'account-local-' + crypto.randomUUID(), store = new LocalStore(name)
  const runtime = await ClientRuntime.open('https://unit.invalid/api', 'acct-local', '', '', store, { localOnly: true })
  try {
    const path = '/v1/tasks?owner_id=acct-local', tasks = { tasks: [{ id: 'saved', state: 'PAUSED' }] }
    await store.cacheResponse(path, tasks, await store.beginRead())
    expect(await runtime.request('GET', path)).toEqual(tasks)
    await expect(runtime.request('POST', '/v1/transfers', {})).rejects.toBeInstanceOf(DataError)
    await runtime.sync(true); runtime.start()
    expect(fetcher).not.toHaveBeenCalled()
    expect(await storeName('https://unit.invalid/api', 'acct-local', '')).not.toBe(await storeName('https://other.invalid/api', 'acct-local', ''))
    expect(await storeName('https://unit.invalid/api', 'acct-local', '')).not.toBe(await storeName('https://unit.invalid/api', 'acct-other', ''))
  } finally { await runtime.close(); await Dexie.delete(name); vi.unstubAllGlobals() }
})
