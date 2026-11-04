import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { App } from './main'
import type { GuestJob } from './guest/store'
import type { GuestPeer } from './guest/types'

const visitor = vi.hoisted(() => {
  const listeners = new Set<() => void>()
  const state = { ready: true, connected: true, busy: false, offline: false, checkedAt: Date.now(), jobs: [] as GuestJob[], connections: { invites: [], candidates: [], peers: [] as GuestPeer[] } }
  let snapshot = state
  return { state, listeners, emit: () => { snapshot = { ...state, connections: { ...state.connections } }; listeners.forEach(listener => listener()) }, runtime: { profile: { device: 'visitor-device' }, getSnapshot: () => snapshot, now: () => Date.now(), subscribe: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn) }, connect: vi.fn(), send: vi.fn(), renamePeer: vi.fn(), removePeer: vi.fn(), download: vi.fn() } }
})
vi.mock('./guest/runtime', () => ({ getGuestRuntime: async () => visitor.runtime }))
const peer: GuestPeer = { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'device-peer', device_name: 'MacBook Pro', encryption_key: 'key', signing_key: 'key', expires_at: '2099-01-01T00:00:00Z', online: true }
const job = (phase: GuestJob['phase'], name = 'Design source.fig'): GuestJob => ({ id: phase, created: 1, direction: 'send', name, plainSize: 100, phase, received: 0, peer })
beforeEach(() => { visitor.state.jobs = []; visitor.state.connections.peers = []; visitor.state.offline = false; visitor.state.checkedAt = Date.now(); visitor.emit(); vi.clearAllMocks(); visitor.runtime.renamePeer.mockResolvedValue(undefined); visitor.runtime.removePeer.mockResolvedValue(undefined) })
afterEach(() => { cleanup(); window.history.replaceState({}, '', '/') })

test('home is the graph with persistent file, add-device and notification entries', async () => {
  render(<App initialEntries={['/']} />)
  await screen.findByRole('button', { name: '本机' })
  expect(screen.getByRole('button', { name: '文件' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '通知' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '选择文件' })).toBeInTheDocument()
  expect(screen.queryByText('复制发送命令')).not.toBeInTheDocument()
  const graph = document.querySelector('.guest-network')
  fireEvent.click(screen.getByRole('button', { name: '文件' })); await screen.findByRole('heading', { name: '文件' })
  fireEvent.click(screen.getByRole('button', { name: '关闭' })); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(document.querySelector('.guest-network')).toBe(graph)
})
test('more menu keeps settings and language without duplicate workspace entries', async () => {
  render(<App initialEntries={['/']} />)
  await screen.findByRole('button', { name: '本机' })
  fireEvent.keyDown(screen.getByRole('button', { name: '更多' }), { key: 'ArrowDown' })
  expect(await screen.findByRole('menuitem', { name: '小窝与云仓' })).toBeInTheDocument()
  expect(screen.getByRole('menuitemradio', { name: '中文' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: /^(设备|文件|账号)$/ })).not.toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
  expect(screen.getByRole('button', { name: '文件' })).toBeInTheDocument()
})
test('legacy task and local-file links enter filters in the same file surface', async () => {
  const { unmount } = render(<App initialEntries={['/tasks']} />)
  await screen.findByRole('heading', { name: '文件' }); expect(screen.getByRole('tab', { name: '进行中' })).toHaveAttribute('aria-selected', 'true')
  unmount(); render(<App initialEntries={['/local-files']} />)
  await screen.findByRole('heading', { name: '文件' }); expect(screen.getByRole('tab', { name: '本机' })).toHaveAttribute('aria-selected', 'true')
})
test('files filter active, saved and expired records without discarding their state', async () => {
  visitor.state.jobs = [job('paused', 'Active.zip'), { ...job('complete', 'Saved.pdf'), plain: new Blob(['saved']), direction: 'receive' }, { ...job('stopped', 'Expired.zip'), error: 'FILE_EXPIRED' }]
  visitor.emit()
  render(<App initialEntries={['/files']} />); await screen.findByRole('heading', { name: '文件' })
  expect(screen.getByRole('button', { name: /Active.zip/ })).toBeInTheDocument()
  fireEvent.mouseDown(screen.getByRole('tab', { name: '本机' }), { button: 0, ctrlKey: false }); expect(screen.getByRole('button', { name: /Saved.pdf/ })).toBeInTheDocument(); expect(screen.queryByRole('button', { name: /Active.zip/ })).not.toBeInTheDocument()
  fireEvent.mouseDown(screen.getByRole('tab', { name: '异常' }), { button: 0, ctrlKey: false }); expect(screen.getByRole('button', { name: /Expired.zip/ })).toBeInTheDocument(); expect(screen.queryByRole('button', { name: /Saved.pdf/ })).not.toBeInTheDocument()
})
test('sent filter shows completed outgoing records without treating stored uploads as delivery', async () => {
  visitor.state.jobs = [{ ...job('complete', 'Sent.txt'), id: 'sent' }, { ...job('complete', 'Received.txt'), id: 'received', direction: 'receive', plain: new Blob(['x']) }, { ...job('waiting', 'Waiting.txt'), id: 'waiting' }]
  visitor.emit()
  const task = { id: 'stored-task', kind: 'UPLOAD', reference_id: 'stored-entry', state: 'COMPLETED', progress_bytes: 100, total_bytes: 100, version: 2, updated_at: '2026-11-03T16:00:00Z' }
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => ({ ok: true, json: async () => url.toString().includes('/v1/transfers') ? { transfers: [{ id: 'stored-transfer', entry_id: 'stored-entry', file_name: 'Cloud.pdf', state: 'PENDING', sender_owner_id: 'dev-owner', sender_device_id: 'dev-device' }] } : { tasks: [task] } })))
  render(<App initialEntries={['/local-files']} />); await screen.findByRole('heading', { name: '文件' })
  fireEvent.mouseDown(screen.getByRole('tab', { name: '已发送' }), { button: 0, ctrlKey: false })
  expect(screen.getByRole('button', { name: /Sent.txt.*已发送.*已送达/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Received.txt|Waiting.txt|Cloud.pdf/ })).not.toBeInTheDocument()
})
test('account sent status requires a completed transfer from the current device', async () => {
  const tasks = ['mine', 'other'].map(id => ({ id, kind: 'UPLOAD', reference_id: id, state: 'COMPLETED', progress_bytes: 1, total_bytes: 1, version: 1, updated_at: '2026-11-03T16:00:00Z' }))
  const transfers = [{ id: 'a', entry_id: 'mine', file_name: 'Mine.pdf', state: 'COMPLETED', sender_owner_id: 'dev-owner', sender_device_id: 'dev-device' }, { id: 'b', entry_id: 'other', file_name: 'Other.pdf', state: 'COMPLETED', sender_owner_id: 'dev-owner', sender_device_id: 'other-device' }]
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => ({ ok: true, json: async () => url.toString().includes('/v1/transfers') ? { transfers } : { tasks } })))
  render(<App initialEntries={['/files']} />); await screen.findByRole('button', { name: /Mine.pdf/ })
  fireEvent.mouseDown(screen.getByRole('tab', { name: '已发送' }), { button: 0, ctrlKey: false })
  expect(screen.getByRole('button', { name: /Mine.pdf.*已发送.*已送达/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Other.pdf/ })).not.toBeInTheDocument()
})
test('all devices appear and editing a local name leaves other devices intact', async () => {
  visitor.state.connections.peers = Array.from({ length: 5 }, (_, i) => ({ ...peer, pair_id: 'pair-' + i, device_name: 'Device ' + i }))
  visitor.emit()
  visitor.runtime.renamePeer.mockImplementation(async (id: string, name: string) => { visitor.state.connections.peers = visitor.state.connections.peers.map(peer => peer.pair_id === id ? { ...peer, local_name: name } : peer); visitor.emit() })
  render(<App initialEntries={['/']} />); await screen.findByRole('button', { name: 'Device 4' }); expect(document.querySelectorAll('.guest-peer')).toHaveLength(5)
  fireEvent.click(screen.getByRole('button', { name: 'Device 0' })); fireEvent.click(screen.getByRole('button', { name: '修改备注' }))
  fireEvent.change(await screen.findByLabelText('本机显示的名称'), { target: { value: 'Design Mac' } }); fireEvent.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => expect(visitor.runtime.renamePeer).toHaveBeenCalledWith('pair-0', 'Design Mac')); await screen.findByRole('button', { name: 'Design Mac' })
  expect(visitor.state.connections.peers[0].device_name).toBe('Device 0'); expect(visitor.state.connections.peers[1].local_name).toBeUndefined()
})
test('remove requires confirmation, blocks active work and keeps adding available', async () => {
  visitor.state.connections.peers = [peer]; visitor.emit(); render(<App initialEntries={['/']} />)
  fireEvent.click(await screen.findByRole('button', { name: 'MacBook Pro' })); fireEvent.click(screen.getByRole('button', { name: '移除连接' }))
  await screen.findByRole('heading', { name: '移除连接？' }); expect(visitor.runtime.removePeer).not.toHaveBeenCalled()
  act(() => { visitor.state.jobs = [job('paused')]; visitor.emit() })
  expect(await screen.findByText('请先结束当前传输。')).toBeInTheDocument(); expect(screen.queryByRole('button', { name: '移除连接' })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '关闭' })); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(await screen.findByRole('button', { name: '连接设备' })).toBeInTheDocument()
})
test('notification conflicts stay actionable and replies use the current request version', async () => {
  const transfer = { id: 'request-2', version: 3, state: 'PENDING', sender_owner_id: 'owner-a', sender_device_id: 'phone-a', file_name: 'Review.pdf' }; let conflict = true
  const fetcher = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => init?.method === 'POST' ? conflict ? { ok: false, status: 409, json: async () => ({ code: 'STATE_CONFLICT' }) } : { ok: true, json: async () => ({ ...transfer, state: 'ACCEPTED' }) } : { ok: true, json: async () => ({ tasks: [], transfers: [transfer] }) })
  vi.stubGlobal('fetch', fetcher); render(<App initialEntries={['/notifications']} />)
  fireEvent.click(await screen.findByRole('button', { name: /Review.pdf/ })); fireEvent.click(await screen.findByRole('button', { name: '拒绝' })); await screen.findByText('请求状态已变化，正在为你刷新。')
  conflict = false; fireEvent.click(screen.getByRole('button', { name: '接收' })); await waitFor(() => expect(fetcher.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(2))
  const call = fetcher.mock.calls.find(([, init]) => init?.method === 'POST'); expect(JSON.parse(String(call?.[1]?.body)).expected_version).toBe(3)
})
test('account task controls in merged file details preserve current versions', async () => {
  let task = { id: 'task-1', kind: 'DOWNLOAD', reference_id: 'request-1', state: 'RUNNING', progress_bytes: 1500000, total_bytes: 2400000, version: 1, updated_at: '2026-10-31T10:00:00Z' }; const actions: { action: string; version: number }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'POST') { const action = url.toString().split('/').pop()!; actions.push({ action, version: JSON.parse(String(init.body)).expected_version }); task = { ...task, state: action === 'pause' ? 'PAUSED' : action === 'resume' ? 'RUNNING' : 'CANCELLED', version: task.version + 1 }; return { ok: true, json: async () => task } }
    return { ok: true, json: async () => url.toString().includes('/v1/transfers') ? { transfers: [{ id: 'request-1', file_name: 'Review.pdf' }] } : { tasks: [task] } }
  }))
  render(<App initialEntries={['/tasks']} />); fireEvent.click(await screen.findByRole('button', { name: /Review.pdf/ })); fireEvent.click(await screen.findByRole('button', { name: '暂停' })); await screen.findByText('已暂停，进度已保留。')
  fireEvent.click(screen.getByRole('button', { name: '继续' })); await screen.findByText('已继续传输。')
  fireEvent.click(screen.getByRole('button', { name: '取消传输' })); fireEvent.click(screen.getByRole('button', { name: '确认' })); await screen.findByText('已取消任务。')
  expect(actions).toEqual([{ action: 'pause', version: 1 }, { action: 'resume', version: 2 }, { action: 'cancel', version: 3 }])
})
test('encrypted notification metadata never becomes a claimed plaintext name', async () => {
  const transfer = { id: 'encrypted', version: 1, state: 'PENDING', sender_owner_id: 'owner-a', sender_device_id: 'phone-a', file_name: 'encrypted', size_bytes: 127 }
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ tasks: [], transfers: [transfer] }) })))
  render(<App initialEntries={['/notifications']} />); fireEvent.click(await screen.findByRole('button', { name: /phone-a/ })); expect(await screen.findByText('加密文件')).toBeInTheDocument(); expect(screen.queryByText('encrypted')).not.toBeInTheDocument()
})

test('a completed upload in merged files means cloud storage, not recipient delivery', async () => {
  const task = { id: 'stored-task', kind: 'UPLOAD', reference_id: 'stored-entry', state: 'COMPLETED', progress_bytes: 100, total_bytes: 100, version: 2, updated_at: '2026-11-03T16:00:00Z' }
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => ({ ok: true, json: async () => url.toString().includes('/v1/transfers') ? { transfers: [{ id: 'stored-transfer', entry_id: 'stored-entry', file_name: 'Cloud.pdf' }] } : { tasks: [task] } })))
  render(<App initialEntries={['/files']} />)
  fireEvent.click(await screen.findByRole('button', { name: /Cloud.pdf.*已暂存到云端/ }))
  expect(await screen.findByText('已暂存到云端')).toBeInTheDocument()
  expect(screen.queryByText('已送达')).not.toBeInTheDocument()
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
