import { API_URL } from '../api/client'
import { authState } from '../auth/client'
import { assertCryptoContext, base64, digest, utf8 } from '../data/crypto'
import type { DeviceProfile } from '../data/crypto'
import type { ClientRuntime } from '../data/runtime'
import { GuestStore } from './store'
import type { GuestJob } from './store'
import type { GuestLease, GuestConnections, GuestInvite, GuestCandidate, GuestPeer, GuestTransfer, ProgressSnapshot } from './types'
import { decodeFileFrame, decodeText, decryptDare, encodeFileFrame, hashBlob, openManifest, prepareFile, safeFileName, suite, wireChunkSize } from './crypto'
import { setGuestHistoryAvailable, setGuestWorkPending } from './continuity'
import { updateFrozen } from '../pwa/guard'
import { DirectLink, DirectUnavailable, fallbackSignal } from './direct'
import type { DirectProof } from './direct'
import { LocalRuntime } from '../local/runtime'
import { restoreTextHistory } from './restore-text'
import { checkStoredKeys, connectionFault, connectionStage, requestStage } from './connection-diagnostics'
import type { ConnectionFault, ConnectionStage } from './connection-diagnostics'
import { reconcileGuestPeers, saveGuestLease } from './session-recovery'

export class GuestError extends Error { constructor(public code: string, public status = 0) { super(code) } }
type Snapshot = { ready: boolean; connected: boolean; busy: boolean; offline: boolean; error?: string; fault?: ConnectionFault; checkedAt?: number; connections: GuestConnections; jobs: GuestJob[]; invite?: GuestInvite }
type ControlledTask = { version: number; state: string; progress_bytes: number; total_bytes: number; progress_revision: number }
const emptyConnections = (): GuestConnections => ({ invites: [], candidates: [], peers: [] })
const activeJob = (job: GuestJob) => !['complete', 'stopped'].includes(job.phase)
const hasSpool = (job: GuestJob) => !!job.snapshot && !['NOT_STARTED', 'NOT_REQUIRED'].includes(job.snapshot.spool.state)
const progressPhase = (snapshot: ProgressSnapshot): GuestJob['phase'] => snapshot.completion.delivered ? 'complete' : [snapshot.state, snapshot.delivery.state].some(state => ['CANCELLED', 'REJECTED', 'EXPIRED', 'FAILED', 'DELETED'].includes(state)) ? 'stopped' : snapshot.delivery.state === 'PAUSED' ? 'paused' : snapshot.delivery.state === 'VERIFYING' ? 'verifying' : snapshot.state === 'PENDING' ? 'waiting' : 'running'
const deviceName = () => /iPhone/.test(navigator.userAgent) ? 'iPhone' : /iPad/.test(navigator.userAgent) ? 'iPad' : /Android/.test(navigator.userAgent) ? 'Android' : /Macintosh/.test(navigator.userAgent) ? 'Mac' : /Windows/.test(navigator.userAgent) ? 'Windows PC' : 'Web browser'
const wait = (ms: number, signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
  if (signal?.aborted) { reject(signal.reason); return }
  const finish = () => { signal?.removeEventListener('abort', abort); resolve() }, timer = setTimeout(finish, ms)
  const abort = () => { clearTimeout(timer); signal?.removeEventListener('abort', abort); reject(signal?.reason) }
  signal?.addEventListener('abort', abort, { once: true })
})

export class GuestRuntime {
  local!: LocalRuntime
  private snapshot: Snapshot = { ready: false, connected: false, busy: false, offline: navigator.onLine === false, connections: emptyConnections(), jobs: [] }
  private listeners = new Set<() => void>()
  private lease?: GuestLease
  private enrollment?: Promise<void>
  private polling?: Promise<void>
  private workers = new Map<string, AbortController>()
  private commands = new Map<string, Promise<unknown>>()
  private stopped = false
  private lifetime = new AbortController()
  private workerTasks = new Set<Promise<unknown>>()
  private closing?: Promise<void>
  private timer?: ReturnType<typeof setTimeout>
  private failures = 0
  private lastFault?: ConnectionFault
  private channel?: BroadcastChannel
  private constructor(readonly server: string, readonly store: GuestStore, readonly profile: DeviceProfile, private readonly account?: { owner: string; csrf?: string; offset: number }) {}
  static async open(server = API_URL,localOnly=false) {
    assertCryptoContext()
    const absolute = new URL(server, location.href).href.replace(/\/$/, '')
    const store = new GuestStore('pinelink-visitor:' + await digest(utf8.encode(absolute)))
    try {
      const runtime = new GuestRuntime(absolute, store, await store.profile())
      runtime.local = await LocalRuntime.open(store, () => runtime.changed())
      runtime.lease = await store.leases.get('self')
      await restoreTextHistory(store, runtime.profile, runtime.local.profile)
      // Persisted route history is not a live PeerConnection after reopening.
      await store.jobs.filter(job => job.authority !== 'LOCAL_DEVICE' && activeJob(job) && job.transport?.state === 'direct').modify(job => { job.transport = { state: 'connecting', proof: job.transport?.proof } })
      await runtime.refreshLocal()
      runtime.snapshot = { ...runtime.snapshot, ready: true, connected: !!runtime.lease && runtime.lease.device_id === runtime.profile.device }
      if (typeof BroadcastChannel !== 'undefined') { runtime.channel = new BroadcastChannel(store.name); runtime.channel.onmessage = () => { void runtime.refreshLocal().catch(() => undefined) } }
      window.addEventListener('online', runtime.wake); window.addEventListener('offline', runtime.wake)
      document.addEventListener('visibilitychange', runtime.visibility)
      if (runtime.lease&&!localOnly) void runtime.ensureSession().then(() => runtime.poll()).catch(error => runtime.noteError(error))
      return runtime
    } catch (error) { store.close(); throw error }
  }
  static async openAccount(reader: ClientRuntime, local: LocalRuntime) {
    assertCryptoContext()
    if (reader.access.localOnly && authState().offlineAccount?.id !== reader.owner) throw new GuestError('ACCOUNT_SIGN_IN_REQUIRED', 401)
    const store = new GuestStore('pinelink-account-conversation:' + await digest(utf8.encode(`${reader.server}\n${reader.owner}\n${reader.profile.device}`)))
    try {
      const runtime = new GuestRuntime(reader.server, store, reader.profile, { owner: reader.owner, csrf: reader.access.csrf, offset: (await reader.store.target()).offset })
      runtime.local = local
      await restoreTextHistory(store, runtime.profile, local.profile)
      if (!reader.access.localOnly) await runtime.ensureSession()
      await store.jobs.filter(job => job.authority !== 'LOCAL_DEVICE' && activeJob(job) && job.transport?.state === 'direct').modify(job => { job.transport = { state: 'connecting', proof: job.transport?.proof } })
      await runtime.refreshLocal()
      runtime.snapshot = { ...runtime.snapshot, ready: true, connected: !!runtime.lease }
      if (typeof BroadcastChannel !== 'undefined') { runtime.channel = new BroadcastChannel(store.name); runtime.channel.onmessage = () => { void runtime.refreshLocal().catch(() => undefined) } }
      window.addEventListener('online', runtime.wake); window.addEventListener('offline', runtime.wake)
      document.addEventListener('visibilitychange', runtime.visibility)
      if (runtime.lease && navigator.onLine !== false) void runtime.poll().catch(error => runtime.noteError(error))
      return runtime
    } catch (error) { store.close(); throw error }
  }
  getSnapshot = () => this.snapshot
  now = () => Date.now() + (this.lease?.offset ?? 0)
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener) } }
  private emit(patch: Partial<Snapshot> = {}) { if (this.stopped) return; this.snapshot = { ...this.snapshot, ...patch, ...('error' in patch && !patch.error ? { fault: undefined } : {}) }; if (!this.account) setGuestWorkPending(this.snapshot.jobs.filter(activeJob).length + (this.snapshot.busy ? 1 : 0)); for (const listener of this.listeners) listener() }
  private trace<T>(stage: ConnectionStage, task: () => Promise<T>, method?: string, path?: string) { return connectionStage({ channel: this.account ? 'account' : 'visitor', stage, method, path }, task) }
  private noteError(error: unknown) {
    if (this.stopped) return
    this.failures++; this.lastFault = connectionFault(error, { channel: this.account ? 'account' : 'visitor', stage: 'local-data' })
    this.emit({ offline: navigator.onLine === false, error: error instanceof Error ? error.message : 'GUEST_UNAVAILABLE', fault: this.lastFault }); this.schedulePoll()
  }
  async diagnostics() {
    const keys = await checkStoredKeys(this.profile)
    const local = await Promise.all([this.store.jobs.count(), this.store.guestPeers.count()]).then(([jobs, peers]) => ({ jobs, peers, schema: this.store.verno })).catch(() => ({ unavailable: true }))
    return { channel: this.account ? 'account' : 'visitor', publicFingerprint: /^[a-f0-9]{64}$/.test(this.profile.root) ? this.profile.root.slice(0, 12) : 'invalid', connected: this.snapshot.connected, activeFailure: this.snapshot.fault, lastFailure: this.lastFault, keys, local, lease: { present: !!this.lease, expired: this.lease ? Date.parse(this.lease.expires_at) <= this.now() : undefined, deviceMatches: this.lease ? this.lease.device_id === this.profile.device : undefined, finiteClockOffset: this.lease ? Number.isFinite(this.lease.offset) : undefined } }
  }
  private schedulePoll() {
    clearTimeout(this.timer)
    if (this.stopped || navigator.onLine === false || this.snapshot.error === 'GUEST_IDENTITY_CHANGED') return
    const delay = this.snapshot.error ? Math.min(30000, 1000 * 2 ** Math.min(5, Math.max(0, this.failures - 1))) : document.hidden ? 4000 : 1000
    this.timer = setTimeout(() => { if (this.snapshot.error) void this.ensureSession().then(() => this.poll()).catch(error => this.noteError(error)); else void this.poll() }, delay)
  }
  private wake = () => { this.emit({ offline: navigator.onLine === false }); if (navigator.onLine !== false && this.lease) void this.ensureSession().then(() => this.poll()).catch(error => this.noteError(error)) }
  private visibility = () => { if (!document.hidden) this.wake() }
  private async refreshLocal() {
    const jobs = (await this.store.jobs.toArray()).sort((a, b) => b.created - a.created).map(job=>job.authority==='LOCAL_DEVICE'&&activeJob(job)&&!this.local.reachable(job.peer.root)?{...job,peer:{...job.peer,online:false},phase:job.phase==='waiting'?'waiting' as const:'paused' as const,local:{...job.local!,disconnected:true}}:job)
    const peers = [...(await this.store.guestPeers.toArray()).filter(peer => !peer.replacedBy && (peer.reconnectRequired || Date.parse(peer.expires_at) > this.now())), ...(await this.store.localPeers.toArray()).map(peer=>({...peer,online:this.local.reachable(peer.root)}))]
    this.lifetime.signal.throwIfAborted()
    if (!this.account) { setGuestHistoryAvailable(jobs.length > 0); setGuestWorkPending(jobs.filter(activeJob).length) }
    this.emit({ jobs, connections: { ...this.snapshot.connections, peers } })
  }
  private async changed() { await this.refreshLocal(); this.channel?.postMessage('changed') }
  private async exclusive<T>(name: string, task: () => Promise<T>, signal = this.lifetime.signal): Promise<T> {
    // IndexedDB commits also compare current values. Locks coordinate network/cookie order across tabs.
    signal.throwIfAborted()
    return navigator.locks ? navigator.locks.request(this.store.name + ':' + name, { mode: 'exclusive', signal }, task) : task()
  }
  private command<T>(key: string, task: () => Promise<T>) {
    if (updateFrozen()) return Promise.reject(new GuestError('UPDATE_IN_PROGRESS'))
    const existing = this.commands.get(key); if (existing) return existing as Promise<T>
    const work = task().finally(() => { if (this.commands.get(key) === work) this.commands.delete(key) }); this.commands.set(key, work); return work
  }
  private async publicRequest<T>(path: string, input: unknown, headers: Record<string, string> = {}) {
    return this.trace(requestStage(path), async () => {
    const response = await fetch(this.server + path, { method: 'POST', headers: { 'Content-Type': 'application/json', ...headers }, body: JSON.stringify(input), credentials: 'include', cache: 'no-store', redirect: 'error', signal: AbortSignal.any([this.lifetime.signal, AbortSignal.timeout(15000)]) })
    const value = await response.json().catch(() => ({})) as T & { code?: string }
    this.lifetime.signal.throwIfAborted()
    if (!response.ok) throw new GuestError(value.code ?? 'GUEST_UNAVAILABLE', response.status)
    return value
    }, 'POST', path)
  }
  async ensureSession() {
    if (this.account) {
      const account = this.account
      return this.trace('session', async () => {
      if (this.stopped) throw new Error('GUEST_CLOSED')
      const session = authState().session
      if (!session || session.account.id !== account.owner || session.device_id !== this.profile.device || session.csrf !== account.csrf) throw new GuestError('ACCOUNT_SIGN_IN_REQUIRED', 401)
      this.lease = { id: 'self', owner_id: session.account.id, device_id: session.device_id, device_name: deviceName(), expires_at: session.expires_at, csrf: session.csrf, offset: account.offset }
      this.emit({ connected: true, error: undefined, offline: navigator.onLine === false })
      })
    }
    if (this.enrollment) return this.enrollment
    this.enrollment = this.trace('session', () => this.exclusive('enroll', async () => {
      if (this.stopped) throw new Error('GUEST_CLOSED')
      const challenge = await this.publicRequest<{ challenge: string; unix_time: number }>('/v1/guest/challenge', { signing_key: this.profile.signingKey, client_kind: 'web', surface: 'web' })
      const input = { challenge: challenge.challenge, device_id: this.profile.device, encryption_key: this.profile.encryptionKey, device_name: deviceName() }, body = JSON.stringify(input)
      const signature = base64(new Uint8Array(await this.trace('sign', async () => crypto.subtle.sign('Ed25519', this.profile.signingPrivate, utf8.encode('pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n' + await digest(utf8.encode(body)))), 'POST', '/v1/guest/enroll')))
      const result = await this.publicRequest<{ guest: Omit<GuestLease, 'id' | 'csrf' | 'offset'>; csrf: string }>('/v1/guest/enroll', input, { 'X-Pinelink-Signature': signature })
      const lease = { ...result.guest, id: 'self', csrf: result.csrf, offset: challenge.unix_time * 1000 - Date.now() }
      const detached = await saveGuestLease(this.store, this.profile, lease, this.lifetime.signal)
      for (const id of detached) this.workers.get(id)?.abort()
      this.lease = lease
      this.emit({ connected: true, error: undefined, offline: false })
    })).finally(() => { this.enrollment = undefined })
    return this.enrollment
  }
  async reconnect() {
    try { await this.ensureSession(); await this.poll() }
    catch (error) { this.noteError(error); throw error }
    // poll publishes its failure instead of rejecting; keep retry callers truthful.
    if (this.snapshot.error) throw new GuestError(this.snapshot.error)
  }
  async request<T>(method: string, path: string, input?: unknown, signal?: AbortSignal): Promise<T> {
    return this.trace(requestStage(path), async () => {
    this.lifetime.signal.throwIfAborted()
    const lease = this.lease ?? await this.store.leases.get('self')
    if (!lease) throw new GuestError('GUEST_EXPIRED', 401)
    const body = input === undefined ? '' : JSON.stringify(input), timestamp = Math.floor((Date.now() + lease.offset) / 1000).toString()
    const signature = base64(new Uint8Array(await this.trace('sign', async () => crypto.subtle.sign('Ed25519', this.profile.signingPrivate, utf8.encode(`${method}\n${path}\n${timestamp}\n${await digest(utf8.encode(body))}`)), method, path)))
    const response = await fetch(this.server + path, { method, body: method === 'GET' ? undefined : body, credentials: 'include', cache: 'no-store', redirect: 'error', signal: AbortSignal.any([this.lifetime.signal, AbortSignal.timeout(15000), ...(signal ? [signal] : [])]), headers: { 'Content-Type': 'application/json', 'X-Pinelink-Owner': lease.owner_id, 'X-Pinelink-Device': this.profile.device, 'X-Pinelink-Timestamp': timestamp, 'X-Pinelink-Signature': signature, 'X-Pinelink-CSRF': lease.csrf } })
    const value = await response.json().catch(() => ({})) as T & { code?: string }
    this.lifetime.signal.throwIfAborted(); signal?.throwIfAborted()
    if (!response.ok) throw new GuestError(response.status === 410 && /^\/v1\/(?:transfers|p2p|tasks)\//.test(path) ? 'FILE_EXPIRED' : value.code ?? 'GUEST_UNAVAILABLE', response.status)
    return value
    }, method, path)
  }
  async connect() {
    return this.command('connect', async () => {
      this.emit({ busy: true, error: undefined })
      try {
        await this.ensureSession()
        const pending = this.snapshot.invite
        if (pending && !pending.consumed && Date.parse(pending.expires_at) > this.now()) return pending
        const invite = await this.exclusive('invite', async () => {
          const existing = await this.request<GuestConnections>('GET', '/v1/guest/connections')
          const live = existing.invites.find(i => !i.consumed && Date.parse(i.expires_at) > this.now())
          return this.request<GuestInvite>('POST', '/v1/guest/invites', { request_id: live?.id ?? crypto.randomUUID() })
        })
        this.emit({ invite }); void this.poll(); return invite
      } finally { this.emit({ busy: false }) }
    })
  }
  async join(value: string) {
    return this.command('join', async () => {
      this.emit({ busy: true, error: undefined })
      try { await this.ensureSession(); const isToken = /^[A-Za-z0-9_-]{22}$/.test(value); const result = await this.request<GuestCandidate>('POST', '/v1/guest/join', isToken ? { token: value } : { code: value }); await this.poll(); return result }
      finally { this.emit({ busy: false }) }
    })
  }
  async respondCandidate(candidate: string, approve: boolean) {
    return this.command('candidate:' + candidate, async () => { await this.request('POST', `/v1/guest/candidates/${encodeURIComponent(candidate)}/${approve ? 'approve' : 'decline'}`, {}); await this.poll() })
  }
  async renamePeer(pair: string, name: string) {
    if(pair.startsWith('local:'))return this.local.rename(pair,name)
    const value = name.trim()
    if (!value || Array.from(value).length > 64 || /[\u0000-\u001f\u007f]/.test(value)) throw new GuestError('DEVICE_NAME_INVALID')
    return this.exclusive('peer:' + pair, async () => {
      await this.store.transaction('rw', this.store.guestPeers, async () => {
        if (!await this.store.guestPeers.get(pair)) throw new GuestError('CONNECTION_GONE')
        await this.store.guestPeers.update(pair, { local_name: value })
      }); await this.changed()
    })
  }
  async removePeer(pair: string) {
    if(pair.startsWith('local:'))return this.local.remove(pair)
    return this.command('remove:' + pair, () => this.exclusive('peer:' + pair, async () => {
      if (navigator.onLine === false) throw new GuestError('CONNECTION_OFFLINE')
      await this.ensureSession()
      if ((await this.store.guestPeers.get(pair))?.reconnectRequired) throw new GuestError('CONNECTION_REPAIR_REQUIRED')
      try { await this.request('DELETE', '/v1/guest/connections/' + encodeURIComponent(pair)) }
      catch (error) { if (error instanceof GuestError && error.status === 409) { await this.poll(); throw new GuestError('CONNECTION_BUSY', 409) }; throw error }
      await this.store.transaction('rw', this.store.guestPeers, this.store.peerRemovals, async () => {
        const peer = await this.store.guestPeers.get(pair)
        await this.store.peerRemovals.put({ pair, expires: Math.max(this.now() + 86400000, Date.parse(peer?.expires_at ?? '') || 0) + 86400000 })
        await this.store.guestPeers.delete(pair)
      }); await this.changed(); await this.poll()
    }))
  }
  async send(file: File, peer: GuestPeer, maxBytes: number, text?: { sensitivity: 'NORMAL' | 'SENSITIVE' }) {
    if(peer.authority==='LOCAL_DEVICE')return this.command('send',async()=>{this.emit({busy:true,error:undefined});try{return await this.local.send(file,peer,64<<20,text)}finally{this.emit({busy:false})}})
    return this.command('send', async () => {
      if (file.size > maxBytes) throw new GuestError('FILE_TOO_LARGE')
      if (this.snapshot.jobs.filter(activeJob).length >= 2) throw new GuestError('GUEST_LIMIT_REACHED')
      this.emit({ busy: true, error: undefined })
      try {
        await this.ensureSession()
        if (peer.reconnectRequired || (await this.store.guestPeers.get(peer.pair_id))?.reconnectRequired) throw new GuestError('CONNECTION_REPAIR_REQUIRED')
        const prepared = await prepareFile(file, { sender: this.profile.encryptionKey, receiver: peer.encryption_key }, text)
        this.lifetime.signal.throwIfAborted()
        const job: GuestJob = { id: crypto.randomUUID(), created: Date.now(), direction: 'send', peer, prepared, ...(text ? { payloadKind: 'TEXT', text: { cipher: prepared.cipher, manifest: prepared.senderManifest } } : {}), received: 0, name: prepared.name, plainSize: file.size, phase: 'preparing' }
        // Persist the immutable encrypted attempt before creating a network-side offer.
        await this.store.jobs.add(job); await this.changed(); this.startJob(job); return job.id
      } finally { this.emit({ busy: false }) }
    })
  }
  async accept(id: string) {
    if(id.startsWith('local:'))return this.local.action(id,'accept')
    return this.command('accept:' + id, async () => {
      const job = await this.store.jobs.get(id); if (!job?.transfer) return
      await this.requireJobConnection(job)
      const transfer = await this.respondTransfer(job, 'accept')
      await this.store.jobs.update(id, { transfer, phase: 'running', error: undefined }); await this.changed(); this.startJob({ ...job, transfer, phase: 'running' })
    })
  }
  async reject(id: string) {
    if(id.startsWith('local:'))return this.local.action(id,'reject')
    return this.command('reject:' + id, async () => {
      const job = await this.store.jobs.get(id); if (!job?.transfer) return
      await this.requireJobConnection(job)
      const transfer = await this.respondTransfer(job, 'reject')
      this.workers.get(id)?.abort(); await this.store.jobs.update(id, { transfer, phase: 'stopped' }); await this.changed()
    })
  }
  private action(transfer: GuestTransfer) { return { owner_id: this.lease!.owner_id, device_id: this.profile.device, expected_version: transfer.version } }
  private async requireJobConnection(job: GuestJob) {
    if (job.reconnectRequired || job.peer.reconnectRequired || (await this.store.guestPeers.get(job.peer.pair_id))?.reconnectRequired) throw new GuestError('CONNECTION_REPAIR_REQUIRED')
  }
  private async respondTransfer(job: GuestJob, action: 'accept' | 'reject' | 'cancel') {
    const original = job.transfer!
    for (let attempt = 0; attempt < 2; attempt++) {
      const current = await this.request<GuestTransfer>('GET', `/v1/transfers/${original.id}`)
      // Attaching a sealed cloud entry advances the aggregate version while
      // preserving the exact content the user confirmed. Never retry a new file.
      if (current.content_root !== original.content_root || current.size_bytes !== original.size_bytes || current.file_name !== original.file_name || current.encryption_suite !== original.encryption_suite || current.content_kind !== original.content_kind || current.sender_owner_id !== original.sender_owner_id || current.sender_device_id !== original.sender_device_id || current.recipient_owner_id !== original.recipient_owner_id || current.recipient_device_id !== original.recipient_device_id) throw new GuestError('FILE_JOB_CHANGED')
      try { return await this.request<GuestTransfer>('POST', `/v1/transfers/${current.id}/${action}`, { ...this.action(current), ...(action === 'accept' && current.content_kind === 'TEXT' ? { accept_content_kind: 'TEXT' } : {}) }) }
      catch (error) { if (attempt !== 0 || !(error instanceof GuestError) || error.code !== 'STATE_CONFLICT') throw error }
    }
    throw new GuestError('STATE_CONFLICT')
  }
  private startJob(job: GuestJob) {
    if (job.reconnectRequired || job.peer.reconnectRequired || job.authority==='LOCAL_DEVICE' || this.stopped || this.workers.has(job.id) || !activeJob(job) || job.phase === 'paused' || (job.retryAt ?? 0) > Date.now() || (job.direction === 'send' && job.snapshot?.completion.stored) || (job.direction === 'receive' && job.transfer?.state === 'PENDING' && (job.directAttempted || hasSpool(job)))) return
    const controller = new AbortController(); this.workers.set(job.id, controller)
    const work = () => this.exclusive('job:' + job.id, async () => {
      const current = await this.store.jobs.get(job.id)
      if (!current || !activeJob(current) || current.phase === 'paused' || controller.signal.aborted) return
      await (current.direction === 'send' ? this.sendJob(current, controller.signal) : this.receiveJob(current, controller.signal))
    }, controller.signal)
    const operation = work().catch(async error => {
      if (!controller.signal.aborted && !this.stopped) {
        await this.store.transaction('rw', this.store.jobs, async () => {
          const current = await this.store.jobs.get(job.id); if (controller.signal.aborted || this.stopped || !current || !activeJob(current)) return
          const failures = (current.failures ?? 0) + 1
          await this.store.jobs.update(job.id, { error: error instanceof Error ? error.message : 'TRANSFER_INTERRUPTED', failures, retryAt: Date.now() + Math.min(30000, failures * 3000), ...((error instanceof GuestError && [403, 404, 410].includes(error.status)) ? { phase: 'stopped' as const } : {}) })
        }); await this.changed()
      }
    }).catch(error => this.noteError(error)).finally(() => { if (this.workers.get(job.id) === controller) this.workers.delete(job.id); this.workerTasks.delete(operation) })
    this.workerTasks.add(operation)
  }
  private async sendJob(job: GuestJob, signal: AbortSignal) {
    if (!job.prepared) throw new Error('FILE_SOURCE_MISSING')
    let transfer = job.transfer
    if (!transfer) {
      const p = job.prepared
      transfer = await this.request<GuestTransfer>('POST', job.payloadKind === 'TEXT' ? '/v1/p2p/text' : '/v1/p2p', { sender_owner_id: this.lease!.owner_id, sender_device_id: this.profile.device, recipient_owner_id: job.peer.owner_id, recipient_device_id: job.peer.device_id, guest_pair_id: job.peer.pair_id, client_request_id: job.id, file_name: p.name, size_bytes: p.cipher.size, content_root: p.root, encryption_suite: suite, encrypted_manifest: p.recipientManifest, sender_encrypted_manifest: p.senderManifest }, signal)
      if (transfer.content_kind !== job.payloadKind) throw new Error('FILE_JOB_CHANGED')
      await this.store.jobs.update(job.id, { transfer, phase: 'waiting', error: undefined }); await this.changed()
    }
    const progress = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${transfer.id}/progress`, undefined, signal)
    if (progress.completion.stored || progress.completion.delivered) return
    job = (await this.store.jobs.get(job.id))!
    if (!job.directAttempted && !hasSpool({ ...job, snapshot: progress })) {
      await this.store.jobs.update(job.id, { directAttempted: true }); await this.transport(job.id, 'connecting', signal)
      let link: DirectLink | undefined
      try {
        link = this.directLink(job, signal)
        const proof = await link.connect(); await this.transport(job.id, 'direct', signal, proof)
        await this.waitAccepted(job, link.signal)
        const from = await link.waitReady()
        let observedAt = Date.now()
        for (let index = Math.floor(from / wireChunkSize); index < Math.max(1, Math.ceil(job.prepared!.cipher.size / wireChunkSize)); index++) {
          signal.throwIfAborted()
          const bytes = new Uint8Array(await job.prepared!.cipher.slice(index * wireChunkSize, (index + 1) * wireChunkSize).arrayBuffer())
          await link.send(await encodeFileFrame(index, bytes), Math.min(job.prepared!.cipher.size, (index + 1) * wireChunkSize))
          if (Date.now() - observedAt >= 500) { const snap = await this.observe(job, signal); if (snap.delivery.state === 'PAUSED') throw new DirectUnavailable(); await this.transport(job.id, 'direct', signal, await link.proof()); observedAt = Date.now() }
        }
        await this.waitDelivered(job, signal, link); return
      } catch (error) {
        signal.throwIfAborted()
        // Check the authoritative receipt before interpreting a final channel
        // close as a failure. Never upload a second copy after direct delivery.
        const latest = await this.observe(job, signal)
        if (latest.completion.delivered) return
        if (latest.delivery.acknowledged_bytes === transfer.size_bytes && transfer.size_bytes > 0 && await this.receiptGrace(job, signal)) return
        if (!(error instanceof DirectUnavailable) && !(error instanceof TypeError) && !(error instanceof DOMException && error.name === 'NotSupportedError')) throw error
      } finally { await link?.close() }
    }
    await this.fallback(job, signal)
    await this.sendCloud(job, signal)
  }
  private directLink(job: GuestJob, signal: AbortSignal) {
    const transfer = job.transfer!
    return new DirectLink(job.direction === 'send' ? 'SENDER' : 'RECEIVER', transfer.id, transfer.content_root, transfer.size_bytes,
      (method, suffix, input, callSignal) => this.request(method, `/v1/p2p/${transfer.id}${suffix}`, input === undefined ? undefined : { ...input as object, owner_id: this.lease!.owner_id, device_id: this.profile.device }, callSignal ?? signal), signal)
  }
  private async transport(id: string, state: NonNullable<GuestJob['transport']>['state'], signal: AbortSignal, proof?: DirectProof) {
    signal.throwIfAborted()
    await this.store.transaction('rw', this.store.jobs, async () => {
      const current = await this.store.jobs.get(id); signal.throwIfAborted()
      if (current && (activeJob(current) || current.transport?.state === state)) await this.store.jobs.update(id, { transport: { state, proof: proof ?? current.transport?.proof } })
    })
    await this.changed()
  }
  private async observe(job: GuestJob, signal: AbortSignal) {
    const snap = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${job.transfer!.id}/progress`, undefined, signal)
    await this.applyProgress(job.id, snap)
    signal.throwIfAborted()
    if (['CANCELLED', 'REJECTED', 'EXPIRED', 'FAILED', 'DELETED'].includes(snap.state)) throw new GuestError('FILE_EXPIRED', 410)
    return snap
  }
  private async waitAccepted(job: GuestJob, signal: AbortSignal) {
    while (true) { const snap = await this.observe(job, signal); if (snap.completion.delivered) return; if (snap.state === 'ACCEPTED') { if (snap.delivery.state === 'PAUSED') throw new DirectUnavailable(); return }; await wait(500, signal) }
  }
  private async waitDelivered(job: GuestJob, signal: AbortSignal, link?: DirectLink) {
    let disconnectedAt = 0
    while (true) {
      const snap = await this.observe(job, signal)
      if (link) await this.transport(job.id, 'direct', signal, await link.proof())
      if (snap.completion.delivered) return
      if (snap.delivery.state === 'PAUSED' && (snap.delivery.acknowledged_bytes < job.transfer!.size_bytes || !job.transfer!.size_bytes)) throw new DirectUnavailable()
      // A lost channel cannot hide behind a still-connected ICE transport.
      if (link?.signal.aborted) {
        disconnectedAt ||= Date.now()
        if (snap.delivery.acknowledged_bytes < job.transfer!.size_bytes || Date.now() - disconnectedAt > 1500) throw new DirectUnavailable()
      }
      await wait(500, signal)
    }
  }
  private async receiptGrace(job: GuestJob, signal: AbortSignal) {
    const until = Date.now() + 1500
    do { if ((await this.observe(job, signal)).completion.delivered) return true; await wait(300, signal) } while (Date.now() < until)
    return false
  }
  private async fallback(job: GuestJob, signal: AbortSignal) {
    await this.transport(job.id, 'switching', signal)
    const result = await this.request<{ signals: { channel: string }[] | null }>('GET', `/v1/p2p/${job.transfer!.id}/signals?after=0`, undefined, signal)
    if (!result.signals?.some(fallbackSignal)) await this.request('POST', `/v1/p2p/${job.transfer!.id}/signals`, { owner_id: this.lease!.owner_id, device_id: this.profile.device, role: job.direction === 'send' ? 'SENDER' : 'RECEIVER', channel: job.direction === 'send' ? 'web-direct-failed-sender' : 'web-direct-failed', payload: 'unavailable' }, signal)
  }
  private async sendCloud(job: GuestJob, signal: AbortSignal) {
    const transfer = job.transfer!
    const progress = await this.observe(job, signal)
    if (progress.completion.stored || progress.completion.delivered) return
    if (progress.delivery.acknowledged_bytes === transfer.size_bytes && transfer.size_bytes > 0 && await this.receiptGrace(job, signal)) return
    const socket = await this.openSocket(transfer.id, 'sender', signal)
    try {
      await this.transport(job.id, 'cloud', signal)
      for (let index = 0; index < Math.max(1, Math.ceil(job.prepared!.cipher.size / wireChunkSize)); index++) {
        signal.throwIfAborted()
        while (socket.bufferedAmount > 256 << 10) { await wait(25, signal); if (socket.readyState !== WebSocket.OPEN) throw new Error('TRANSFER_INTERRUPTED') }
        if (socket.readyState !== WebSocket.OPEN) throw new Error('TRANSFER_INTERRUPTED')
        const bytes = new Uint8Array(await job.prepared!.cipher.slice(index * wireChunkSize, (index + 1) * wireChunkSize).arrayBuffer())
        socket.send(await encodeFileFrame(index, bytes))
      }
      while (true) { await wait(600, signal); const snap = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${transfer.id}/progress`, undefined, signal); await this.applyProgress(job.id, snap); if (snap.completion.stored || snap.completion.delivered) break; if (socket.readyState !== WebSocket.OPEN) throw new Error('TRANSFER_INTERRUPTED') }
    } finally { socket.close(); }
  }
  private async openSocket(id: string, role: string, signal: AbortSignal, from?: number) {
    const grant = await this.request<{ ticket: string }>('POST', `/v1/p2p/${id}/relay-ticket`, { role, ...(from !== undefined ? { resume_from: from } : {}) }, signal)
    const url = new URL(this.server + `/v1/p2p/${id}/relay?role=${role}`); url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(url, ['pinelink-relay-v1', 'pinelink-ticket.' + grant.ticket]); socket.binaryType = 'arraybuffer'
    const abort = () => { socket.close() }; signal.addEventListener('abort', abort, { once: true }); socket.addEventListener('close', () => signal.removeEventListener('abort', abort), { once: true })
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => { socket.close(); reject(new Error('TRANSFER_INTERRUPTED')) }, 15000)
      socket.addEventListener('open', () => { clearTimeout(timer); resolve() }, { once: true }); socket.addEventListener('error', () => { clearTimeout(timer); reject(new Error('TRANSFER_INTERRUPTED')) }, { once: true }); socket.addEventListener('close', () => { clearTimeout(timer); reject(new Error('TRANSFER_INTERRUPTED')) }, { once: true })
    })
    signal.throwIfAborted(); return socket
  }
  private async receiveJob(job: GuestJob, signal: AbortSignal) {
    const transfer = job.transfer!
    const snapshot = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${transfer.id}/progress`, undefined, signal)
    if (transfer.state !== 'PENDING' && job.received >= transfer.size_bytes && (transfer.size_bytes > 0 || await this.store.chunks.get([job.id, 0]))) { await this.verifyAndFinish(job.id, await this.incomingManifest(job, signal), signal); return }
    if (!job.directAttempted && !hasSpool({ ...job, snapshot })) {
      await this.store.jobs.update(job.id, { directAttempted: true }); await this.transport(job.id, 'connecting', signal)
      let link: DirectLink | undefined
      try {
        link = this.directLink(job, signal)
        await this.transport(job.id, 'direct', signal, await link.connect())
        await this.waitAccepted(job, link.signal)
        const manifest = await this.incomingManifest(job, link.signal)
        const live = await this.observe(job, signal), route = { kind: 'DIRECT', epoch: (live.route?.epoch ?? 0) + 1 }
        await this.reportReceived(job, job.received, route, signal)
        const from = Math.floor(Math.min(job.received, live.delivery.acknowledged_bytes) / wireChunkSize) * wireChunkSize
        const direct = link, receive = this.frameReceiver(job, manifest, route, from, 1 << 20, received => direct.acknowledge(received))
        await link.receive(from, receive)
        await this.transport(job.id, 'direct', signal, await link.proof()); return
      } catch (error) {
        signal.throwIfAborted()
        if (!(error instanceof DirectUnavailable) && !(error instanceof TypeError) && !(error instanceof DOMException && error.name === 'NotSupportedError')) throw error
      } finally { await link?.close() }
    }
    job = (await this.store.jobs.get(job.id))!
    if (job.transfer?.state !== 'PENDING' && job.received >= transfer.size_bytes && (transfer.size_bytes > 0 || await this.store.chunks.get([job.id, 0]))) { await this.verifyAndFinish(job.id, await this.incomingManifest(job, signal), signal); return }
    await this.fallback(job, signal)
    const current = await this.observe(job, signal)
    if (current.state === 'PENDING') return
    if (current.delivery.state === 'PAUSED') return
    await this.receiveCloud(job, await this.incomingManifest(job, signal), current, signal)
  }
  private async incomingManifest(job: GuestJob, signal: AbortSignal) {
    // The server intentionally withholds the recipient key envelope until file
    // consent. Transport negotiation needs neither that envelope nor file data.
    const current = await this.request<GuestTransfer>('GET', `/v1/transfers/${job.transfer!.id}`, undefined, signal)
    if (current.content_root !== job.transfer!.content_root || current.size_bytes !== job.transfer!.size_bytes || current.content_kind !== job.payloadKind) throw new Error('FILE_JOB_CHANGED')
    if (!current.encrypted_manifest || !['ACCEPTED', 'COMPLETED'].includes(current.state)) throw new Error('FILE_MANIFEST_MISSING')
    const manifest = await openManifest(this.profile, current.encrypted_manifest); signal.throwIfAborted()
    if ((manifest.content_kind === 'TEXT') !== (job.payloadKind === 'TEXT')) throw new Error('FILE_MANIFEST_INVALID')
    await this.store.transaction('rw', this.store.jobs, async () => {
      const newest = await this.store.jobs.get(job.id); signal.throwIfAborted()
      if (!newest || !activeJob(newest) || newest.phase === 'paused') throw new DOMException('Transfer stopped', 'AbortError')
      await this.store.jobs.update(job.id, { transfer: current.version >= (newest.transfer?.version ?? 0) ? current : newest.transfer, name: safeFileName(manifest.file_name), plainSize: manifest.plaintext_size, error: undefined })
    })
    await this.changed(); return manifest
  }
  private async reportReceived(job: GuestJob, received: number, route: { kind: string; epoch: number }, signal: AbortSignal) {
    await this.request('POST', `/v1/p2p/${job.transfer!.id}/progress`, { owner_id: this.lease!.owner_id, device_id: this.profile.device, received_bytes: received, route: route.kind, route_epoch: route.epoch }, signal)
  }
  private frameReceiver(job: GuestJob, manifest: Awaited<ReturnType<typeof openManifest>>, route: { kind: string; epoch: number }, from: number, threshold: number, acknowledged?: (received: number) => void) {
    let lastAck = from, lastReport = Date.now()
    return async (buffer: ArrayBuffer, signal: AbortSignal) => {
      signal.throwIfAborted()
      const frame = await decodeFileFrame(buffer, job.transfer!.size_bytes); signal.throwIfAborted()
      const received = await this.store.storeFrame(job.id, frame.index, frame.bytes, signal)
      if (received - lastAck >= threshold || Date.now() - lastReport >= 500 || received === job.transfer!.size_bytes) {
        await this.reportReceived(job, received, route, signal); signal.throwIfAborted()
        lastAck = received; lastReport = Date.now(); await this.changed()
      }
      // Peer flow control acknowledges durable local chunks; visible progress
      // still uses the separately committed server ledger. This avoids a cloud
      // round trip for every small SCTP window on a fast LAN.
      signal.throwIfAborted(); acknowledged?.(received)
      if (received === job.transfer!.size_bytes) { await this.verifyAndFinish(job.id, manifest, signal, route); return true }
      return false
    }
  }
  private async receiveCloud(job: GuestJob, manifest: Awaited<ReturnType<typeof openManifest>>, snapshot: ProgressSnapshot, signal: AbortSignal) {
    const transfer = job.transfer!
    const from = Math.min(Math.floor(job.received / wireChunkSize) * wireChunkSize, Math.floor(snapshot.delivery.acknowledged_bytes / wireChunkSize) * wireChunkSize)
    const route = { kind: 'RELAY', epoch: Math.max(1, (snapshot.route?.epoch ?? 0) + (snapshot.route?.kind === 'RELAY' ? 0 : 1)) }
    const socket = await this.openSocket(transfer.id, 'receiver', signal, from)
    let drain = Promise.resolve()
    try {
      // A sealed cloud copy can send immediately on WebSocket open. Install
      // handlers synchronously, then gate processing on route initialization;
      // awaiting HTTP/IndexedDB first would silently lose those first frames.
      const setup = this.transport(job.id, 'cloud', signal).then(() => this.reportReceived(job, from, route, signal))
      const receive = this.frameReceiver(job, manifest, route, from, 1 << 20)
      await new Promise<void>((resolve, reject) => {
        let chain = Promise.resolve(), finished = false
        const fail = (error: unknown) => { if (!finished) { finished = true; reject(error) } }
        chain = setup.catch(fail); drain = chain
        socket.onmessage = event => {
          chain = chain.then(async () => {
            if (finished || signal.aborted) return
            if (!(event.data instanceof ArrayBuffer)) throw new Error('FILE_FRAME_INVALID')
            if (await receive(event.data, signal)) { finished = true; resolve() }
          }).catch(fail)
          drain = chain
        }
        socket.onclose = () => { void chain.then(() => { if (!finished) fail(new Error('TRANSFER_INTERRUPTED')) }) }
        socket.onerror = () => fail(new Error('TRANSFER_INTERRUPTED'))
        signal.addEventListener('abort', () => fail(signal.reason), { once: true })
      })
    } finally { socket.close(); await drain.catch(() => undefined) }
  }
  private async verifyAndFinish(id: string, manifest: Awaited<ReturnType<typeof openManifest>>, signal: AbortSignal, selected?: { kind: string; epoch: number }) {
    const job = (await this.store.jobs.get(id))!; if (!job.transfer) throw new Error('FILE_JOB_CHANGED')
    await this.store.jobs.update(id, { phase: 'verifying' }); await this.changed()
    const cipher = await this.store.receivedCipher(job)
    if (await hashBlob(cipher) !== job.transfer.content_root) throw new Error('FILE_CHECKSUM_MISMATCH')
    const plain = await decryptDare(cipher, manifest); signal.throwIfAborted()
    if (job.payloadKind === 'TEXT') {
      decodeText(new Uint8Array(await plain.arrayBuffer()))
      if (!job.transfer.encrypted_manifest) throw new Error('FILE_MANIFEST_MISSING')
      await this.store.jobs.update(id, { text: { cipher, manifest: job.transfer.encrypted_manifest }, name: safeFileName(manifest.file_name), plainSize: manifest.plaintext_size })
    } else await this.store.jobs.update(id, { plain, name: safeFileName(manifest.file_name), plainSize: manifest.plaintext_size })
    const snapshot = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${job.transfer.id}/progress`, undefined, signal)
    const route = selected ?? { kind: snapshot.route?.kind === 'DIRECT' ? 'DIRECT' : 'RELAY', epoch: Math.max(1, snapshot.route?.epoch ?? 0) }
    await this.reportReceived(job, job.transfer.size_bytes, route, signal)
    const current = await this.request<GuestTransfer>('GET', `/v1/transfers/${job.transfer.id}`, undefined, signal)
    const result = await this.request<GuestTransfer & { progress?: ProgressSnapshot }>('POST', `/v1/p2p/${current.id}/complete`, this.action(current), signal)
    const { progress: projection, ...completed } = result
    const fresh = projection ?? await this.request<ProgressSnapshot>('GET', `/v1/p2p/${current.id}/progress`, undefined, signal).catch(() => undefined)
    await this.store.transaction('rw', this.store.jobs, this.store.chunks, async () => {
      const newest = await this.store.jobs.get(id); if (!newest) return
      const snapshot = fresh?.transfer_id === completed.id && fresh.completion.delivered && (fresh.state_version >= (newest.snapshot?.state_version ?? 0)) && (fresh.route?.epoch ?? 0) >= (newest.snapshot?.route?.epoch ?? 0) ? fresh : newest.snapshot
      await this.store.jobs.update(id, { transfer: completed.version >= (newest.transfer?.version ?? 0) ? completed : newest.transfer, snapshot, phase: 'complete', error: undefined })
      if (newest.payloadKind === 'TEXT' && newest.text) await this.store.chunks.where('job').equals(id).delete()
    }); await this.changed()
  }
  private async applyProgress(id: string, snapshot: ProgressSnapshot) {
    await this.store.transaction('rw', this.store.jobs, async () => {
      const job = await this.store.jobs.get(id); if (!job) return
      if (snapshot.state_version < (job.transfer?.version ?? 0) || snapshot.control_revision < (job.controlRevision ?? 0)) return
      if (job.snapshot && (snapshot.state_version < job.snapshot.state_version || snapshot.progress_revision < job.snapshot.progress_revision || (snapshot.route?.epoch ?? 0) < (job.snapshot.route?.epoch ?? 0))) return
      const phase = progressPhase(snapshot)
      await this.store.jobs.update(id, { snapshot, phase, controlRevision: snapshot.control_revision, ...(snapshot.completion.delivered ? { error: undefined, ...(job.direction === 'send' ? { prepared: undefined } : {}) } : {}) })
    })
    await this.refreshLocal()
    const current = this.snapshot.jobs.find(job => job.id === id)
    if (current && (current.phase === 'stopped' || current.phase === 'complete' && current.direction === 'send' && current.transport?.state === 'cloud' || current.direction === 'receive' && (current.phase === 'paused' || ['connecting', 'direct'].includes(current.transport?.state ?? '') && hasSpool(current)))) this.workers.get(id)?.abort()
  }
  async control(id: string, action: 'pause' | 'resume' | 'cancel') {
    if(id.startsWith('local:'))return this.local.action(id,action)
    return this.command('control:' + action + ':' + id, async () => {
      const job = await this.store.jobs.get(id); if (!job?.transfer) return
      await this.requireJobConnection(job)
      let controlRevision = job.controlRevision ?? 0
      let updatedTransfer = job.transfer
      let changedTask: ControlledTask | undefined
      if (action === 'cancel' && job.direction === 'send') updatedTransfer = await this.respondTransfer(job, 'cancel')
      else {
        const tasks = await this.request<{ tasks: { id: string; reference_id: string; kind: string; version: number }[] }>('GET', '/v1/tasks')
        const task = tasks.tasks?.find(t => t.kind === 'DOWNLOAD' && t.reference_id === job.transfer!.id); if (!task) throw new Error('TRANSFER_INTERRUPTED')
        changedTask = await this.request<ControlledTask>('POST', `/v1/tasks/${task.id}/${action}`, { owner_id: this.lease!.owner_id, device_id: this.profile.device, expected_version: task.version }); controlRevision = changedTask.version
      }
      this.workers.get(id)?.abort()
      const checkpoint = job.snapshot ?? (changedTask ? await this.request<ProgressSnapshot>('GET', `/v1/p2p/${job.transfer.id}/progress`) : undefined)
      await this.store.transaction('rw', this.store.jobs, async () => {
        const current = await this.store.jobs.get(id); if (!current || controlRevision < (current.controlRevision ?? 0)) return
        if (current.phase === 'complete' || current.transfer && ['COMPLETED', 'CANCELLED', 'REJECTED', 'EXPIRED', 'FAILED'].includes(current.transfer.state)) return
        const previous = current.snapshot ?? checkpoint
        if (previous && previous.control_revision > controlRevision) return
        // Task control and recipient ACKs serialize on the same server row. Publish
        // its committed byte boundary together with PAUSED, never a stale poll's
        // boundary that would appear to keep moving after pause is confirmed.
        const snapshot = previous && changedTask && !(previous.control_revision === changedTask.version && previous.progress_revision > changedTask.progress_revision) ? { ...previous, control_revision: changedTask.version, progress_revision: changedTask.progress_revision,
          delivery: { ...previous.delivery, state: changedTask.state === 'RUNNING' && changedTask.progress_bytes === changedTask.total_bytes ? 'VERIFYING' : changedTask.state, acknowledged_bytes: changedTask.progress_bytes, total_bytes: changedTask.total_bytes, revision: changedTask.progress_revision },
          capabilities: { ...previous.capabilities, pause: changedTask.state === 'RUNNING' && changedTask.progress_bytes < changedTask.total_bytes, resume: changedTask.state === 'PAUSED' },
        } : previous
        await this.store.jobs.update(id, { transfer: updatedTransfer.version >= (current.transfer?.version ?? 0) ? updatedTransfer : current.transfer, controlRevision, snapshot, phase: action === 'cancel' ? 'stopped' : snapshot ? progressPhase(snapshot) : current.phase, error: undefined })
      })
      await this.changed(); await this.poll()
    })
  }
  async retry(id: string) { if(id.startsWith('local:'))return this.local.action(id,'resume'); await this.ensureSession(); const job = await this.store.jobs.get(id); if (!job) return; await this.requireJobConnection(job); await this.store.jobs.update(id, { error: undefined, retryAt: 0, failures: 0 }); this.startJob({ ...job, retryAt: 0 }); await this.poll() }
  async dismiss(id: string) { await this.store.jobs.update(id, { dismissed: true }); await this.changed() }
  async download(id: string) {
    const job = await this.store.jobs.get(id); if (!job?.plain) throw new Error('FILE_SOURCE_MISSING')
    const url = URL.createObjectURL(job.plain), link = document.createElement('a'); link.href = url; link.download = safeFileName(job.name); document.body.append(link); link.click(); link.remove(); setTimeout(() => URL.revokeObjectURL(url), 60000)
  }
  async poll() {
    if (this.polling) return this.polling
    if (!this.lease || this.stopped || navigator.onLine === false) return
    clearTimeout(this.timer)
    this.polling = (async () => {
      const [connections, transfers] = await Promise.all([this.request<GuestConnections>('GET', '/v1/guest/connections'), this.request<{ transfers: GuestTransfer[] | null }>('GET', '/v1/transfers')])
      await this.exclusive('roster', () => this.store.transaction('rw', [this.store.guestPeers, this.store.peerRemovals, this.store.jobs], () => reconcileGuestPeers(this.store, connections.peers, this.now())))
      connections.peers = [...(await this.store.guestPeers.toArray()).filter(peer => !peer.replacedBy),...(await this.store.localPeers.toArray()).map(peer=>({...peer,online:this.local.reachable(peer.root)}))]
      this.emit({ connections, offline: false, error: undefined, checkedAt: this.now(), invite: this.snapshot.invite ? { ...this.snapshot.invite, consumed: connections.invites.find(i => i.id === this.snapshot.invite!.id)?.consumed ?? true } : undefined })
      for (const transfer of transfers.transfers ?? []) {
        try {
          const own = await this.store.jobs.where('transfer.id').equals(transfer.id).first()
          let job = own
          if (!job && transfer.recipient_owner_id === this.lease!.owner_id) {
            const peer = connections.peers.find(p => p.owner_id === transfer.sender_owner_id && p.device_id === transfer.sender_device_id)
            if (!peer) continue
            job = { id: transfer.id, created: Date.parse(transfer.created_at ?? '') || this.now(), direction: 'receive', peer, transfer, ...(transfer.content_kind === 'TEXT' ? { payloadKind: 'TEXT' } : {}), received: 0, name: safeFileName(transfer.file_name), plainSize: Math.max(0, transfer.size_bytes - Math.ceil(transfer.size_bytes / (wireChunkSize + 32)) * 32), phase: transfer.state === 'PENDING' ? 'waiting' : 'running' }
            const candidate = job
            await this.store.transaction('rw', this.store.jobs, async () => { if (!await this.store.jobs.get(candidate.id)) await this.store.jobs.add(candidate) })
          }
          if (!job) continue
          const id = job.id
          await this.store.transaction('rw', this.store.jobs, async () => { const current = await this.store.jobs.get(id); if (current && transfer.content_kind !== current.payloadKind) throw new Error('FILE_JOB_CHANGED'); if (current && transfer.version >= (current.transfer?.version ?? 0)) await this.store.jobs.update(id, { transfer }) })
          if (job.phase === 'complete' && job.snapshot?.completion.delivered && transfer.state === 'COMPLETED') continue
          const snap = await this.request<ProgressSnapshot>('GET', `/v1/p2p/${transfer.id}/progress`)
          await this.applyProgress(job.id, snap)
        } catch (error) {
          // A single history projection is not the availability of the service.
          if (this.stopped || error instanceof GuestError && error.status === 401) throw error
          const job = await this.store.jobs.where('transfer.id').equals(transfer.id).first()
          if (job) await this.store.jobs.update(job.id, { error: error instanceof Error ? error.message : 'TRANSFER_INTERRUPTED', retryAt: Date.now() + 30000 })
        }
      }
      this.failures = 0
      await this.refreshLocal()
      for (const job of this.snapshot.jobs.filter(activeJob)) this.startJob(job)
    })().catch(async error => { this.noteError(error); if (error instanceof GuestError && error.status === 401 && error.code === 'GUEST_EXPIRED') await this.ensureSession().catch(error => this.noteError(error)) }).finally(() => {
      this.polling = undefined
      this.schedulePoll()
    })
    return this.polling
  }
  close() { return this.closing ??= this.finishClose() }
  private async finishClose() {
    this.stopped = true; this.lifetime.abort(); clearTimeout(this.timer); for (const worker of this.workers.values()) worker.abort()
    window.removeEventListener('online', this.wake); window.removeEventListener('offline', this.wake); document.removeEventListener('visibilitychange', this.visibility); this.channel?.close()
    await Promise.allSettled([...this.workerTasks, ...this.commands.values(), ...(this.polling ? [this.polling] : []), ...(this.enrollment ? [this.enrollment] : []), ...(!this.account ? [this.local.close()] : [])]); this.store.close(); if (!this.account) setGuestWorkPending(0)
  }
}
let runtime: Promise<GuestRuntime> | undefined
export const getGuestRuntime = (localOnly=false) => runtime ??= GuestRuntime.open(undefined,localOnly).catch(error => { runtime = undefined; throw error })
export async function closeGuestRuntime() { const current = runtime; runtime = undefined; if (current) await current.then(value => value.close()).catch(() => undefined) }
