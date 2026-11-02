import '@testing-library/jest-dom/vitest'
import { beforeEach, vi } from 'vitest'

beforeEach(() => {
  window.scrollTo = vi.fn()
  localStorage.clear()
  Object.defineProperty(navigator, 'languages', { value: ['zh-CN'], configurable: true })
  vi.stubGlobal('fetch', vi.fn(async () => ({
    ok: true,
    json: async () => ({ tasks: [], transfers: [] }),
  })))
})
