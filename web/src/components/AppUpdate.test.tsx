import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { AppUpdate } from './AppUpdate'

const store = vi.hoisted(() => ({ state: { status: 'latest', waiting: false, locked: false, notice: 1, explicit: true }, listeners: new Set<() => void>(), close: vi.fn() }))
vi.mock('../pwa', () => ({ pwaSnapshot: () => store.state, subscribePwa: (fn: () => void) => { store.listeners.add(fn); return () => store.listeners.delete(fn) }, dismissUpdateNotice: (notice: number) => store.close(notice), applyAppUpdate: vi.fn() }))
vi.mock('../i18n', () => ({ useI18n: () => ({ locale: 'en-US' }) }))
vi.mock('../pwa/guard', () => ({ safeToUpdate: () => true }))
vi.mock('../motion/components', () => ({ MotionTooltip: ({ children }: { children: React.ReactNode }) => children, MotionButton: ({ icon: _icon, ...props }: React.ComponentProps<'button'> & { icon?: boolean }) => <button {...props} /> }))
const tick = async (ms: number) => { await act(async () => { vi.advanceTimersByTime(ms) }) }
const publish = async (patch: Partial<typeof store.state>) => { await act(async () => { store.state = { ...store.state, ...patch }; store.listeners.forEach(fn => fn()) }) }
beforeEach(() => { vi.useFakeTimers(); store.state = { status: 'latest', waiting: false, locked: false, notice: 1, explicit: true }; store.close.mockClear() })
afterEach(() => { cleanup(); document.body.innerHTML = ''; vi.useRealTimers() })
it('restarts the full duration for an identical result and closes only that notice', async () => {
  render(<AppUpdate />); await tick(7000)
  expect(store.close).not.toHaveBeenCalled()
  await publish({ notice: 2 }); await tick(2000)
  expect(store.close).not.toHaveBeenCalled()
  await tick(6001); expect(store.close).toHaveBeenCalledExactlyOnceWith(2)
})
it('keeps explicit feedback visible with a selected device and service failure', async () => {
  document.body.innerHTML = '<div class="guest-network" data-selected="peer"></div><div class="guest-error">Service unavailable</div>'
  render(<AppUpdate />); await tick(1)
  expect(document.querySelector('.app-update-notice')).toHaveTextContent('You’re up to date')
})
it('starts a full visible duration after an open dialog closes', async () => {
  const dialog = document.createElement('div'); dialog.setAttribute('role', 'dialog'); dialog.setAttribute('data-state', 'open'); document.body.append(dialog)
  render(<AppUpdate />); await tick(10000)
  expect(document.querySelector('.app-update-notice')).toBeNull(); expect(store.close).not.toHaveBeenCalled()
  await act(async () => { dialog.setAttribute('data-state', 'closed'); dialog.setAttribute('aria-hidden', 'true') })
  expect(document.querySelector('.app-update-notice')).toBeInTheDocument()
  await tick(7999); expect(store.close).not.toHaveBeenCalled()
  await tick(2); expect(store.close).toHaveBeenCalledExactlyOnceWith(1)
})
it('uses the primitive close control to dismiss the current notice', () => {
  const { getByRole } = render(<AppUpdate />)
  fireEvent.click(getByRole('button', { name: 'Close' }))
  expect(store.close).toHaveBeenCalledExactlyOnceWith(1)
})
it('does not time out or dismiss a locked update', async () => {
  store.state = { ...store.state, status: 'applying', locked: true }
  const { queryByRole } = render(<AppUpdate />); await tick(20000)
  expect(store.close).not.toHaveBeenCalled(); expect(queryByRole('button', { name: 'Close' })).toBeNull()
})
