import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MotionProvider } from '../motion/react'
import { TextBubble } from './TextBubble'
const labels = { copy: 'Copy', copied: 'Copied', copyFailed: 'Select text to copy', sensitive: 'Sensitive', reveal: 'Reveal', hide: 'Hide' }
const view = (id: string, value: string, sensitive = false) => <MotionProvider><TextBubble id={id} value={value} sensitive={sensitive} labels={labels} /></MotionProvider>
afterEach(() => { cleanup(); vi.restoreAllMocks() })
it('renders Markdown without HTML, remote images or unsafe links and copies the original', async () => {
  const raw = '# Heading\n\n**bold** [safe](https://example.com) [bad](javascript:alert(1)) ![remote](https://example.com/pixel.png)\n\n<script>alert(1)</script>'
  const writeText = vi.fn().mockResolvedValue(undefined); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  const { container } = render(view('one', raw))
  expect(screen.getByRole('heading', { name: 'Heading' })).toBeInTheDocument()
  expect(screen.getByText('bold').tagName).toBe('STRONG')
  expect(screen.getByRole('link', { name: 'safe' })).toHaveAttribute('rel', 'noopener noreferrer')
  expect(screen.queryByRole('link', { name: 'bad' })).not.toBeInTheDocument()
  expect(container.querySelector('img,script')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(raw)); expect(await screen.findByRole('status')).toHaveTextContent('Copied')
})
it('masks sensitive text until requested and reports clipboard failure without claiming success', async () => {
  const writeText = vi.fn().mockRejectedValue(new Error('denied')); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  const { rerender } = render(view('one', 'private value', true))
  expect(screen.queryByText('private value')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Copy' })); expect(await screen.findByRole('status')).toHaveTextContent('Select text to copy')
  expect(screen.queryByText('Copied')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Reveal' })); expect(screen.getByText('private value')).toBeInTheDocument()
  rerender(view('two', 'another private value', true)); expect(screen.queryByText('another private value')).not.toBeInTheDocument()
})
