import { afterEach, beforeEach, expect, it, vi } from 'vitest'

vi.mock('./data/background-sync', () => ({ requestBackgroundSync: vi.fn(async () => undefined) }))
let stop: (() => void) | undefined
const deferred = () => { let resolve!: () => void, reject!: () => void; const promise = new Promise<void>((ok, no) => { resolve = ok; reject = no }); return { promise, resolve, reject } }
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve() }
beforeEach(() => { vi.resetModules(); vi.stubEnv('PROD', true); vi.spyOn(document, 'readyState', 'get').mockReturnValue('complete'); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); Object.defineProperty(navigator, 'onLine', { configurable: true, value: true }); window.matchMedia = vi.fn().mockReturnValue({ matches: false }) })
afterEach(() => { stop?.(); stop = undefined; vi.restoreAllMocks(); vi.unstubAllEnvs(); delete (navigator as unknown as Record<string, unknown>).serviceWorker })
async function setup() {
  const request = deferred(), update = vi.fn(() => request.promise)
  const registration = Object.assign(new EventTarget(), { update, installing: null, waiting: null, active: {} })
  const workers = Object.assign(new EventTarget(), { register: vi.fn(async () => registration), ready: Promise.resolve(registration) })
  Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: workers })
  const pwa = await import('./pwa'); stop = pwa.startPwa(); await flush()
  expect(update).toHaveBeenCalledTimes(1)
  return { pwa, request, update }
}
it('gives an explicit result when a tap joins a background check', async () => {
  const { pwa, request, update } = await setup()
  expect(pwa.pwaSnapshot().status).toBe('idle')
  const checked = pwa.checkAppUpdate()
  expect(pwa.pwaSnapshot()).toMatchObject({ status: 'checking', explicit: true })
  request.resolve(); await checked
  expect(update).toHaveBeenCalledTimes(1)
  expect(pwa.pwaSnapshot()).toMatchObject({ status: 'latest', explicit: true })
})
it('keeps failure feedback for a manual request joined to background work', async () => {
  const { pwa, request } = await setup(), checked = pwa.checkAppUpdate()
  request.reject(); await checked
  expect(pwa.pwaSnapshot()).toMatchObject({ status: 'error', explicit: true })
})
it('does not resurrect a checking notice the user already closed', async () => {
  const { pwa, request } = await setup(), checked = pwa.checkAppUpdate()
  pwa.dismissUpdateNotice(); request.resolve(); await checked
  expect(pwa.pwaSnapshot().status).toBe('idle')
})
it('gives repeated identical results separate lifetimes and ignores stale closes', async () => {
  const { pwa, request } = await setup(), checked = pwa.checkAppUpdate()
  request.resolve(); await checked
  const previous = pwa.pwaSnapshot().notice
  await pwa.checkAppUpdate()
  expect(pwa.pwaSnapshot().status).toBe('latest')
  expect(pwa.pwaSnapshot().notice).toBeGreaterThan(previous)
  pwa.dismissUpdateNotice(previous)
  expect(pwa.pwaSnapshot().status).toBe('latest')
  pwa.dismissUpdateNotice(pwa.pwaSnapshot().notice)
  expect(pwa.pwaSnapshot().status).toBe('idle')
})
it('reports unavailable checks instead of silently doing nothing', async () => {
  const pwa = await import('./pwa')
  await pwa.checkAppUpdate(); expect(pwa.pwaSnapshot().status).toBe('error')
  Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
  await pwa.checkAppUpdate(); expect(pwa.pwaSnapshot()).toMatchObject({ status: 'offline', explicit: true })
})
