import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { App } from './main'
import zh from '../../i18n/zh-CN.json'
import en from '../../i18n/en-US.json'
import { fileMetadataLabel, formatTime } from './format'

afterEach(cleanup)

function mockApi() {
  const task = { id: 'task-1', kind: 'DOWNLOAD', reference_id: 'request-1', state: 'RUNNING', progress_bytes: 1500000, total_bytes: 2400000, version: 1, updated_at: '2026-10-31T10:00:00Z' }
  const transfer = { id: 'request-1', version: 1, state: 'PENDING', sender_owner_id: 'team-design', sender_device_id: 'macbook-pro', file_name: 'design.pdf', size_bytes: 2400000 }
  vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => {
    const path = url.toString()
    if (path.includes('/v1/notifications')) return { ok: true, json: async () => ({ transfers: [transfer] }) }
    if (path.includes('/v1/devices')) return { ok: true, json: async () => ({ devices: [{ device_id: 'dev-device', owner_id: 'dev-owner', status: 'ACTIVE', last_seen_at: '2026-10-31T10:00:00Z' }] }) }
    if (path.includes('/v1/quota')) return { ok: true, json: async () => ({ quota_bytes: 1000, used_bytes: 900 }) }
    if (path.includes('/v1/transfers')) return { ok: true, json: async () => ({ transfers: [{ ...transfer, entry_id: 'entry-1', sender_owner_id: 'dev-owner' }] }) }
    if (path.includes('/v1/files/')) return { ok: true, json: async () => ({ id: 'entry-1', file_name: 'design.pdf', state: 'AVAILABLE', version: 4, retention_class: 'TEMP_24H' }) }
    if (path.includes('include_deleted=true')) return { ok: true, json: async () => ({ tasks: [{ ...task, state: 'CANCELLED', error_code: 'TRANSFER_CANCELLED' }] }) }
    return { ok: true, json: async () => ({ tasks: [task] }) }
  }))
}

describe('i18n', () => {
  test('both locales have the same message keys and localized metadata', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
    expect(fileMetadataLabel('encrypted', 127, 'en-US')).toBe('The name will be visible after local decryption · 127 B')
    expect(formatTime('2026-10-31T10:00:00Z', 'en-US')).toMatch(/AM|PM/)
    expect(formatTime('2026-10-31T10:00:00Z', 'zh-CN')).not.toMatch(/AM|PM/)
  })

  test('English UI switches to Chinese and restores the saved preference', async () => {
    const first = render(<App initialEntries={['/']} localePreference="en-US" />)
    expect(await screen.findByRole('heading', { name: 'Send files' })).toBeInTheDocument()
    expect(document.documentElement.lang).toBe('en-US')
    fireEvent.keyDown(screen.getByRole('button', { name: 'More' }), { key: 'ArrowDown' })
    fireEvent.click(await screen.findByRole('menuitemradio', { name: en.languageZh }))
    expect(await screen.findByRole('heading', { name: '发送文件' })).toBeInTheDocument()
    expect(localStorage.getItem('pinelink.locale')).toBe('zh-CN')
    first.unmount()
    render(<App initialEntries={['/']} />)
    expect(await screen.findByRole('heading', { name: '发送文件' })).toBeInTheDocument()
  })

  test('system locale follows a language change', async () => {
    render(<App initialEntries={['/']} localePreference="system" />)
    Object.defineProperty(navigator, 'languages', { value: ['en-US'], configurable: true })
    fireEvent(window, new Event('languagechange'))
    expect(await screen.findByRole('heading', { name: 'Send files' })).toBeInTheDocument()
  })

  test('task, notification, history and settings are localized beyond the home page', async () => {
    mockApi()
    const routes: [string, string, string[]][] = [
      ['/tasks', 'Current task', ['Pause', 'Cancel']],
      ['/notifications', 'Notifications', ['Sending device', 'Sending account', 'Accept', 'Reject']],
      ['/history', 'History & issues', ['The sender cancelled this transfer. The file is not marked as received.']],
      ['/settings', 'Devices & storage', ['Storage usage', 'Keep permanently', 'Storage is nearly full. Remove files you no longer need first.']],
    ]
    for (const [route, title, labels] of routes) {
      const view = render(<App initialEntries={[route]} localePreference="en-US" />)
      expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument()
      if (route === '/tasks') {
        expect(await screen.findByRole('progressbar', { name: 'Actual transfer progress' })).toHaveAttribute('aria-valuenow', '62')
        fireEvent.click(screen.getByRole('button', { name: 'Transfer details' }))
      }
      for (const label of labels) {
        if (label === 'Actual transfer progress' || label === 'Storage usage') expect(await screen.findByRole('progressbar', { name: label })).toBeInTheDocument()
        else expect(await screen.findByText(label)).toBeInTheDocument()
      }
      view.unmount()
    }
  })
})
