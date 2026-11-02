import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { App } from './main'

afterEach(() => {
  cleanup()
  window.history.replaceState({}, '', '/')
})

test('new transfer view keeps exactly three actions', async () => {
  render(<App initialEntries={['/']} />)
  expect(await screen.findByRole('button', { name: '选择文件' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '通知' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '更多' })).toBeInTheDocument()
  expect(screen.getAllByRole('button')).toHaveLength(2)
  expect(await screen.findByText('暂无任务。文件与接收设备确认后会出现在这里。')).toBeInTheDocument()
})

test('notification shows the real sender and accepts only once', async () => {
  const transfer = { id: 'request-1', version: 2, state: 'PENDING', sender_owner_id: 'team-design', sender_device_id: 'macbook-pro', file_name: '设计回顾.pdf', size_bytes: 2516582 }
  const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => ({
    ok: true,
    json: async () => init?.method === 'POST' ? { ...transfer, state: 'ACCEPTED' } : { tasks: [], transfers: [transfer] },
  }))
  vi.stubGlobal('fetch', fetchMock)
  render(<App initialEntries={['/']} />)
  fireEvent.click(await screen.findByRole('link', { name: '通知' }))
  expect(await screen.findByText('设计回顾.pdf')).toBeInTheDocument()
  expect(screen.getByText('macbook-pro')).toBeInTheDocument()
  expect(screen.getByText('team-design')).toBeInTheDocument()
  expect(screen.getByText('2.4 MB')).toBeInTheDocument()
  expect(screen.getAllByRole('button')).toHaveLength(3)
  fireEvent.click(screen.getByRole('button', { name: '接受' }))
  expect(await screen.findByText('已接受文件请求。')).toBeInTheDocument()
  const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
  expect(call?.[0].toString()).toContain('/v1/transfers/request-1/accept')
  expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ owner_id: 'dev-owner', device_id: 'dev-device', expected_version: 2 })
})

test('notification conflict refreshes rather than claiming success', async () => {
  const transfer = { id: 'request-2', version: 1, state: 'PENDING', sender_owner_id: 'owner-a', sender_device_id: 'phone-a' }
  vi.stubGlobal('fetch', vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => init?.method === 'POST'
    ? { ok: false, status: 409, json: async () => ({ code: 'STATE_CONFLICT' }) }
    : { ok: true, json: async () => ({ tasks: [], transfers: [transfer] }) }))
  render(<App initialEntries={['/']} />)
  fireEvent.click(await screen.findByRole('link', { name: '通知' }))
  fireEvent.click(await screen.findByRole('button', { name: '拒绝' }))
  expect(await screen.findByText('请求状态已变化，正在为你刷新。')).toBeInTheDocument()
  expect(screen.queryByText('已拒绝文件请求。')).not.toBeInTheDocument()
})

test('null notification and task lists have an empty state', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ tasks: null, transfers: null }) })))
  render(<App initialEntries={['/']} />)
  fireEvent.click(await screen.findByRole('link', { name: '通知' }))
  expect(await screen.findByText('没有待处理的文件请求')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '接受' })).not.toBeInTheDocument()
})

test('task controls preserve progress and use the latest version', async () => {
  let task = { id: 'task-1', kind: 'DOWNLOAD', reference_id: 'request-1', state: 'RUNNING', progress_bytes: 1500000, total_bytes: 2400000, version: 1, updated_at: '2026-10-31T10:00:00Z' }
  const actions: { action: string; version: number }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    const path = url.toString()
    if (init?.method === 'POST') {
      const action = path.split('/').pop()!
      const body = JSON.parse(String(init.body))
      actions.push({ action, version: body.expected_version })
      task = { ...task, state: action === 'pause' ? 'PAUSED' : action === 'resume' ? 'RUNNING' : 'CANCELLED', version: task.version + 1 }
      return { ok: true, json: async () => task }
    }
    return { ok: true, json: async () => path.includes('/v1/transfers') ? { transfers: [{ id: 'request-1', file_name: '设计回顾.pdf', sender_device_id: 'macbook-pro' }] } : { tasks: [task] } }
  }))
  render(<App initialEntries={['/tasks']} />)
  expect(await screen.findByText('设计回顾.pdf')).toBeInTheDocument()
  expect(screen.getByRole('progressbar', { name: '真实传输进度' })).toHaveAttribute('aria-valuenow', '63')
  expect(screen.getAllByRole('button')).toHaveLength(3)
  fireEvent.click(screen.getByRole('button', { name: '暂停' }))
  expect(await screen.findByText('已暂停，进度已保留。')).toBeInTheDocument()
  fireEvent.click(await screen.findByRole('button', { name: '继续' }))
  expect(await screen.findByText('已继续传输。')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认取消' }))
  expect(await screen.findByText('已取消任务。')).toBeInTheDocument()
  expect(actions).toEqual([{ action: 'pause', version: 1 }, { action: 'resume', version: 2 }, { action: 'cancel', version: 3 }])
  expect(screen.getByText('63%')).toBeInTheDocument()
})

test('file preparation creates a command without claiming a transfer', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  render(<App initialEntries={['/']} />)
  expect(await screen.findByRole('button', { name: '选择文件' })).toBeInTheDocument()
  fireEvent.change(document.querySelector('input[type="file"]')!, { target: { files: [new File(['content'], '设计回顾.pdf', { type: 'application/pdf' })] } })
  fireEvent.click(await screen.findByRole('button', { name: '继续' }))
  fireEvent.change(screen.getByLabelText('接收账户'), { target: { value: 'team-design' } })
  fireEvent.change(screen.getByLabelText('接收设备'), { target: { value: 'macbook-pro' } })
  fireEvent.change(screen.getByLabelText('文件在终端中的路径'), { target: { value: 'D:\\Transfers\\设计回顾.pdf' } })
  expect(screen.getAllByRole('button')).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: '复制发送命令' }))
  expect(await screen.findByText('已复制。请在已登记设备的终端执行；这里尚未发送文件。')).toBeInTheDocument()
  expect(writeText).toHaveBeenCalledWith("pinelink p2p send 'D:\\Transfers\\设计回顾.pdf' --to-owner 'team-design' --to-device 'macbook-pro' --path auto")
  expect(screen.queryByRole('button', { name: '接受' })).not.toBeInTheDocument()
})

test('history deletion confirms and preserves the audit record', async () => {
  const task = { id: 'task-history', kind: 'DOWNLOAD', reference_id: 'request-history', state: 'CANCELLED', progress_bytes: 100, total_bytes: 1000, version: 2, error_code: 'TRANSFER_CANCELLED', updated_at: '2026-10-31T10:00:00Z' }
  const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    const path = url.toString()
    if (init?.method === 'POST') return { ok: true, json: async () => ({ ...task, state: 'DELETED', version: 3 }) }
    if (path.includes('/v1/transfers')) return { ok: true, json: async () => ({ transfers: [{ id: 'request-history', file_name: '设计回顾.pdf', sender_device_id: 'macbook-pro' }] }) }
    return { ok: true, json: async () => ({ tasks: [task] }) }
  })
  vi.stubGlobal('fetch', fetchMock)
  render(<App initialEntries={['/history']} />)
  expect(await screen.findByText('设计回顾.pdf')).toBeInTheDocument()
  expect(screen.getByText('对方取消了这次传输。文件未标记为已收到。')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '从列表删除' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认删除' }))
  expect(await screen.findByText('已从默认列表删除，历史记录仍然保留。')).toBeInTheDocument()
  const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
  expect(call?.[0].toString()).toContain('/v1/tasks/task-history/delete')
  expect(JSON.parse(String(call?.[1]?.body)).expected_version).toBe(2)
})

test('persistent storage uses the current file version and real quota', async () => {
  let file = { id: 'entry-1', file_name: '设计回顾.pdf', state: 'AVAILABLE', version: 4, retention_class: 'TEMP_24H' }
  const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    const path = url.toString()
    if (init?.method === 'POST') {
      file = { ...file, retention_class: 'PERSISTENT', version: 5 }
      return { ok: true, json: async () => file }
    }
    if (path.includes('/v1/devices')) return { ok: true, json: async () => ({ devices: [{ device_id: 'dev-device', owner_id: 'dev-owner', status: 'ACTIVE', last_seen_at: '2026-10-31T10:00:00Z' }] }) }
    if (path.includes('/v1/quota')) return { ok: true, json: async () => ({ quota_bytes: 1000, used_bytes: 900 }) }
    if (path.includes('/v1/transfers')) return { ok: true, json: async () => ({ transfers: [{ id: 'request-owned', entry_id: 'entry-1', sender_owner_id: 'dev-owner', file_name: '设计回顾.pdf' }] }) }
    if (path.includes('/v1/files/')) return { ok: true, json: async () => file }
    return { ok: true, json: async () => ({ tasks: [] }) }
  })
  vi.stubGlobal('fetch', fetchMock)
  render(<App initialEntries={['/settings']} />)
  fireEvent.click(await screen.findByRole('button', { name: '长期保存' }))
  expect(await screen.findByText('已长期保存；之后仍可重复获取。')).toBeInTheDocument()
  expect(screen.getByText('空间即将用完，请先清理不再需要的文件。')).toBeInTheDocument()
  const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
  expect(JSON.parse(String(call?.[1]?.body)).expected_version).toBe(4)
})

test('encrypted metadata is not presented as a plaintext file name', async () => {
  const transfer = { id: 'request-encrypted', version: 1, state: 'PENDING', sender_owner_id: 'owner-a', sender_device_id: 'phone-a', file_name: 'encrypted', size_bytes: 127 }
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ tasks: [], transfers: [transfer] }) })))
  render(<App initialEntries={['/notifications']} />)
  expect(await screen.findByText('加密文件')).toBeInTheDocument()
  expect(screen.getByText('名称在本机解密后可见 · 127 B')).toBeInTheDocument()
  expect(screen.queryByText('encrypted')).not.toBeInTheDocument()
})
