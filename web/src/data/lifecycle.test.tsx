import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { StrictMode } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ClientRuntime } from './runtime'
import { LocalStore } from './store'
import { ClientWorker } from './ClientWorker'
import { clientRuntime, closeClient, initializeClient } from './index'

let runtime: ClientRuntime
beforeEach(async () => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
  runtime = await ClientRuntime.open('https://unit.invalid', 'owner', '', '', new LocalStore('lifecycle-' + crypto.randomUUID()))
  await runtime.store.patchTarget({ bound: 'API_ONLY' })
})
afterEach(async () => {
  cleanup()
  await closeClient()
  await runtime.close()
  await runtime.store.delete()
  vi.restoreAllMocks()
  vi.useRealTimers()
})
it('shares initialization, rejects a different identity, and closes an opening session', async () => {
  let resolve!: (value: ClientRuntime) => void
  const opening = new Promise<ClientRuntime>(r => { resolve = r })
  const open = vi.spyOn(ClientRuntime, 'open').mockReturnValue(opening)
  const a = initializeClient('https://unit.invalid', 'owner', '', '')
  const b = initializeClient('https://unit.invalid/', 'owner', '', '')
  await expect(initializeClient('https://unit.invalid', 'other-owner', '', '')).rejects.toThrow('CLIENT_CONFIGURATION_CHANGED')
  const closed = closeClient()
  resolve(runtime)
  expect(await a).toBe(await b)
  await closed
  expect(open).toHaveBeenCalledTimes(1)
  expect(clientRuntime()).toBeUndefined()
  expect(runtime.store.isOpen()).toBe(false)
})
it('retries initialization after failure without retaining a poisoned singleton', async () => {
  const open = vi.spyOn(ClientRuntime, 'open').mockRejectedValueOnce(new Error('unavailable')).mockResolvedValue(runtime)
  await expect(initializeClient('https://unit.invalid', 'owner', '', '')).rejects.toThrow('unavailable')
  expect(() => clientRuntime()).toThrow('unavailable')
  await initializeClient('https://unit.invalid', 'owner', '', '')
  expect(clientRuntime()).toBe(runtime)
  expect(open).toHaveBeenCalledTimes(2)
})
it('replays real worker setup and cleanup under StrictMode with one loop and no leaked listener', async () => {
  await runtime.store.cacheResponse('/v1/tasks', { tasks: [] }, 1)
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ tasks: [] })))
  vi.stubGlobal('fetch', fetcher)
  const start = vi.spyOn(runtime, 'start'), stop = vi.spyOn(runtime, 'stop')
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  const view = render(<StrictMode><ClientWorker runtime={runtime}><div /></ClientWorker></StrictMode>)
  await act(async () => { await runtime.sync(true) })
  expect(start).toHaveBeenCalledTimes(2)
  expect(stop).toHaveBeenCalledTimes(1)
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(vi.getTimerCount()).toBe(1)
  view.unmount()
  expect(vi.getTimerCount()).toBe(0)
  window.dispatchEvent(new Event('online'))
  await Promise.resolve()
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(runtime.store.isOpen()).toBe(true)
})
