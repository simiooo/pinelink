import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { MotionProvider } from '../motion/react'
import { I18nProvider } from '../i18n'
import { GuestGraph } from './Graph'
import type { GuestJob } from './store'
import type { GuestPeer } from './types'

const peer: GuestPeer = { pair_id: 'pair', owner_id: 'guest-peer', device_id: 'peer-device', device_name: 'Peer phone', encryption_key: 'key', signing_key: 'key', expires_at: '2099-01-01T00:00:00Z', online: false }
const job = (): GuestJob => ({ id: 'job', created: 0, direction: 'receive', peer, received: 35, name: 'file.bin', plainSize: 100, phase: 'running', transfer: { id: 'transfer', state: 'ACCEPTED', version: 2, file_name: 'file.bin', size_bytes: 100, content_root: 'root', encryption_suite: 'suite', sender_owner_id: 'guest-peer', sender_device_id: 'peer-device', recipient_owner_id: 'guest-self', recipient_device_id: 'self', expires_at: peer.expires_at, received_bytes: 35 }, snapshot: { transfer_id: 'transfer', state: 'ACCEPTED', state_version: 2, control_revision: 1, progress_revision: 1, delivery: { state: 'RUNNING', acknowledged_bytes: 35, total_bytes: 100, revision: 1 }, spool: { state: 'SEALED', acknowledged_bytes: 100, total_bytes: 100 }, completion: { delivered: false, stored: true }, capabilities: { pause: true, resume: false, cancel: false, restore: false } } })
const view = (value?: GuestJob) => <I18nProvider initialPreference="en-US"><MotionProvider><GuestGraph peers={[peer]} job={value} onConnect={() => {}} onConversation={() => {}} onDetails={() => {}} dragging={false} onFileDrop={() => {}} localDevice="self" /></MotionProvider></I18nProvider>
afterEach(cleanup)

it('labels the remote identity independently of the local transfer slot', () => {
  const accountPeer = { ...peer, pair_id: 'account', owner_id: 'acct-peer', device_name: 'Account peer' }
  const visitorPeer = { ...peer, pair_id: 'visitor', authority: 'ACCOUNT' as const, device_name: 'Visitor peer' }
  const { container } = render(<I18nProvider initialPreference="en-US"><MotionProvider><GuestGraph peers={[accountPeer, visitorPeer]} onConnect={() => {}} onConversation={() => {}} onDetails={() => {}} dragging={false} onFileDrop={() => {}} localDevice="account-self" localAccount /></MotionProvider></I18nProvider>)
  const nodes = container.querySelectorAll('.guest-peer')
  expect(nodes[0].querySelector('.guest-account-icon')).toBeInTheDocument()
  expect(nodes[1].querySelector('.guest-account-icon')).not.toBeInTheDocument()
  expect(container.querySelector('.guest-local .guest-account-icon')).toBeInTheDocument()
})

it('opens one conversation action while retaining device management', () => {
  const open = vi.fn()
  render(<I18nProvider initialPreference="en-US"><MotionProvider><GuestGraph peers={[peer]} onConnect={() => {}} onConversation={open} onDetails={() => {}} dragging={false} onFileDrop={() => {}} localDevice="self" /></MotionProvider></I18nProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Peer phone' }))
  const actions = within(screen.getByRole('group', { name: 'Device actions' }))
  expect(actions.getAllByRole('button')).toHaveLength(3)
  fireEvent.click(actions.getByRole('button', { name: 'Conversation' }))
  expect(open).toHaveBeenCalledWith(peer)
})

it('keeps the endpoint device and an independent cloud node with two real byte ledgers', () => {
  const { container } = render(view(job()))
  expect(container.querySelector('.guest-peers .guest-node-label')).toHaveTextContent('Peer phone')
  expect(container.querySelectorAll('.guest-node')).toHaveLength(3)
  expect(screen.getByRole('button', { name: 'Cloud · Transfer details' })).toBeInTheDocument()
  expect(screen.getByRole('progressbar', { name: 'Cloud copy' })).toHaveAttribute('aria-valuenow', '100')
  expect(screen.getByRole('progressbar', { name: 'Received' })).toHaveAttribute('aria-valuenow', '35')
  expect(container.querySelector('.guest-peer')).toHaveAttribute('data-online', 'false')
})

it('opens device management on a route endpoint and details on the cloud', () => {
  const details = vi.fn(), conversation = vi.fn()
  render(<I18nProvider initialPreference="en-US"><MotionProvider><GuestGraph peers={[peer]} job={job()} onConnect={() => {}} onConversation={conversation} onDetails={details} dragging={false} onFileDrop={() => {}} localDevice="self" /></MotionProvider></I18nProvider>)
  fireEvent.click(screen.getByRole('button', { name: 'Peer phone' }))
  expect(details).not.toHaveBeenCalled()
  const actions = within(screen.getByRole('group', { name: 'Device actions' }))
  expect(actions.getByRole('button', { name: 'Rename' })).toBeInTheDocument()
  expect(actions.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument()
  fireEvent.click(actions.getByRole('button', { name: 'Conversation' }))
  expect(conversation).toHaveBeenCalledWith(peer)
  fireEvent.click(screen.getByRole('button', { name: 'Cloud · Transfer details' }))
  expect(details).toHaveBeenCalledOnce()
})

it('stops the paused download while allowing a separately confirmed cloud upload to continue', () => {
  const value = job(); value.phase = 'paused'; value.snapshot!.delivery.state = 'PAUSED'
  value.snapshot!.completion.stored = false; value.snapshot!.spool = { state: 'SPOOLING', acknowledged_bytes: 58, total_bytes: 100 }
  const { container } = render(view(value))
  expect(container.querySelector('[data-ledger=spool]')).toHaveAttribute('data-running', 'true')
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-running', 'false')
  expect(screen.getByRole('progressbar', { name: 'Cloud copy' })).toHaveAttribute('aria-valuenow', '58')
  expect(screen.getByRole('progressbar', { name: 'Received' })).toHaveAttribute('aria-valuenow', '35')
})

it('does not imply recipient completion when the entire cloud copy is stored', () => {
  const value = job(); value.direction = 'send'; value.phase = 'waiting'; value.snapshot!.delivery.acknowledged_bytes = 0; value.snapshot!.delivery.state = 'WAITING_FOR_ACCEPTANCE'
  const { container, rerender } = render(view(value)); const endpoint = container.querySelector('.guest-peer')
  expect(screen.getByRole('progressbar', { name: 'Cloud copy' })).toHaveAttribute('aria-valuenow', '100')
  expect(screen.getByRole('progressbar', { name: 'Received' })).toHaveAttribute('aria-valuenow', '0')
  expect(container.querySelectorAll('.guest-curve[data-running=true]')).toHaveLength(0)
  rerender(view())
  expect(container.querySelector('.guest-cloud')).not.toBeInTheDocument()
  expect(container.querySelector('.guest-peer')).toBe(endpoint)
})

it('draws one real direct delivery ledger and freezes it while paused', () => {
  const value = job(); value.transport = { state: 'direct' }; value.snapshot!.route = { kind: 'DIRECT', epoch: 1 }
  value.snapshot!.spool.state = 'NOT_REQUIRED'; value.snapshot!.completion.stored = false
  const { container, rerender } = render(view(value)), curve = container.querySelector('[data-ledger=delivery]')
  expect(container.querySelector('.guest-network')).toHaveAttribute('data-route', 'direct')
  expect(container.querySelectorAll('.guest-node')).toHaveLength(2)
  expect(container.querySelectorAll('.guest-curve')).toHaveLength(1)
  expect(curve).toHaveAttribute('data-blue', '0.35')
  expect(curve).toHaveAttribute('data-running', 'true')
  value.phase = 'paused'; value.snapshot!.delivery.state = 'PAUSED'; rerender(view(value))
  expect(container.querySelector('[data-ledger=delivery]')).toBe(curve)
  expect(curve).toHaveAttribute('data-running', 'false')
  expect(screen.getByRole('button', { name: 'Paused' })).toBeInTheDocument()
})
it('retains acknowledged bytes and the same device nodes while switching to the actual cloud route', () => {
  const value = job(); value.transport = { state: 'switching' }; value.snapshot!.route = { kind: 'DIRECT', epoch: 1 }
  value.snapshot!.spool.state = 'NOT_REQUIRED'; value.snapshot!.completion.stored = false
  const { container, rerender } = render(view(value)), endpoint = container.querySelector('.guest-peer')
  expect(container.querySelector('.guest-cloud')).not.toBeInTheDocument()
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-blue', '0.35')
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-running', 'false')
  value.transport.state = 'cloud'; value.snapshot!.route = { kind: 'RELAY', epoch: 2 }
  value.snapshot!.spool = { state: 'SPOOLING', acknowledged_bytes: 58, total_bytes: 100 }; rerender(view(value))
  expect(container.querySelector('.guest-peer')).toBe(endpoint)
  expect(container.querySelectorAll('.guest-node')).toHaveLength(3)
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-blue', '0.35')
  expect(container.querySelector('[data-ledger=spool]')).toHaveAttribute('data-blue', '0.58')
})
it('does not resurrect a cached direct connection after a reload', () => {
  const value = job(); value.snapshot!.spool.state = 'NOT_REQUIRED'; value.snapshot!.completion.stored = false; value.snapshot!.route = { kind: 'DIRECT', epoch: 1 }
  const { container } = render(view(value))
  expect(container.querySelector('.guest-network')).toHaveAttribute('data-route', 'connecting')
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-running', 'false')
})
it('keeps the verified delivery path distinct from an independent cloud backup', () => {
  const value = job(); value.phase = 'complete'; value.snapshot!.completion.delivered = true; value.snapshot!.route = { kind: 'DIRECT', epoch: 1 }
  const { container } = render(view(value))
  expect(container.querySelector('.guest-network')).toHaveAttribute('data-route', 'direct')
  expect(container.querySelector('.guest-cloud')).not.toBeInTheDocument()
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-blue', '1')
})
it('keeps a still incomplete cloud copy independent after confirmed delivery', () => {
  const value = job(); value.phase = 'complete'; value.snapshot!.completion.delivered = true
  value.snapshot!.completion.stored = false; value.snapshot!.spool = { state: 'SPOOLING', acknowledged_bytes: 58, total_bytes: 100 }
  const { container } = render(view(value))
  expect(container.querySelector('[data-ledger=delivery]')).toHaveAttribute('data-blue', '1')
  expect(container.querySelector('[data-ledger=spool]')).toHaveAttribute('data-blue', '0.58')
})
