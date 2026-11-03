import { StrictMode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { useAccountReader } from './account'

const state = vi.hoisted(() => ({ auth: { capabilities: { enabled: true }, session: { account: { id: 'acct-a' }, device_id: 'device-a', csrf: 'csrf-a' } }, oldVisitor: { owner: 'guest-original', profile: { device: 'visitor' }, access: { localOnly: true }, close: vi.fn() }, open: vi.fn() }))
vi.mock('../auth/client', () => ({ authState: () => state.auth, sessionExpired: vi.fn() }))
vi.mock('../data', () => ({ clientRuntime: () => state.oldVisitor }))
vi.mock('../data/runtime', () => ({ ClientRuntime: { open: (...args: unknown[]) => state.open(...args) } }))
vi.mock('../data/service-profile', () => ({ loadServerPin: async () => 'pin' }))
vi.mock('../api/client', () => ({ API_URL: 'https://unit.invalid/api', IDENTITY: { owner: 'visitor', device: 'visitor' } }))

it('StrictMode and a late account open cannot prime a replaced account or close the visitor runtime', async () => {
  const pending = new Map<string, (value: unknown) => void>()
  state.open.mockImplementation((_url: string, owner: string) => new Promise(resolve => pending.set(owner, resolve)))
  const reader = (suffix: string) => ({ owner: 'acct-' + suffix, profile: { device: 'device-' + suffix }, access: { csrf: 'csrf-' + suffix }, prime: vi.fn().mockResolvedValue(undefined), close: vi.fn().mockResolvedValue(undefined) })
  const a = reader('a'), b = reader('b')
  const hook = renderHook(useAccountReader, { wrapper: StrictMode }); await waitFor(() => expect(pending.has('acct-a')).toBe(true))
  state.auth = { ...state.auth, session: { account: { id: 'acct-b' }, device_id: 'device-b', csrf: 'csrf-b' } }; hook.rerender(); await waitFor(() => expect(pending.has('acct-b')).toBe(true))
  await act(async () => { pending.get('acct-a')!(a) }); expect(a.close).toHaveBeenCalled(); expect(a.prime).not.toHaveBeenCalled()
  await act(async () => { pending.get('acct-b')!(b) }); await waitFor(() => expect(hook.result.current.reader).toBe(b)); expect(b.prime).toHaveBeenCalledWith('acct-b')
  expect(state.oldVisitor.close).not.toHaveBeenCalled(); hook.unmount(); expect(b.close).toHaveBeenCalled()
})
