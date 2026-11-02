import { base64, canonical, digest, nonce, proof, utf8, verify } from './crypto'
import type { DeviceProfile } from './crypto'
import { LocalStore, storeName } from './store'
import { preparePage } from './wire'
import type { Page } from './wire'
import { requestBackgroundSync } from './background-sync'

export class DataError extends Error {
  constructor(public status: number, public code: string) { super(code) }
}
type SignedProfile = { root: string; epoch: string; roster_version: number; uploaded: number; signature: string }
type ServerInfo = { version: number; epoch: string; public_key: string; unix_time: number; nonce: string; signature: string }
const cacheable = (path: string) => /^\/v1\/(tasks|transfers|notifications|devices|quota|files|pulls)(\/|\?|$)/.test(path) && !/\/(content|manifest|envelope|chunks)(\/|\?|$)/.test(path)
const invalidatesCache = (error: unknown): error is DataError => error instanceof DataError && [401, 403, 404, 410].includes(error.status)

export class ClientRuntime {
  private active?: Promise<void>
  private attempt?: { skipped: boolean }
  private timer?: ReturnType<typeof setTimeout>
  private started = false
  private catchup = false
  private generation = 0
  private closed = false
  private closing?: Promise<void>
  private lifetime = new AbortController()
  private operations = new Set<Promise<unknown>>()
  private controllers = new Set<AbortController>()
  private wake = () => { void this.sync(true).catch(() => undefined) }
  private constructor(readonly server: string, readonly owner: string, readonly pin: string, readonly store: LocalStore, readonly profile: DeviceProfile) {}
  static async open(server: string, owner: string, device = '', pin = '', store?: LocalStore) {
    const absolute = new URL(server, location.href).href.replace(/\/$/, '')
    const database = store ?? new LocalStore(await storeName(absolute, owner, device))
    try {
      const profile = await database.profile(device)
      return new ClientRuntime(absolute, owner, pin, database, profile)
    } catch (error) { database.close(); throw error }
  }
  private assertOpen() {
    if (this.closed) throw new DataError(0, 'CLIENT_CLOSED')
  }
  prime(scope: string) {
    return this.track(async () => {
      const owner = encodeURIComponent(this.owner), device = encodeURIComponent(this.profile.device)
      const identity = `owner_id=${owner}&device_id=${device}`
      await this.store.watchResources([
        `/v1/tasks?${identity}&include_deleted=false`, `/v1/tasks?${identity}&include_deleted=true`,
        `/v1/transfers?${identity}`, `/v1/notifications?${identity}&state=PENDING`,
        `/v1/devices?owner_id=${owner}`, `/v1/quota?scope_id=${encodeURIComponent(scope)}`,
      ])
      await this.store.patchTarget({ prefetch: true })
    })
  }
  private track<T>(work: () => Promise<T>): Promise<T> {
    this.assertOpen()
    const operation = Promise.resolve().then(work)
    this.operations.add(operation)
    void operation.then(() => this.operations.delete(operation), () => this.operations.delete(operation))
    return operation
  }
  private async wire<T>(method: string, path: string, input?: unknown, signed = true): Promise<T> {
    this.assertOpen()
    const controller = new AbortController()
    this.controllers.add(controller)
    const timeout = setTimeout(() => controller.abort(), 6000)
    try {
      const body = input === undefined ? '' : JSON.stringify(input)
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (signed) {
        const target = await this.store.target()
        const timestamp = Math.floor((Date.now() + target.offset) / 1000).toString()
        const bytes = utf8.encode(`${method}\n${path}\n${timestamp}\n${await digest(utf8.encode(body))}`)
        headers['X-Pinelink-Owner'] = this.owner
        headers['X-Pinelink-Device'] = this.profile.device
        headers['X-Pinelink-Timestamp'] = timestamp
        headers['X-Pinelink-Signature'] = base64(new Uint8Array(await crypto.subtle.sign('Ed25519', this.profile.signingPrivate, bytes)))
      }
      this.assertOpen()
      controller.signal.throwIfAborted()
      const response = await fetch(this.server + path, { method, headers, body: method === 'GET' ? undefined : body, signal: controller.signal, redirect: 'error', credentials: 'omit', cache: 'no-store' })
      const reader = response.body?.getReader()
      let text = ''
      if (reader) {
        let total = 0
        const decoder = new TextDecoder('utf-8', { fatal: true })
        while (true) {
          const part = await reader.read()
          if (part.done) break
          total += part.value.length
          if (total > 12 * 1024 * 1024) { await reader.cancel(); throw new Error('RESPONSE_TOO_LARGE') }
          text += decoder.decode(part.value, { stream: true })
        }
        text += decoder.decode()
      } else text = await response.text()
      this.assertOpen()
      controller.signal.throwIfAborted()
      let value: unknown
      try { value = text ? JSON.parse(text) : {} } catch { throw new DataError(response.status, 'INVALID_SERVER_RESPONSE') }
      if (!response.ok) throw new DataError(response.status, (value as { code?: string })?.code ?? 'REQUEST_FAILED')
      return value as T
    } finally { clearTimeout(timeout); this.controllers.delete(controller) }
  }
  async request<T>(method: string, path: string, input?: unknown): Promise<T> {
    return this.track(() => this.performRequest<T>(method, path, input))
  }
  private async performRequest<T>(method: string, path: string, input?: unknown): Promise<T> {
    this.assertOpen()
    if (path === '/v1/devices/revoke' && (input as { device_id?: string })?.device_id !== this.profile.device) throw new DataError(403, 'DEVICE_REVOCATION_FORBIDDEN')
    let started = 0
    try {
      if (navigator.onLine === false && method === 'GET' && cacheable(path)) {
        const saved = await this.store.responses.get(path)
        if (saved?.invalid) throw new DataError(saved.invalid.status, saved.invalid.code)
        if (saved && !saved.pending) return saved.value as T
      }
      if (!(await this.store.target()).bound) {
        try { await this.sync(true) } catch (error) { if (!(await this.store.target()).bound) throw error }
      }
      for (let attempt = 0; attempt < 2; attempt++) {
        const cached = method === 'GET' && cacheable(path)
        started = cached ? await this.store.beginRead() : 0
        const result = await this.wire<T>(method, path, input)
        this.assertOpen()
        if (cached) {
          await this.store.cacheResponse(path, result, started)
          const saved = await this.store.responses.get(path)
          this.assertOpen()
          if (saved?.invalid) throw new DataError(saved.invalid.status, saved.invalid.code)
          if (saved && !saved.pending) return saved.value as T
          // A control completed during the very first read. No earlier cache
          // exists to use; obtain a fresh snapshot instead of returning stale data.
          continue
        }
        if (method !== 'GET') await this.store.cacheMutation(result)
        this.assertOpen()
        return result
      }
      throw new DataError(409, 'RESPONSE_SUPERSEDED')
    } catch (error) {
      this.assertOpen()
      if (started && method === 'GET' && cacheable(path) && invalidatesCache(error)) await this.store.invalidateResponse(path, { status: error.status, code: error.code }, started)
      const retryable = !(error instanceof DataError) || error.status >= 500 || error.status === 408 || error.status === 429
      if (method === 'GET' && cacheable(path) && retryable) {
        if (started) await this.store.noteRefreshFailure(path, started)
        void requestBackgroundSync()
        const saved = await this.store.responses.get(path)
        this.assertOpen()
        if (saved?.invalid) throw new DataError(saved.invalid.status, saved.invalid.code)
        if (saved && !saved.pending) return saved.value as T
      }
      throw error
    }
  }
  private async checkProfile(profile: SignedProfile) {
    if (profile.root !== this.profile.root || !await verify(this.pin, proof('profile', profile), profile.signature)) throw new Error('INVALID_SERVER_PROFILE')
    return profile
  }
  private async pass() {
    let target = await this.store.target()
    let epoch = 'API_ONLY'
    if (this.pin) {
      const challenge = nonce()
      const info = await this.wire<ServerInfo>('GET', '/v1/replication/info?nonce=' + challenge, undefined, false)
      if (info.version !== 1 || info.public_key !== this.pin || info.nonce !== challenge || !await verify(this.pin, proof('info', info), info.signature)) throw new Error('INVALID_SERVER_INFO')
      epoch = info.epoch
      await this.store.patchTarget({ offset: info.unix_time * 1000 - Date.now() })
    }
    if (target.bound !== epoch) {
      await this.wire('POST', '/v1/devices/register', { owner_id: this.owner, device_id: this.profile.device, public_key: this.profile.encryptionKey, signing_public_key: this.profile.signingKey })
      if (this.pin) {
        const profile = await this.checkProfile(await this.wire<SignedProfile>('POST', '/v1/replication/bind', { root: this.profile.root, signing_key: this.profile.signingKey, encryption_key: this.profile.encryptionKey }))
        if (profile.epoch !== epoch) throw new Error('EPOCH_CHANGED')
        await this.store.patchTarget({ bound: epoch, rosterVersion: profile.roster_version, rosterHash: '', pendingRoster: undefined })
        await this.store.reset(epoch)
      } else await this.store.patchTarget({ bound: epoch })
    }
    if (this.pin) {
      const profile = await this.checkProfile(await this.wire<SignedProfile>('GET', '/v1/replication/profile?stream=' + this.profile.stream))
      if (profile.epoch !== epoch) throw new Error('EPOCH_CHANGED')
      const peers = (await this.store.peers.toArray()).filter(p => !p.revoked).sort((a, b) => a.root.localeCompare(b.root)).map(({ root, signing_key, encryption_key }) => ({ root, signing_key, encryption_key }))
      const hash = await digest(utf8.encode(canonical(peers)))
      target = await this.store.target()
      if (target.rosterHash !== hash || target.pendingRoster) {
        const pending = target.pendingRoster ?? { operation_id: crypto.randomUUID(), expected_version: profile.roster_version, peers }
        await this.store.patchTarget({ pendingRoster: pending })
        try {
          const acknowledged = await this.checkProfile(await this.wire<SignedProfile>('PUT', '/v1/replication/peers', pending))
          await this.store.patchTarget({ rosterVersion: acknowledged.roster_version, rosterHash: await digest(utf8.encode(canonical((pending as { peers: unknown }).peers))), pendingRoster: undefined })
        } catch (error) {
          if (error instanceof DataError && error.status === 409) await this.store.patchTarget({ pendingRoster: undefined })
          throw error
        }
        const current = (await this.store.peers.toArray()).filter(p => !p.revoked).sort((a, b) => a.root.localeCompare(b.root)).map(({ root, signing_key, encryption_key }) => ({ root, signing_key, encryption_key }))
        if (await digest(utf8.encode(canonical(current))) !== hash) return
      }
      for (let i = 0; i < 4; i++) {
        target = await this.store.target()
        if (target.epoch !== epoch) { await this.store.reset(epoch); target = await this.store.target() }
        const path = target.bootstrapped ? 'events' : 'snapshot'
        const query = new URLSearchParams({ epoch, after: String(target.cursor), upper: String(target.upper), key: target.key, limit: '100' })
        let page: Page
        try { page = await this.wire<Page>('GET', `/v1/replication/${path}?${query}`) } catch (error) {
          if (error instanceof DataError && error.status === 409) await this.store.reset(epoch)
          throw error
        }
        if (page.epoch !== epoch || page.after !== target.cursor || Boolean(page.snapshot) === target.bootstrapped || (!target.bootstrapped && target.upper >= 0 && page.upper !== target.upper)) throw new Error('INVALID_SNAPSHOT_RANGE')
        const trusted = new Map((await this.store.peers.toArray()).map(p => [p.root, p]))
        await this.store.apply(this.profile, page, await preparePage(this.profile, page, this.pin, trusted))
        if (!page.more) break
      }
    }
    let retry: unknown
    for (const row of await this.store.reserveRefreshes(4)) {
      const started = await this.store.beginRead()
      try {
        const value = await this.wire('GET', row.path)
        await this.store.cacheResponse(row.path, value, started)
        if (row.path.startsWith('/v1/tasks?') && (await this.store.target()).prefetch) {
          const tasks = (value as { tasks?: { kind?: string; reference_id?: string }[] })?.tasks
          if (Array.isArray(tasks)) await this.store.watchResources(tasks.flatMap(task => {
            if (!/^[a-f0-9-]{36}$/i.test(task.reference_id ?? '')) return []
            const id = task.reference_id!
            if (task.kind === 'UPLOAD') return [`/v1/files/${id}?owner_id=${encodeURIComponent(this.owner)}`]
            if (task.kind === 'DOWNLOAD') return [`/v1/pulls/${id}?owner_id=${encodeURIComponent(this.owner)}&device_id=${encodeURIComponent(this.profile.device)}`]
            return []
          }))
        }
      } catch (error) {
        this.assertOpen()
        if (invalidatesCache(error)) await this.store.invalidateResponse(row.path, { status: error.status, code: error.code }, started)
        else { await this.store.noteRefreshFailure(row.path, started); retry ??= error }
      }
    }
    if (retry) throw retry
  }
  sync(force = false): Promise<void> {
    if (this.closed) return Promise.reject(new DataError(0, 'CLIENT_CLOSED'))
    if (this.active) {
      const active = this.active, attempt = this.attempt
      return force ? active.then(() => attempt?.skipped ? this.sync(true) : undefined) : active
    }
    const attempt: { skipped: boolean } = { skipped: false }
    this.attempt = attempt
    const work = async () => {
      this.assertOpen()
      const target = await this.store.target()
      const delay = target.retryAt - Date.now()
      if (!force && delay > 0 && delay <= 60000) return
      try {
        // onLine is a hint: a local/self-hosted service may still be reachable
        // without an Internet connection. Real requests decide reachability.
        await this.pass()
        this.catchup = Boolean((await this.store.target()).more) || await this.store.responses.where('pending').equals(1).count() > 0
        await this.store.patchTarget({ failures: 0, retryAt: Date.now() + (this.catchup ? 500 : 5000), error: '' })
      } catch (error) {
        this.assertOpen()
        this.catchup = false
        const failures = Math.min((await this.store.target()).failures + 1, 6)
        await this.store.patchTarget({ failures, retryAt: Date.now() + Math.min(60000, 1000 * 2 ** failures), error: error instanceof DataError ? error.code : 'SYNC_RETRY_PENDING' })
        if (!(error instanceof DataError) || error.status >= 500 || [408, 429].includes(error.status)) void requestBackgroundSync()
        throw error
      }
    }
    this.active = this.track(() => navigator.locks
      ? navigator.locks.request(`sync:${this.store.name}`, force ? { signal: this.lifetime.signal } : { ifAvailable: true }, lock => {
        if (lock) return work()
        attempt.skipped = true
      })
      : work()).finally(() => { this.active = undefined })
    return this.active
  }
  start() {
    this.assertOpen()
    if (this.started) return
    this.started = true
    const generation = ++this.generation
    window.addEventListener('online', this.wake)
    window.addEventListener('pageshow', this.wake)
    document.addEventListener('visibilitychange', this.wake)
    const loop = async () => {
      if (!this.started || generation !== this.generation) return
      if (document.visibilityState !== 'hidden') await this.sync().catch(() => undefined)
      if (this.started && generation === this.generation) this.timer = setTimeout(loop, this.catchup ? 500 : 5000)
    }
    void loop()
  }
  stop() {
    this.started = false
    ++this.generation
    clearTimeout(this.timer)
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', this.wake)
      window.removeEventListener('pageshow', this.wake)
      document.removeEventListener('visibilitychange', this.wake)
    }
  }
  close(): Promise<void> {
    if (this.closing) return this.closing
    this.closed = true
    this.stop()
    this.lifetime.abort()
    for (const c of this.controllers) c.abort()
    this.closing = Promise.allSettled([...this.operations, ...(this.active ? [this.active] : [])]).then(() => { this.store.close() })
    return this.closing
  }
}
