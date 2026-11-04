import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { DeviceProfile } from '../data/crypto'
import { MotionProvider } from '../motion/react'
import type { GuestJob } from './store'
import { Conversation } from './Conversation'

const labels = { copy: 'Copy', copied: 'Copied', copyFailed: 'Copy failed', sensitive: 'Sensitive', reveal: 'Reveal', hide: 'Hide', unavailable: 'Unavailable', waiting: 'Waiting', sent: 'Sent', received: 'Received', stored: 'Stored', file: 'File' }
afterEach(cleanup)
it('shows file messages in chronological order and opens their existing details', () => {
  const jobs = [
    { id: 'later', created: 2, direction: 'send', name: 'Report.pdf', plainSize: 1024, phase: 'complete' },
    { id: 'earlier', created: 1, direction: 'receive', name: 'Notes.txt', plainSize: 5, phase: 'complete' },
  ] as GuestJob[]
  const onJob = vi.fn()
  render(<MotionProvider><Conversation jobs={jobs} profile={{} as DeviceProfile} labels={labels} locale="en-US" onJob={onJob} /></MotionProvider>)
  const buttons = screen.getAllByRole('button')
  expect(buttons.map(button => button.textContent)).toEqual(['Notes.txt5 B', 'Report.pdf1.0 KB'])
  fireEvent.click(buttons[1]); expect(onJob).toHaveBeenCalledWith(jobs[0])
  expect(screen.getByText('Sent')).toBeInTheDocument()
})
