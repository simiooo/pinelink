import { StrictMode } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { App } from '../main'
import * as auth from './client'

vi.mock('./client', async original => ({ ...await original<typeof import('./client')>(), authState: () => ({ capabilities: { enabled: true, google: true, email: true }, session: undefined }), login: vi.fn(), googleLogin: vi.fn(), startEmail: vi.fn() }))
afterEach(() => { cleanup(); vi.resetAllMocks() })
it('StrictMode mounts never start sign-in; Google works with the password form empty', async () => {
  render(<StrictMode><App initialEntries={['/login']} localePreference="en-US" /></StrictMode>)
  const google = await screen.findByRole('button', { name: /^Continue with Google$/ })
  expect(auth.login).not.toHaveBeenCalled(); expect(auth.googleLogin).not.toHaveBeenCalled(); expect(auth.startEmail).not.toHaveBeenCalled()
  expect(google.closest('form')).toBeNull()
  fireEvent.click(google)
  await waitFor(() => expect(auth.googleLogin).toHaveBeenCalledTimes(1))
  expect(auth.login).not.toHaveBeenCalled()
})
it('a pending password submit rejects duplicate keyboard and click submissions', async () => {
  let fail!: (reason: Error) => void
  vi.mocked(auth.login).mockImplementation(() => new Promise((_, reject) => { fail = reject }))
  render(<StrictMode><App initialEntries={['/login']} localePreference="en-US" /></StrictMode>)
  const password = await screen.findByLabelText('Password', { exact: true })
  fireEvent.change(screen.getByLabelText('Email', { exact: true }), { target: { value: 'person@example.com' } })
  fireEvent.change(password, { target: { value: 'a sufficiently long private password' } })
  fireEvent.submit(password.closest('form')!); fireEvent.submit(password.closest('form')!)
  expect(auth.login).toHaveBeenCalledTimes(1)
  fail(new auth.AuthError('INVALID_CREDENTIALS', 401))
  expect(await screen.findByRole('alert')).toHaveTextContent('Email or password is incorrect.')
})
it('new-password help and visibility control keep separate accessible names', async () => {
  render(<StrictMode><App initialEntries={['/register']} localePreference="en-US" /></StrictMode>)
  const password = await screen.findByLabelText('New password', { exact: true })
  expect(password).toHaveAttribute('aria-describedby', 'password-hint')
  fireEvent.click(screen.getByRole('button', { name: /^Show password$/ }))
  expect(password).toHaveAttribute('type', 'text')
  expect(auth.startEmail).not.toHaveBeenCalled()
})
