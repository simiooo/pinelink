import { StrictMode, useLayoutEffect } from 'react'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MotionOwner } from './owner'
import { MotionButton, MotionList, MotionMeter } from './components'
import { MotionProvider, useMotion } from './react'
import { confirmedPercent } from './tokens'
import { MotionSurface } from './surface'

it('an exiting surface cannot restore focus over a newly opened surface', async () => {
  vi.useFakeTimers()
  function Fixture({ open, identity }: { open: boolean; identity: string }) { return <MotionProvider><button>Choose file</button><MotionSurface key={identity} open={open} onClose={() => undefined} title="File" closeLabel="Close"><button>Send file</button></MotionSurface></MotionProvider> }
  const view = render(<Fixture open={false} identity="first" />)
  try {
    const trigger = screen.getByRole('button', { name: 'Choose file' }); trigger.focus()
    view.rerender(<Fixture open identity="first" />)
    const restore = vi.spyOn(trigger, 'focus')
    view.rerender(<Fixture open={false} identity="first" />)
    view.rerender(<Fixture open identity="second" />)
    await act(async () => { vi.advanceTimersByTime(250) })
    expect(screen.getByRole('button', { name: 'Send file' })).toBeInTheDocument()
    expect(restore).not.toHaveBeenCalled()
  } finally { view.unmount(); vi.useRealTimers() }
})

afterEach(() => { cleanup(); vi.restoreAllMocks() })

it('does not round partial confirmed bytes up to a complete transfer', () => {
  expect(confirmedPercent(.58)).toBe(58)
  expect(confirmedPercent(.99999)).toBe(99)
  expect(confirmedPercent(1)).toBe(100)
})

it('never invokes actions on StrictMode setup and gates two clicks until the actual promise settles', async () => {
  let finish!: () => void
  const action = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
  render(<StrictMode><MotionProvider><MotionButton onClick={action}>Send</MotionButton></MotionProvider></StrictMode>)
  expect(action).not.toHaveBeenCalled()
  const button = screen.getByRole('button', { name: 'Send' })
  fireEvent.click(button); fireEvent.click(button)
  expect(action).toHaveBeenCalledTimes(1)
  expect(button).toHaveAttribute('aria-busy', 'true')
  await act(async () => finish())
  expect(button).not.toBeDisabled()
  expect(button).not.toHaveAttribute('aria-busy')
})

it('late action completion after unmount cannot retain motion resources or invoke a second action', async () => {
  let owner!: MotionOwner, finish!: () => void
  function Probe() { owner = useMotion(); return null }
  const action = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
  const view = render(<StrictMode><MotionProvider><Probe /><MotionButton onClick={action}>Keep</MotionButton></MotionProvider></StrictMode>)
  fireEvent.click(screen.getByRole('button', { name: 'Keep' }))
  view.unmount(); await act(async () => finish())
  expect(action).toHaveBeenCalledTimes(1)
  expect(owner.diagnostics()).toMatchObject({ attached: 0, poses: 0, animations: 0, loops: 0, styles: 0 })
})

it('reports exact bounded meter values immediately through the shadcn/Radix progress primitive', () => {
  const view = render(<MotionProvider><MotionMeter value={.42} label="Storage" /></MotionProvider>)
  expect(screen.getByRole('progressbar', { name: 'Storage' })).toHaveAttribute('aria-valuenow', '42')
  view.rerender(<MotionProvider><MotionMeter value={Infinity} label="Storage" /></MotionProvider>)
  expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')
  view.rerender(<MotionProvider><MotionMeter value={1.3} label="Storage" /></MotionProvider>)
  expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
})

it('keyed list filtering preserves local input and releases removed element records', () => {
  let owner!: MotionOwner
  function Probe() { const current = useMotion(); useLayoutEffect(() => { owner = current }, [current]); return null }
  function Fixture({ ids }: { ids: string[] }) {
    return <StrictMode><MotionProvider><Probe /><MotionList identity={ids.join(',')}>
      {ids.map(id => <div key={id} data-motion-key={id}><input aria-label={id} defaultValue={id} /></div>)}
    </MotionList></MotionProvider></StrictMode>
  }
  const view = render(<Fixture ids={['a','b','c']} />)
  fireEvent.change(screen.getByLabelText('b'), { target: { value: 'draft still here' } })
  view.rerender(<Fixture ids={['c','b']} />)
  expect(screen.getByLabelText('b')).toHaveValue('draft still here')
  expect(owner.diagnostics().poses).toBeLessThanOrEqual(2)
  view.unmount(); expect(owner.diagnostics().poses).toBe(0)
})
