import { expect, it, vi } from 'vitest'
import { pendingUpdate, updateMessageSource } from './messages'
const message = (source: unknown, type = 'PINELINK_UPDATE') => ({ data: { type }, source } as MessageEvent)
it('does not inspect unrelated window or OAuth messages', () => {
  const getter = vi.fn(() => { throw new DOMException('Blocked a frame', 'SecurityError') })
  const window = Object.defineProperty({}, 'scriptURL', { get: getter })
  expect(updateMessageSource(message(window, 'oauth'), 'https://pinelink.example')).toBeUndefined()
  expect(getter).not.toHaveBeenCalled()
  expect(updateMessageSource(message(window), 'https://pinelink.example')).toBeUndefined()
})
it('accepts only this origin’s application worker', () => {
  const source = { scriptURL: 'https://pinelink.example/sw.js' }
  expect(updateMessageSource(message(source), 'https://pinelink.example')).toBe(source)
  expect(updateMessageSource(message({ scriptURL: 'https://foreign.example/sw.js' }), 'https://pinelink.example')).toBeUndefined()
  expect(updateMessageSource(message({ scriptURL: 'https://pinelink.example/other.js' }), 'https://pinelink.example')).toBeUndefined()
})
it('does not advertise a first installation or an already active worker as an update', () => {
  const first = {} as ServiceWorker, next = {} as ServiceWorker
  expect(pendingUpdate({ active: null, waiting: first })).toBeUndefined()
  expect(pendingUpdate({ active: first, waiting: first })).toBeUndefined()
  expect(pendingUpdate({ active: first, waiting: null })).toBeUndefined()
  expect(pendingUpdate({ active: first, waiting: next })).toBe(next)
})
