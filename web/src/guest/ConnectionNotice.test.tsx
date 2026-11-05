import { fireEvent, render, screen, waitFor, cleanup } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MotionProvider } from '../motion/react'
import { I18nProvider } from '../i18n'
import { ConnectionNotice } from './ConnectionNotice'
import type { ConnectionFault } from './connection-diagnostics'

afterEach(cleanup)
it('opens scoped failure details and copies the diagnostic result without another server request', async () => {
  const diagnostics = vi.fn(async () => ({ channels: [{ channel: 'visitor', keys: { signing: 'mismatch' } }] })), writeText = vi.fn(async (_text: string) => {})
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  const fault: ConnectionFault = { channel: 'visitor', stage: 'connections', code: 'DEVICE_UNAUTHORIZED', status: 401, kind: 'credentials', at: 1 }
  render(<I18nProvider initialPreference="zh-CN"><MotionProvider><ConnectionNotice message="连接身份校验失败" visible faults={[fault]} retry={async () => {}} diagnostics={diagnostics} /></MotionProvider></I18nProvider>)
  expect(diagnostics).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: '查看连接状态' }))
  expect(await screen.findByRole('dialog')).toHaveTextContent('访客连接 · 设备列表')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('dialog')).toHaveTextContent('DEVICE_UNAUTHORIZED · HTTP 401')
  await waitFor(() => expect(screen.getByRole('button', { name: '复制诊断' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: '复制诊断' }))
  await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1))
  expect(JSON.parse(writeText.mock.calls[0][0])).toMatchObject({ schema: 'pinelink-connection-v1', failures: [fault], channels: [{ channel: 'visitor', keys: { signing: 'mismatch' } }] })
  expect(diagnostics).toHaveBeenCalledTimes(1)
})
