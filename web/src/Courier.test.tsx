import { render } from '@testing-library/react'
import { expect, test } from 'vitest'
import { Courier } from './components/Courier'
import type { CourierAction } from './components/Courier'

const actions: CourierAction[] = ['run', 'pause', 'store', 'notify', 'pull', 'seal', 'persist', 'permission', 'deny', 'complete']

test('all courier actions have a real state-bound representation', () => {
  for (const action of actions) {
    const { getByRole, unmount } = render(<Courier action={action} label={`状态 ${action}`} />)
    expect(getByRole('img', { name: `状态 ${action}` })).toHaveClass(`courier-${action}`)
    unmount()
  }
})
