import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ClientRuntime } from './runtime'
import { LocalStore } from './store'

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}
const path = '/v1/tasks?include_deleted=true'
const running = { tasks: [{ id: 'task', version: 1, state: 'RUNNING' }] }
const paused = { id: 'task', version: 2, state: 'PAUSED' }
const resources: ClientRuntime[] = []
beforeEach(() => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
  Object.defineProperty(navigator, 'locks', { value: undefined, configurable: true })
})
afterEach(async () => {
  vi.useRealTimers()
  for (const r of resources.splice(0)) { await r.close(); await r.store.delete() }
  vi.restoreAllMocks()
})
async function runtime() {
  const r = await ClientRuntime.open('https://unit.invalid', 'owner', '', '', new LocalStore('race-' + crypto.randomUUID()))
  resources.push(r)
  await r.store.patchTarget({ bound: 'API_ONLY' })
  return r
}
it('does not let a same-millisecond response undo a completed control', async () => {
  const r = await runtime()
  vi.spyOn(Date, 'now').mockReturnValue(42)
  await r.store.cacheResponse(path, running, 1)
  const entered = deferred<void>(), response = deferred<Response>()
  vi.stubGlobal('fetch', vi.fn(async () => { entered.resolve(); return response.promise }))
  const read = r.request('GET', path)
  await entered.promise
  await r.store.cacheMutation(paused)
  response.resolve(new Response(JSON.stringify(running)))
  expect(await read).toEqual({ tasks: [paused] })
})
it('fences two reads even when the wall clock goes backwards', async () => {
  const r = await runtime()
  const first = deferred<Response>(), entered = deferred<void>()
  const now = vi.spyOn(Date, 'now').mockReturnValue(200)
  vi.stubGlobal('fetch', vi.fn()
    .mockImplementationOnce(() => { entered.resolve(); return first.promise })
    .mockResolvedValueOnce(new Response(JSON.stringify({ tasks: [paused] }))))
  const old = r.request('GET', path)
  await entered.promise
  now.mockReturnValue(100)
  await r.request('GET', path)
  first.resolve(new Response(JSON.stringify(running)))
  expect(await old).toEqual({ tasks: [paused] })
})
it('drains foreground work during close, rejects late success, and cannot reopen a closed runtime', async () => {
  const r = await runtime()
  const first = deferred<Response>(), entered = deferred<void>()
  vi.stubGlobal('fetch', vi.fn(() => { entered.resolve(); return first.promise }))
  const read = r.request('GET', path)
  const rejected = expect(read).rejects.toThrow('CLIENT_CLOSED')
  await entered.promise
  const close = r.close()
  first.resolve(new Response(JSON.stringify(running)))
  await rejected
  await close
  expect(r.store.isOpen()).toBe(false)
  await expect(r.request('GET', path)).rejects.toThrow('CLIENT_CLOSED')
  expect(() => r.start()).toThrow('CLIENT_CLOSED')
})
it('does not schedule another loop after close while a sync request was pending', async () => {
  const r = await runtime()
  const first = deferred<Response>(), entered = deferred<void>()
  await r.store.cacheResponse(path, running, 1)
  vi.stubGlobal('fetch', vi.fn(() => { entered.resolve(); return first.promise }))
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  r.start()
  await entered.promise
  const close = r.close()
  first.resolve(new Response(JSON.stringify(running)))
  await close
  expect(vi.getTimerCount()).toBe(0)
})
it('retries a first read superseded by a control before any cache existed', async () => {
  const r = await runtime(), entered = deferred<void>(), response = deferred<Response>()
  const fetcher = vi.fn()
    .mockImplementationOnce(() => { entered.resolve(); return response.promise })
    .mockResolvedValueOnce(new Response(JSON.stringify({ tasks: [paused] })))
  vi.stubGlobal('fetch', fetcher)
  const read = r.request('GET', path)
  await entered.promise
  await r.store.cacheMutation(paused)
  response.resolve(new Response(JSON.stringify(running)))
  expect(await read).toEqual({ tasks: [paused] })
  expect(fetcher).toHaveBeenCalledTimes(2)
})
it('uses one identity and unique read order across concurrent database connections', async () => {
  const a = new LocalStore('tabs-' + crypto.randomUUID()), b = new LocalStore(a.name)
  try {
    const profiles = await Promise.all([a.profile(), b.profile()])
    expect(profiles[0].root).toBe(profiles[1].root)
    const tokens = await Promise.all(Array.from({ length: 16 }, (_, i) => (i % 2 ? a : b).beginRead()))
    expect(new Set(tokens).size).toBe(16)
    tokens.sort((a, b) => a - b)
    await a.cacheResponse(path, { tasks: [paused] }, tokens.at(-1)!)
    expect(await b.cacheResponse(path, running, tokens[0])).toBe(false)
    expect((await a.responses.get(path))?.value).toEqual({ tasks: [paused] })
  } finally { a.close(); b.close(); await a.delete() }
})
it('a foreground sync waits after joining a skipped background lock attempt', async () => {
  const r = await runtime(), skipped = deferred<void>()
  const lock = { name: 'sync', mode: 'exclusive' }
  const request = vi.fn((_name: string, options: LockOptions, callback: (lock: unknown) => unknown) => options.ifAvailable ? skipped.promise.then(() => callback(null)) : Promise.resolve(callback(lock)))
  Object.defineProperty(navigator, 'locks', { value: { request }, configurable: true })
  const background = r.sync(), foreground = r.sync(true)
  skipped.resolve()
  await Promise.all([background, foreground])
  expect(request).toHaveBeenCalledTimes(2)
  expect(request.mock.calls[1][1].ifAvailable).not.toBe(true)
  expect(request.mock.calls[1][1].signal).toBeInstanceOf(AbortSignal)
})
