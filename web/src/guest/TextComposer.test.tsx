import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MotionProvider } from '../motion/react'
import { TextComposer } from './TextComposer'
import { textByteLimit } from './crypto'
const labels = { text: 'Text', sensitive: 'Sensitive', input: 'Message', send: 'Send', tooLong: 'Too long', invalidText: 'Invalid text', unsupported: 'Unsupported', sendFile: 'Send as file', sensitiveFileWarning: 'File contents can be saved and displayed.', confirm: 'Confirm', cancel: 'Cancel' }
const renderComposer = (value: string, supported = true, sensitivity: 'NORMAL' | 'SENSITIVE' = 'NORMAL', onSend = vi.fn(), onSendFile = vi.fn()) => <MotionProvider><TextComposer value={value} sensitivity={sensitivity} supported={supported} labels={labels} onValue={() => {}} onSensitivity={() => {}} onSend={onSend} onSendFile={onSendFile}/></MotionProvider>
afterEach(cleanup)
it('counts UTF-8 bytes, rejects invalid Unicode and keeps the original input', () => {
  const send = vi.fn(), { rerender } = render(renderComposer('é'.repeat(textByteLimit / 2), true, 'NORMAL', send))
  expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled(); fireEvent.click(screen.getByRole('button', { name: 'Send' })); expect(send).toHaveBeenCalledOnce()
  rerender(renderComposer('é'.repeat(textByteLimit / 2 + 1))); expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled(); expect(screen.getByText('Too long')).toBeInTheDocument()
  rerender(renderComposer('\ud800')); expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled(); expect(screen.getByText('Invalid text')).toBeInTheDocument()
})
it('does not send to an unsupported client and requires explicit consent before sensitive file fallback', () => {
  const send = vi.fn(), fallback = vi.fn(), { rerender } = render(renderComposer('secret', false, 'SENSITIVE', send, fallback))
  expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled(); fireEvent.click(screen.getByRole('button', { name: /Send as file/ }))
  expect(fallback).not.toHaveBeenCalled(); expect(screen.getByText(labels.sensitiveFileWarning)).toBeInTheDocument()
  rerender(renderComposer('changed', false, 'SENSITIVE', send, fallback)); expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /Send as file/ })); fireEvent.click(screen.getByRole('button', { name: 'Confirm' })); expect(fallback).toHaveBeenCalledOnce(); expect(send).not.toHaveBeenCalled()
})
