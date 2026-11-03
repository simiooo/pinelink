import { StrictMode } from 'react'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { TasksPage } from './routes/tasks'
import { I18nProvider } from './i18n'
import type { Task } from './api/client'

afterEach(cleanup)
it('a late control acknowledgement preserves newer task versions and other rows under StrictMode', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  const task: Task = { id: 'a', kind: 'UPLOAD', reference_id: 'file', state: 'RUNNING', version: 1, progress_bytes: 10, total_bytes: 100, updated_at: '2026-11-01T16:00:00Z' }
  const newer = { ...task, version: 3, progress_bytes: 30 }
  const other = { ...task, id: 'b', reference_id: 'other' }
  client.setQueryData(['tasks'], { tasks: [task] })
  client.setQueryData(['transfers'], [])
  let complete!: (value: object) => void, refresh!: (value: object) => void
  const acknowledgement = new Promise<object>(r => { complete = r })
  const refetch = new Promise<object>(r => { refresh = r })
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) => ({
    ok: true, json: async () => init?.method === 'POST' ? acknowledgement : refetch,
  }))
  vi.stubGlobal('fetch', fetcher)
  render(<StrictMode><I18nProvider initialPreference="en-US"><QueryClientProvider client={client}><TasksPage /></QueryClientProvider></I18nProvider></StrictMode>)
  fireEvent.click(screen.getByRole('button', { name: 'Transfer details' }))
  fireEvent.click(await screen.findByRole('button', { name: /^Pause$/ }))
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
  act(() => { client.setQueryData(['tasks'], { tasks: [newer, other] }) })
  await act(async () => { complete({ ...task, version: 2, state: 'PAUSED' }) })
  expect(client.getQueryData(['tasks'])).toEqual({ tasks: [newer, other] })
  await act(async () => { refresh({ tasks: [newer, other] }) })
  client.clear()
})
