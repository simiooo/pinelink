import Dexie from 'dexie'
import type { Table } from 'dexie'
import { digest, newProfile, utf8 } from './crypto'
import type { DeviceProfile } from './crypto'
import type { Envelope, Page } from './wire'

export type Peer = { root: string; signing_key: string; encryption_key: string; revoked: boolean }
export type Target = { id: string; epoch: string; cursor: number; bootstrapped: boolean; upper: number; key: string; bound: string; rosterVersion: number; rosterHash: string; pendingRoster?: object; failures: number; retryAt: number; error: string; offset: number; cacheClock?: number; cacheBarrier?: number; cacheRevision?: number; backgroundThrough?: number; more?: boolean; prefetch?: boolean }
export type HistoryFact = { id: string; authority: string; aggregate: string; source: string; kind: string; version: number; fact: Record<string, unknown> }
export type PreparedFact = { id: string; hash: string; event: Envelope; fact?: Record<string, unknown>; trustedKey?: string }
export type CachedResponse = { path: string; value: unknown; updated: number; checked?: number; retryPending?: number; pending?: number; invalid?: { status: number; code: string } }

export class LocalStore extends Dexie {
  profiles!: Table<DeviceProfile, string>
  targets!: Table<Target, string>
  peers!: Table<Peer, string>
  inbox!: Table<{ id: string; hash: string; event: Envelope; status: string }, string>
  history!: Table<HistoryFact, string>
  responses!: Table<CachedResponse, string>
  constructor(name: string) {
    super(name)
    this.version(1).stores({ profiles: 'id', targets: 'id', peers: 'root', inbox: 'id,[event.source_root+event.stream_id]', history: 'id,aggregate,source,kind', responses: 'path,updated' })
    this.version(2).stores({ responses: 'path,updated,checked,retryPending,pending' }).upgrade(tx => tx.table('responses').toCollection().modify(row => { row.checked = row.updated; row.retryPending = 0 }))
  }
  watchResources(paths: string[]) {
    return this.transaction('rw', this.responses, async () => {
      const unique = [...new Set(paths)]
      const existing = await this.responses.bulkGet(unique)
      const missing = unique.filter((_, index) => !existing[index])
      if (missing.length) await this.responses.bulkAdd(missing.map(path => ({ path, value: undefined, updated: 0, checked: 0, pending: 1, retryPending: 0 })))
    })
  }
  async profile(device?: string) {
    const existing = await this.profiles.get('self')
    if (existing) return existing
    // Crypto runs outside IndexedDB transactions; concurrent first opens keep
    // the first committed key instead of rotating an established identity.
    const generated = await newProfile(device)
    return this.transaction('rw', this.profiles, async () => {
      const won = await this.profiles.get('self')
      if (won) return won
      await this.profiles.add(generated)
      return generated
    })
  }
  async target(): Promise<Target> {
    return await this.targets.get('cloud') ?? { id: 'cloud', epoch: '', cursor: 0, bootstrapped: false, upper: -1, key: '', bound: '', rosterVersion: 0, rosterHash: '', failures: 0, retryAt: 0, error: '', offset: 0 }
  }
  async patchTarget(fields: Partial<Target>) {
    await this.transaction('rw', this.targets, async () => this.targets.put({ ...await this.target(), ...fields }))
  }
  // Reserve in the database, rather than Date.now(): tabs share the same order,
  // including same-millisecond requests and changes to the system clock.
  private async advanceCacheClock(mutation = false) {
    const target = await this.target()
    const latest = await this.responses.orderBy('updated').last()
    const revision = Math.max(target.cacheClock ?? 0, latest?.updated ?? 0, Date.now()) + 1
    if (!Number.isSafeInteger(revision)) throw new Error('CACHE_CLOCK_EXHAUSTED')
    await this.targets.put({ ...target, cacheClock: revision, ...(mutation ? { cacheBarrier: revision } : {}) })
    return revision
  }
  beginRead() {
    return this.transaction('rw', [this.targets, this.responses], () => this.advanceCacheClock())
  }
  private async cacheChanged() {
    const target = await this.target()
    const cacheRevision = (target.cacheRevision ?? 0) + 1
    if (!Number.isSafeInteger(cacheRevision)) throw new Error('CACHE_REVISION_EXHAUSTED')
    await this.targets.put({ ...target, cacheRevision })
  }
  async cacheResponse(path: string, value: unknown, started: number) {
    return this.transaction('rw', [this.targets, this.responses], async () => {
      const old = await this.responses.get(path)
      if (started < ((await this.target()).cacheBarrier ?? 0) || (old && old.updated > started)) return false
      await this.responses.put({ path, value, updated: started, checked: Math.max(old?.checked ?? 0, started), retryPending: 0 })
      if (!old || old.invalid || JSON.stringify(old.value) !== JSON.stringify(value)) await this.cacheChanged()
      return true
    })
  }
  // Keep the last good value on a network outage, but never resurrect a response
  // the server has explicitly denied or removed, even after an offline restart.
  async invalidateResponse(path: string, invalid: { status: number; code: string }, started: number) {
    return this.transaction('rw', [this.targets, this.responses], async () => {
      const old = await this.responses.get(path)
      if (started < ((await this.target()).cacheBarrier ?? 0) || (old && old.updated > started)) return
      await this.responses.put({ path, value: undefined, invalid, updated: started, checked: Math.max(old?.checked ?? 0, started), retryPending: 0 })
      if (!old?.invalid || old.invalid.status !== invalid.status || old.invalid.code !== invalid.code) await this.cacheChanged()
    })
  }
  noteRefreshFailure(path: string, started: number) {
    return this.transaction('rw', this.responses, async () => {
      const row = await this.responses.get(path)
      if (row && row.updated <= started) await this.responses.update(path, { retryPending: 1 })
    })
  }
  reserveRefreshes(limit: number) {
    return this.transaction('rw', [this.targets, this.responses], async () => {
      const rows = await this.responses.orderBy('checked').limit(limit).toArray()
      if (!rows.length) return rows
      const checked = await this.advanceCacheClock()
      for (const row of rows) await this.responses.update(row.path, { checked })
      return rows
    })
  }
  async cacheMutation(value: unknown) {
    await this.transaction('rw', [this.targets, this.responses], async () => {
      const revision = await this.advanceCacheClock(true)
      if (!value || typeof value !== 'object' || !('id' in value)) return
      const entity = value as Record<string, unknown>
      let anyChanged = false
      for (const row of await this.responses.toArray()) {
        if (!row.value || typeof row.value !== 'object') continue
        const next = structuredClone(row.value) as Record<string, unknown>
        let changed = false
        for (const key of ['tasks', 'transfers', 'devices']) {
          if (!Array.isArray(next[key])) continue
          next[key] = (next[key] as Record<string, unknown>[]).flatMap(item => {
            if (item.id !== entity.id || Number(item.version || 0) > Number(entity.version || 0)) return [item]
            changed = true
            return entity.state === 'DELETED' && row.path.includes('include_deleted=false') ? [] : [entity]
          })
        }
        if (next.id === entity.id && Number(next.version || 0) <= Number(entity.version || 0)) { Object.assign(next, entity); changed = true }
        if (changed) { await this.responses.put({ ...row, value: next, updated: revision }); anyChanged = true }
      }
      if (anyChanged) await this.cacheChanged()
    })
  }
  async reset(epoch: string) {
    await this.patchTarget({ epoch, cursor: 0, bootstrapped: false, upper: -1, key: '', more: true })
  }
  async apply(profile: DeviceProfile, page: Page, rows: PreparedFact[]) {
    await this.transaction('rw', [this.profiles, this.targets, this.peers, this.inbox, this.history], async () => {
      if ((await this.profiles.get('self'))?.root !== profile.root) throw new Error('PROFILE_CHANGED')
      const target = await this.target()
      if (target.epoch && target.epoch !== page.epoch) throw new Error('EPOCH_CHANGED')
      for (const row of rows) {
        const old = await this.inbox.get(row.id)
        if (old && old.hash !== row.hash) throw new Error('EVENT_CONFLICT')
        if (old?.status === 'APPLIED') continue
        const peer = row.event.source_root === profile.root ? undefined : await this.peers.get(row.event.source_root)
        const trusted = row.event.authority === 'SERVER' || row.event.source_root === profile.root || (peer && !peer.revoked && peer.signing_key === row.trustedKey)
        const applied = trusted && row.fact !== undefined
        await this.inbox.put({ id: row.id, hash: row.hash, event: row.event, status: applied ? 'APPLIED' : 'IGNORED_UNTRUSTED' })
        if (applied) {
          const e = row.event
          const id = [e.authority_root, e.aggregate_id, e.source_root, e.kind].join('|')
          const previous = await this.history.get(id)
          if (!previous || e.aggregate_version > previous.version) await this.history.put({ id, authority: e.authority_root, aggregate: e.aggregate_id, source: e.source_root, kind: e.kind, version: e.aggregate_version, fact: row.fact! })
        }
      }
      if (page.snapshot && target.bootstrapped && target.epoch === page.epoch) return
      const next = { ...target, epoch: page.epoch, more: page.more }
      if (page.snapshot && page.more) { next.upper = page.upper; next.key = page.next_key || '' }
      else { next.cursor = Math.max(target.cursor, page.through); next.bootstrapped = true; next.upper = -1; next.key = '' }
      await this.targets.put(next)
    })
  }
}
export const storeName = async (server: string, owner: string, device: string) => 'pinelink-' + await digest(utf8.encode(`${server}\n${owner}\n${device}`))
