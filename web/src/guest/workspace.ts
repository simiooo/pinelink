import { authState, subscribeAuth } from '../auth/client'
import type { ClientRuntime } from '../data/runtime'
import { getGuestRuntime, GuestError, GuestRuntime } from './runtime'
import type { GuestPeer } from './types'
import { registerUpdateGuard } from '../pwa/guard'

type Snapshot = ReturnType<GuestRuntime['getSnapshot']>

// Visitor, account and local data retain their own stores and signing keys.
// The wrapper only combines their views and routes each action back to its owner.
export class WorkspaceRuntime {
  private account?: GuestRuntime
  private reader?: ClientRuntime
  private readerKey?: string
  private accountUnsubscribe?: () => void
  private guestUnsubscribe: () => void
  private authUnsubscribe: () => void
  private opening?: Promise<void>
  private generation = 0
  private disposed = false
  private source?: GuestRuntime
  private accountError?: string
  private listeners = new Set<() => void>()
  private snapshot: Snapshot
  private releaseUpdateGuard: () => void

  constructor(readonly guest: GuestRuntime) {
    this.snapshot = guest.getSnapshot()
    this.releaseUpdateGuard = registerUpdateGuard(() => !this.opening && this.snapshot.ready && !this.snapshot.busy && !this.snapshot.jobs.some(job => !['complete', 'stopped'].includes(job.phase)) && !['preparing', 'show', 'reply', 'verify', 'waiting'].includes(this.local?.getSnapshot?.().phase ?? 'idle'))
    this.guestUnsubscribe = guest.subscribe(() => this.refresh())
    this.authUnsubscribe = subscribeAuth(() => {
      const auth = authState(), current = this.reader
      if (current && ((!auth.session && !auth.offlineAccount) || (auth.session && (auth.session.account.id !== current.owner || auth.session.device_id !== current.profile.device || auth.session.csrf !== current.access.csrf)))) void this.setAccount(undefined)
    })
  }
  get local() { return this.guest.local }
  get profile() { return this.guest.profile }
  get accountProfile() { return this.account?.profile }
  get hasAccount() { return !!this.account }
  now = () => this.guest.now()
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener) } }
  getSnapshot = () => this.snapshot

  private refresh() {
    if (this.disposed) return
    const guest = this.guest.getSnapshot(), account = this.account?.getSnapshot()
    if (!account) this.snapshot = this.accountError ? { ...guest, error: this.accountError } : guest
    else {
      const accountPeers = account.connections.peers.map(peer => ({ ...peer, authority: 'ACCOUNT' as const, online: account.offline || Date.now() - (account.checkedAt ?? 0) > 15000 ? undefined : peer.online }))
      const guestPeers = guest.connections.peers.map(peer => peer.authority === 'LOCAL_DEVICE' || !guest.offline && Date.now() - (guest.checkedAt ?? 0) <= 15000 ? peer : { ...peer, online: undefined })
      const accountJobs = account.jobs.map(job => ({ ...job, authority: 'ACCOUNT' as const, peer: { ...job.peer, authority: 'ACCOUNT' as const } }))
      this.snapshot = {
        ...guest,
        connected: guest.connected || account.connected,
        busy: guest.busy || account.busy,
        error: guest.error ?? account.error ?? this.accountError,
        checkedAt: Math.max(guest.checkedAt ?? 0, account.checkedAt ?? 0),
        invite: this.source === this.guest ? guest.invite : account.invite,
        connections: { invites: [...guest.connections.invites, ...account.connections.invites], candidates: [...guest.connections.candidates, ...account.connections.candidates.map(candidate => ({ ...candidate, authority: 'ACCOUNT' as const }))], peers: [...guestPeers, ...accountPeers] },
        jobs: [...guest.jobs, ...accountJobs].sort((a, b) => b.created - a.created),
      }
    }
    for (const listener of this.listeners) listener()
  }

  async setAccount(reader?: ClientRuntime) {
    const key = reader ? [reader.server, reader.owner, reader.profile.device, reader.access.localOnly ? 'offline' : reader.access.csrf].join('\n') : undefined
    if (this.readerKey === key && (!reader || this.account || this.opening)) { this.reader = reader; return this.opening }
    const generation = ++this.generation, previous = this.account
    this.reader = reader; this.readerKey = key; this.account = undefined; this.accountError = undefined
    this.accountUnsubscribe?.(); this.accountUnsubscribe = undefined
    if (this.source === previous) this.source = undefined
    this.refresh()
    if (previous) await previous.close()
    if (this.disposed || generation !== this.generation) return
    if (!reader) { this.opening = undefined; return }
    const opening = GuestRuntime.openAccount(reader, this.guest.local).then(async account => {
      if (this.disposed || generation !== this.generation) { await account.close(); return }
      this.account = account
      this.accountUnsubscribe = account.subscribe(() => this.refresh())
      this.refresh()
    }).catch(error => {
      if (!this.disposed && generation === this.generation) { this.accountError = error instanceof Error ? error.message : 'ACCOUNT_UNAVAILABLE'; this.refresh() }
      throw error
    }).finally(() => { if (generation === this.generation) this.opening = undefined })
    this.opening = opening
    return opening
  }

  private async connectionRuntime() {
    if (!authState().session) return this.guest
    if (this.opening) await this.opening
    if (!this.account) throw new GuestError('ACCOUNT_UNAVAILABLE')
    return this.account
  }
  async connect() { const runtime = await this.connectionRuntime(); this.source = runtime; this.refresh(); return runtime.connect() }
  async join(value: string) { const runtime = await this.connectionRuntime(); this.source = runtime; this.refresh(); return runtime.join(value) }
  async ensureSession() { return (await this.connectionRuntime()).ensureSession() }
  async poll() { await Promise.all([this.guest.poll(), ...(this.account ? [this.account.poll()] : [])]) }
  private peerRuntime(pair: string) { return this.account?.getSnapshot().connections.peers.some(peer => peer.pair_id === pair) ? this.account : this.guest }
  private jobRuntime(id: string) { return this.account?.getSnapshot().jobs.some(job => job.id === id) ? this.account : this.guest }
  respondCandidate(id: string, approve: boolean) { return (this.account?.getSnapshot().connections.candidates.some(candidate => candidate.id === id) ? this.account : this.guest).respondCandidate(id, approve) }
  renamePeer(pair: string, name: string) { return this.peerRuntime(pair).renamePeer(pair, name) }
  removePeer(pair: string) { return this.peerRuntime(pair).removePeer(pair) }
  send(file: File, peer: GuestPeer, maxBytes: number, text?: { sensitivity: 'NORMAL' | 'SENSITIVE' }) {
    if (peer.authority === 'ACCOUNT') {
      if (!this.account) throw new GuestError('ACCOUNT_UNAVAILABLE')
      return this.account.send(file, peer, maxBytes, text)
    }
    return this.guest.send(file, peer, maxBytes, text)
  }
  accept(id: string) { return this.jobRuntime(id).accept(id) }
  reject(id: string) { return this.jobRuntime(id).reject(id) }
  control(id: string, action: 'pause' | 'resume' | 'cancel') { return this.jobRuntime(id).control(id, action) }
  retry(id: string) { return this.jobRuntime(id).retry(id) }
  dismiss(id: string) { return this.jobRuntime(id).dismiss(id) }
  download(id: string) { return this.jobRuntime(id).download(id) }

  dispose() {
    this.disposed = true; this.generation++; this.guestUnsubscribe(); this.authUnsubscribe(); this.accountUnsubscribe?.(); this.listeners.clear()
    if (this.account) void this.account.close().finally(this.releaseUpdateGuard)
    else this.releaseUpdateGuard()
    this.account = undefined
  }
}

let workspace: Promise<WorkspaceRuntime> | undefined
export const getWorkspaceRuntime = () => workspace ??= getGuestRuntime().then(guest => new WorkspaceRuntime(guest)).catch(error => { workspace = undefined; throw error })
