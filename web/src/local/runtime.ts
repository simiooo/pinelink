import { digest, newProfile, nonce } from '../data/crypto'
import type { DeviceProfile } from '../data/crypto'
import { decodeFileFrame, decodeText, decryptDare, encodeFileFrame, hashBlob, openManifest, prepareFile, safeFileName, suite, wireChunkSize, cipherSize, textByteLimit } from '../guest/crypto'
import type { StoredText } from '../guest/text-storage'
import { directDelay, fragmentFrame, FrameAssembler } from '../guest/direct'
import type { GuestJob, GuestStore } from '../guest/store'
import type { GuestPeer, GuestTransfer, ProgressSnapshot } from '../guest/types'
import { decodePackage, encodePackage, isLocalID, localID, signed, verified } from './protocol'
import type { LocalState, LocalReceipt } from './protocol'
import { LocalSession } from './session'

type Pairing = { phase: 'idle' | 'preparing' | 'show' | 'reply' | 'verify' | 'waiting' | 'connected' | 'expired'; code?: string; peer?: string; value?: string; expected?: string; error?: string }
const terminal = (s: string) => ['COMPLETED','CANCELLED','REJECTED','EXPIRED','FAILED'].includes(s)
const active = (j: GuestJob) => j.authority === 'LOCAL_DEVICE' && !['complete','stopped'].includes(j.phase)
const name = () => /iPhone/.test(navigator.userAgent) ? 'iPhone' : /iPad/.test(navigator.userAgent) ? 'iPad' : /Android/.test(navigator.userAgent) ? 'Android' : /Macintosh/.test(navigator.userAgent) ? 'Mac' : /Windows/.test(navigator.userAgent) ? 'Windows PC' : 'Web browser'
export class LocalRuntime {
  private stopped = false
  private life = new AbortController()
  private pair: Pairing = { phase: 'idle' }
  private pending?: LocalSession
  private links = new Map<string, LocalSession>()
  private sessions = new Set<LocalSession>()
  private listeners = new Set<() => void>()
  private serial = new Map<string, Promise<unknown>>()
  private releases = new Map<LocalSession, () => void>()
  private owner = nonce()
  private workers = new Map<string, { version: number; controller: AbortController; task: Promise<void>; sent: number }>()
  private assemblers = new Map<string, { version: number; assembler: FrameAssembler }>()
  private manifests = new Set<string>()
  private verifying = new Map<string, Promise<void>>()
  private timer?: ReturnType<typeof setInterval>
  private pulse?:Promise<void>
  private mutations = new Set<Promise<unknown>>()
  private constructor(readonly store: GuestStore, readonly profile: DeviceProfile, private changed: () => Promise<void>) {}
  static async open(store: GuestStore, changed: () => Promise<void>) {
    let profile = await store.profiles.get('local')
    if (!profile) { const fresh = { ...await newProfile(), id: 'local' }; profile = await store.transaction('rw', store.profiles, async () => { const won = await store.profiles.get('local'); if (won) return won; await store.profiles.add(fresh); return fresh }) }
    const r = new LocalRuntime(store, profile, changed)
    await store.localUsed.where('expires').below(Date.now()).delete()
    r.timer = setInterval(() => { if(!r.pulse)r.pulse=r.heartbeat().catch(()=>undefined).finally(()=>{r.pulse=undefined}) }, 2000)
    return r
  }
  getSnapshot = () => this.pair
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  private update(p: Partial<Pairing>) { if (this.stopped) return; this.pair = { ...this.pair, ...p }; for (const fn of this.listeners) fn() }
  private task<T>(id: string, work: () => Promise<T>): Promise<T> {
    const result = (this.serial.get(id) ?? Promise.resolve()).catch(() => undefined).then(() => { this.life.signal.throwIfAborted(); return work() })
    this.serial.set(id, result); this.mutations.add(result); void result.finally(() => { this.mutations.delete(result); if (this.serial.get(id) === result) this.serial.delete(id) }).catch(() => undefined); return result
  }
  reachable(root?: string) { return !!root && !!this.links.get(root)?.isReady() }
  private peer(link: LocalSession): GuestPeer {
    const p = link.peer!; return { pair_id: 'local:' + p.root, owner_id: p.root, root: p.root, device_id: p.device, device_name: p.name, signing_key: p.signing_key, encryption_key: p.encryption_key, expires_at: '9999-01-01T00:00:00Z', authority: 'LOCAL_DEVICE', online: link.isReady() }
  }
  private async reserve(link: LocalSession, root: string) {
    const key = this.store.name + ':local-peer:' + root
    let release = () => {}
    if (navigator.locks) release = await new Promise<() => void>((resolve, reject) => {
      void navigator.locks.request(key, { ifAvailable: true }, async lock => { if (!lock) { reject(new Error('LOCAL_OTHER_TAB')); return }; if(this.life.signal.aborted){reject(this.life.signal.reason);return}; await new Promise<void>(done => resolve(done)) }).catch(reject)
    })
    try {
      await this.store.transaction('rw', this.store.localLeases, async () => { const row = await this.store.localLeases.get(root); if (!navigator.locks&&row && row.owner !== this.owner && row.until > Date.now()) throw new Error('LOCAL_OTHER_TAB'); await this.store.localLeases.put({ id: root, owner: this.owner, until: Date.now() + 15000,session:link.id }) })
      link.signal.throwIfAborted()
      this.releases.set(link, release)
    } catch (e) { release();await this.store.transaction('rw',this.store.localLeases,async()=>{const row=await this.store.localLeases.get(root);if(row?.owner===this.owner&&row.session===link.id)await this.store.localLeases.delete(root)}).catch(()=>undefined);throw e }
  }
  private async assertLease(root: string) { const lease = await this.store.localLeases.get(root); if (lease?.owner !== this.owner) throw new Error('LOCAL_OTHER_TAB');await this.store.localLeases.update(root,{until:Date.now()+15000}) }
  private async controlLease<T>(root:string,work:()=>Promise<T>):Promise<T>{
    if((await this.store.localLeases.get(root))?.owner===this.owner)return work()
    const action=async()=>{
      const stamp='control:'+nonce()
      await this.store.transaction('rw',this.store.localLeases,async()=>{const old=await this.store.localLeases.get(root);if(!navigator.locks&&old&&old.owner!==this.owner&&old.until>Date.now())throw new Error('LOCAL_OTHER_TAB');await this.store.localLeases.put({id:root,owner:this.owner,until:Date.now()+15000,session:stamp})})
      try{return await work()}finally{await this.store.transaction('rw',this.store.localLeases,async()=>{const row=await this.store.localLeases.get(root);if(row?.owner===this.owner&&row.session===stamp)await this.store.localLeases.delete(root)})}
    }
    return navigator.locks?navigator.locks.request(this.store.name+':local-peer:'+root,{ifAvailable:true},lock=>{if(!lock)throw new Error('LOCAL_OTHER_TAB');return action()}):action()
  }
  private create(incoming: boolean, id?: string) {
    const link = new LocalSession(this.profile, name(), this.life.signal, incoming, {
      opened: async l => {
        if (this.pair.expected && this.pair.expected !== l.peer!.root) throw new Error('LOCAL_IDENTITY_CHANGED')
        const known = await this.store.localPeers.get('local:' + l.peer!.root)
        this.update({ phase: 'verify', code: l.code, peer: l.peer!.name })
        if (known) { if (known.signing_key !== l.peer!.signing_key || known.encryption_key !== l.peer!.encryption_key) throw new Error('LOCAL_IDENTITY_CHANGED'); await l.confirm() }
      },
      control: (v,l) => this.controlMessage(v,l), data: (v,l) => this.dataMessage(v,l),
      changed: () => { if (link === this.pending && link.code) this.update({ code: link.code, peer: link.peer?.name, phase: link.isReady() ? 'connected' : link.selfConfirmed ? 'waiting' : 'verify' }) },
      closed: l => this.closed(l),
    }, id)
    this.sessions.add(link); this.pending = link; return link
  }
  async invite(expected?: string) {
    if (this.pending && !this.pending.isReady()) this.pending.close()
    this.update({ phase: 'preparing', error: undefined, value: undefined, code: undefined, expected })
    const link = this.create(false)
    try { const value = await encodePackage(await link.createOffer()); link.signal.throwIfAborted(); this.update({ phase: 'show', value }); return value } catch (e) { link.close(); throw e }
  }
  async join(code: string, expected?: string) {
    const packet = await decodePackage(code)
    if (packet.peer.root === this.profile.root) throw new Error('LOCAL_SELF')
    if (expected && expected !== packet.peer.root) throw new Error('LOCAL_IDENTITY_CHANGED')
    if (packet.type === 'answer') {
      const link = this.pending
      if (!link || link.incoming || link.id !== packet.id || link.signal.aborted) throw new Error('LOCAL_CODE_EXPIRED')
      if(link.peer)throw new Error('LOCAL_CODE_USED')
      await this.reserve(link, packet.peer.root)
      try { await link.setAnswer(packet); this.update({ phase: 'preparing' }); return } catch (e) { link.close(); throw e }
    }
    const usedID = packet.id + ':' + packet.nonce
    if (await this.store.localUsed.get(usedID)) throw new Error('LOCAL_CODE_USED')
    if (this.pending && !this.pending.isReady()) this.pending.close()
    this.update({ phase: 'preparing', error: undefined, value: undefined, code: undefined, expected })
    const link = this.create(true, packet.id)
    try {
      await this.reserve(link, packet.peer.root)
      await this.store.transaction('rw', this.store.localUsed, async () => { if (await this.store.localUsed.get(usedID)) throw new Error('LOCAL_CODE_USED'); await this.store.localUsed.add({ id: usedID, expires: Date.now() + 86400000 }) })
      const reply = await encodePackage(await link.createAnswer(packet)); link.signal.throwIfAborted(); this.update({ phase: 'reply', value: reply }); return reply
    } catch (e) { link.close(); throw e }
  }
  async confirm() { if (!this.pending) throw new Error('LOCAL_CODE_EXPIRED'); await this.pending.confirm() }
  cancelPairing() { if (this.pending && !this.pending.isReady()) this.pending.close(); this.pending = undefined; this.update({ phase: 'idle', value: undefined, code: undefined, error: undefined }) }
  private async paired(link: LocalSession) {
    const peer = this.peer(link), old = this.links.get(peer.root!)
    this.links.set(peer.root!, link); if (old && old !== link) old.close()
    await this.store.transaction('rw', this.store.localPeers, this.store.localLeases, async () => { await this.assertLease(peer.root!); const known = await this.store.localPeers.get(peer.pair_id); await this.store.localPeers.put({ ...peer, local_name: known?.local_name }) })
    await this.changed();this.update({ phase: 'connected', value: undefined, peer: peer.device_name })
    const jobs = await this.store.jobs.filter(j => j.authority === 'LOCAL_DEVICE' && j.peer.root === peer.root).toArray()
    for (const job of jobs.slice(-100)) {
      if (job.direction === 'receive' && job.local?.state) await link.send({ type: 'state', state: job.local.state })
      else if (job.direction === 'send' && active(job)) {
        if (job.payloadKind === 'TEXT' && !link.peer?.capabilities?.includes('text-v1')) { await this.store.jobs.update(job.id, { phase: 'paused', error: 'TEXT_UNSUPPORTED' }); continue }
        await this.offer(job, link)
      }
    }
  }
  private async closed(link: LocalSession) {
    this.sessions.delete(link); this.releases.get(link)?.(); this.releases.delete(link)
    const root = link.peer?.root
    if (root && this.links.get(root) === link) {
      this.links.delete(root)
      const jobs = await this.store.jobs.filter(j => active(j) && j.peer.root === root).toArray()
      for (const job of this.stopped?[]:jobs) await this.task(job.id, async () => {
        this.workers.get(job.id)?.controller.abort()
        const current = await this.store.jobs.get(job.id); if (!current || !active(current) || this.links.get(root)?.isReady()) return
        if (current.direction === 'receive' && current.local?.state && !current.local.state.paused && current.local.state.state === 'ACCEPTED') await this.changeState(current, { paused: true, version: current.local.state.version + 1 }, undefined)
        const latest=(await this.store.jobs.get(job.id))!; if(!active(latest))return
        await this.store.jobs.update(job.id, { phase: latest.phase === 'waiting' ? 'waiting' : 'paused', error: 'LOCAL_CONNECTION_LOST', local: { ...latest.local!, disconnected: true } })
      }).catch(() => undefined)
      await this.store.transaction('rw', this.store.localLeases, async () => { const row=await this.store.localLeases.get(root);if(row?.owner===this.owner&&row.session===link.id)await this.store.localLeases.delete(root) })
      await this.changed()
    } else if (root) await this.store.transaction('rw', this.store.localLeases, async () => { const row=await this.store.localLeases.get(root);if(!this.links.has(root)&&row?.owner===this.owner&&row.session===link.id)await this.store.localLeases.delete(root) })
    if (link === this.pending && this.pair.phase !== 'connected') this.update({ phase: link.error==='LOCAL_CONNECTION_LOST'?'expired':'idle', value: undefined, error: link.error==='LOCAL_CONNECTION_LOST'?'LOCAL_CODE_EXPIRED':link.error })
  }
  async rename(pair: string, value: string) {
    const trimmed = value.trim(); if (!trimmed || trimmed.length > 64 || /[\u0000-\u001f\u007f]/.test(trimmed)) throw new Error('DEVICE_NAME_INVALID')
    if (!await this.store.localPeers.get(pair)) throw new Error('CONNECTION_GONE'); await this.store.localPeers.update(pair, { local_name: trimmed }); await this.changed()
  }
  async remove(pair: string) {
    const peer = await this.store.localPeers.get(pair); if (!peer) return
    if (await this.store.jobs.filter(j => j.peer.pair_id === pair && (active(j) || j.local?.intent==='cancel'&&!terminal(j.local.state?.state??'')&&j.local.expires>Date.now())).count()) throw new Error('CONNECTION_BUSY')
    const link = this.links.get(peer.root!); if (link) { await link.send({ type: 'forget' }); link.close() }
    await this.store.localPeers.delete(pair); await this.changed()
  }
  async send(file: File, peer: GuestPeer, maxBytes: number, text?: { sensitivity: 'NORMAL' | 'SENSITIVE' }) {
    const link = this.links.get(peer.root!); if (!link?.isReady()) throw new Error('LOCAL_RECONNECT_REQUIRED')
    if (text && !link.peer?.capabilities?.includes('text-v1')) throw new Error('TEXT_UNSUPPORTED')
    if (file.size > Math.min(maxBytes, 64 << 20)) throw new Error('FILE_TOO_LARGE')
    if (await this.store.jobs.filter(active).count() >= 2) throw new Error('GUEST_LIMIT_REACHED')
    const prepared = await prepareFile(file, { sender: this.profile.encryptionKey, receiver: peer.encryption_key }, text), manifest = await openManifest(this.profile, prepared.senderManifest)
    link.signal.throwIfAborted(); const raw = crypto.randomUUID(), id = localID(raw), expires = Date.now() + 600000
    const transfer: GuestTransfer = { id, state: 'PENDING', version: 0, file_name: prepared.name, size_bytes: prepared.cipher.size, content_root: prepared.root, encryption_suite: suite, sender_owner_id: this.profile.root, sender_device_id: this.profile.device, recipient_owner_id: peer.root!, recipient_device_id: peer.device_id, expires_at: new Date(expires).toISOString(), received_bytes: 0 }
    const job: GuestJob = { id, created: Date.now(), direction: 'send', peer, authority: 'LOCAL_DEVICE', prepared, ...(text ? { payloadKind: 'TEXT', text: { cipher: prepared.cipher, manifest: prepared.senderManifest } } : {}), received: 0, name: prepared.name, plainSize: file.size, phase: 'waiting', transfer, local: { expires, plainRoot: manifest.plaintext_root }, transport: { state: 'direct', proof: await link.proof() } }
    await this.store.jobs.add(job); await this.changed(); await this.offer(job, link); return id
  }
  private offer(job: GuestJob, link: LocalSession) {
    if (!job.prepared || !job.transfer || !job.local) throw new Error('FILE_SOURCE_MISSING')
    if (job.payloadKind === 'TEXT' && !link.peer?.capabilities?.includes('text-v1')) throw new Error('TEXT_UNSUPPORTED')
    return link.send({ type: 'offer', id: job.id, root: job.prepared.root, size: job.prepared.cipher.size, name: job.name, plain_size: job.plainSize, ...(job.payloadKind ? { content_kind: job.payloadKind } : {}), remaining: Math.max(0, job.local.expires - Date.now()) })
  }
  private snapshot(s: LocalState, disconnected = false): ProgressSnapshot {
    const done = s.state === 'COMPLETED', running = s.state === 'ACCEPTED' && !s.paused && !disconnected
    return { transfer_id: s.id, state: s.state, state_version: s.version, control_revision: s.version, progress_revision: s.revision, route: { kind: 'DIRECT', epoch: 1 }, delivery: { state: done ? 'DELIVERED' : s.state === 'PENDING' ? 'WAITING' : terminal(s.state) ? s.state : s.paused || disconnected ? 'PAUSED' : s.received === s.size ? 'VERIFYING' : 'RUNNING', acknowledged_bytes: s.received, total_bytes: s.size, revision: s.revision }, spool: { state: 'NOT_REQUIRED', acknowledged_bytes: 0, total_bytes: s.size }, completion: { delivered: done, stored: false }, capabilities: { pause: running && s.received < s.size, resume: s.state === 'ACCEPTED' && (s.paused || disconnected), cancel: !terminal(s.state), restore: false } }
  }
  private fields(job: GuestJob, state: LocalState, disconnected = false): Partial<GuestJob> {
    return { transfer: { ...job.transfer!, state: state.state, version: state.version, received_bytes: state.received }, snapshot: this.snapshot(state, disconnected), local: { ...job.local!, state, disconnected }, received: job.direction === 'receive' ? Math.max(job.received, state.received) : state.received, phase: state.state === 'COMPLETED' ? 'complete' : terminal(state.state) ? 'stopped' : state.state === 'PENDING' ? 'waiting' : state.paused || disconnected ? 'paused' : state.received === state.size ? 'verifying' : 'running', error: undefined }
  }
  private async journal(state: LocalState) {
    const event = { event_id: `${state.recipient}:${state.id}:${state.version}:${state.revision}`, authority: 'LOCAL_DEVICE' as const, authority_device_id: state.recipient, transfer: state.id, snapshot: state, uploaded: 0 }
    if (!await this.store.localJournal.where('event_id').equals(event.event_id).count()) await this.store.localJournal.add(event)
  }
  private async changeState(job: GuestJob, changes: Partial<LocalState>, link?: LocalSession, chunk?: { index: number; bytes: Uint8Array; hash: string }, plain?: Blob, text?: StoredText) {
    const old = job.local!.state!, unsigned = { ...old, ...changes, signature: '' }, state = await signed(this.profile, 'state', unsigned)
    await this.store.transaction('rw', this.store.jobs, this.store.chunks, this.store.localJournal, this.store.localLeases, async () => {
      await this.assertLease(job.peer.root!)
      const current = await this.store.jobs.get(job.id)
      if (!current || current.local?.state?.version !== old.version || current.local.state.revision !== old.revision || terminal(current.local.state.state)) throw new Error('FILE_JOB_CHANGED')
      if (chunk) {
        if (old.paused || old.state !== 'ACCEPTED') throw new Error('FILE_JOB_CHANGED')
        const previous = await this.store.chunks.get([job.id, chunk.index]); if (previous && previous.hash !== chunk.hash) throw new Error('FILE_FRAME_CHANGED')
        if (!previous) await this.store.chunks.add({ job: job.id, index: chunk.index, bytes: new Blob([chunk.bytes]), hash: chunk.hash })
      }
      const fields=this.fields(current,state)
      if(terminal(state.state)&&state.state!=='COMPLETED'){fields.prepared=undefined;fields.local={...fields.local!,manifest:undefined};await this.store.chunks.where('job').equals(job.id).delete()}
      if (text) await this.store.chunks.where('job').equals(job.id).delete()
      await this.store.jobs.update(job.id, { ...fields, ...(plain ? { plain } : {}), ...(text ? { text } : {}) }); await this.journal(state)
    })
    if (link?.isReady()) await link.send({ type: 'state', state }); await this.changed(); return state
  }
  private async offered(v: Record<string, unknown>, link: LocalSession) {
    if (v.content_kind !== undefined && (v.content_kind !== 'TEXT' || !link.peer?.capabilities?.includes('text-v1') || v.name !== 'Text' || Number(v.plain_size) < 1 || Number(v.plain_size) > textByteLimit)) throw new Error('LOCAL_MESSAGE_INVALID')
    if (typeof v.id !== 'string' || !isLocalID(v.id) || typeof v.root !== 'string' || !/^[a-f0-9]{64}$/.test(v.root) || !Number.isSafeInteger(v.size) || Number(v.size) < 0 || Number(v.size) > cipherSize(64 << 20) || !Number.isSafeInteger(v.plain_size) || Number(v.plain_size) < 0 || cipherSize(Number(v.plain_size)) !== v.size || typeof v.name !== 'string' || safeFileName(v.name) !== v.name || !Number.isSafeInteger(v.remaining) || Number(v.remaining) < 1 || Number(v.remaining) > 600000) throw new Error('LOCAL_MESSAGE_INVALID')
    const id = v.id
    await this.task(id, async () => {
      const old = await this.store.jobs.get(id)
      if (old) {
        if (old.direction !== 'receive' || old.authority !== 'LOCAL_DEVICE' || old.peer.root !== link.peer!.root || old.transfer!.content_root !== v.root || old.transfer!.size_bytes !== v.size || old.payloadKind !== v.content_kind) throw new Error('LOCAL_MESSAGE_INVALID')
        await link.send({ type: 'state', state: old.local!.state! }); return
      }
      if (await this.store.jobs.filter(active).count() >= 2) throw new Error('GUEST_LIMIT_REACHED')
      const stored = await this.store.jobs.filter(j => j.authority === 'LOCAL_DEVICE').toArray()
      if (stored.reduce((n,j) => n + (j.plain?.size ?? j.prepared?.cipher.size ?? j.text?.cipher.size ?? j.received), 0) + Number(v.size) * 2 > 256 << 20) throw new Error('LOCAL_STORAGE_FULL')
      const expires = Date.now() + Number(v.remaining), peer = this.peer(link)
      const state = await signed(this.profile, 'state', { id, sender: peer.root!, recipient: this.profile.root, root: v.root as string, size: Number(v.size), name: v.name as string, plain_size: Number(v.plain_size), ...(v.content_kind === 'TEXT' ? { content_kind: 'TEXT' as const } : {}), state: 'PENDING' as const, version: 1, revision: 0, received: 0, paused: false })
      const transfer: GuestTransfer = { id, state: 'PENDING', version: 1, file_name: state.name, size_bytes: state.size, content_root: state.root, encryption_suite: suite, sender_owner_id: state.sender, sender_device_id: peer.device_id, recipient_owner_id: state.recipient, recipient_device_id: this.profile.device, expires_at: new Date(expires).toISOString(), received_bytes: 0 }
      const job: GuestJob = { id, created: Date.now(), direction: 'receive', peer, authority: 'LOCAL_DEVICE', ...(state.content_kind ? { payloadKind: state.content_kind } : {}), received: 0, name: state.name, plainSize: state.plain_size, phase: 'waiting', transfer, snapshot: this.snapshot(state), local: { expires, state }, transport: { state: 'direct', proof: await link.proof() } }
      await this.store.transaction('rw', this.store.jobs, this.store.localJournal, this.store.localLeases, async () => { await this.assertLease(peer.root!); if (await this.store.jobs.get(id)) throw new Error('FILE_JOB_CHANGED'); await this.store.jobs.add(job); await this.journal(state) })
      await link.send({ type: 'state', state }); await this.changed()
    })
  }
  private async stateMessage(s: LocalState, link: LocalSession) {
    if (!s || !isLocalID(s.id) || s.recipient !== link.peer!.root || s.sender !== this.profile.root || !Number.isSafeInteger(s.version) || s.version < 1 || !Number.isSafeInteger(s.revision) || s.revision < 0 || !Number.isSafeInteger(s.received) || s.received < 0 || s.received > s.size || typeof s.paused !== 'boolean' || !['PENDING','ACCEPTED','COMPLETED','REJECTED','CANCELLED','EXPIRED','FAILED'].includes(s.state) || !await verified(link.peer!.signing_key, 'state', s)) throw new Error('LOCAL_MESSAGE_INVALID')
    await this.task(s.id, async () => {
      const job = await this.store.jobs.get(s.id); if (!job || job.direction !== 'send' || job.peer.root !== link.peer!.root) return
      if (job.transfer!.content_root !== s.root || job.transfer!.size_bytes !== s.size || job.name !== s.name || job.plainSize !== s.plain_size || job.payloadKind !== s.content_kind) throw new Error('LOCAL_MESSAGE_INVALID')
      const old = job.local!.state
      if (old && (s.version < old.version || s.revision < old.revision || s.received < old.received || terminal(old.state))) return
      if (s.state === 'COMPLETED') {
        const receipt = s.receipt
        if (!receipt || receipt.v !== 1 || receipt.id !== s.id || receipt.sender !== s.sender || receipt.recipient !== s.recipient || receipt.root !== s.root || receipt.plaintext_root !== job.local!.plainRoot || receipt.plaintext_size !== job.plainSize || s.received !== s.size || !await verified(link.peer!.signing_key, 'receipt', receipt)) throw new Error('FILE_CHECKSUM_INVALID')
      }
      const trace=await link.proof(), fields=this.fields(job,s), intent=terminal(s.state)||job.local?.intent==='pause'&&s.paused ? undefined : job.local?.intent
      fields.local={...fields.local!,intent}
      if(intent)fields.phase=intent==='cancel'?'stopped':'paused'
      await this.store.transaction('rw', this.store.jobs, this.store.localJournal, this.store.localLeases, async () => { await this.assertLease(job.peer.root!); await this.store.jobs.update(job.id, { ...fields, ...(terminal(s.state) ? { prepared: undefined } : {}), transport: { state: 'direct', proof: trace } }); await this.journal(s) })
      if (s.paused || terminal(s.state)) this.workers.get(job.id)?.controller.abort()
      await this.changed()
      if (intent && !terminal(s.state)) { await link.send({ type: 'action', id: job.id, action: intent, expected: s.version }); return }
      const key = link.id + ':' + job.id + ':' + s.version
      if (s.state === 'ACCEPTED' && !s.paused && job.prepared && !this.manifests.has(key)) { this.manifests.add(key); await link.send({ type: 'manifest', id: job.id, version: s.version, value: job.prepared.recipientManifest }) }
    })
  }
  async action(id: string, action: 'accept' | 'reject' | 'pause' | 'resume' | 'cancel') {
    const original = await this.store.jobs.get(id); if (!original?.local || !active(original)) return
    const link = this.links.get(original.peer.root!)
    if (original.direction === 'send') return this.task(id,async()=>{
      const original=(await this.store.jobs.get(id))!;if(!active(original))return
      if (!['pause','resume','cancel'].includes(action)) throw new Error('LOCAL_MESSAGE_INVALID')
      this.workers.get(id)?.controller.abort()
      if (!link?.isReady() && action === 'resume') throw new Error('LOCAL_RECONNECT_REQUIRED')
      if (action !== 'resume') await this.store.jobs.update(id, { phase: action === 'cancel' ? 'stopped' : 'paused', ...(action==='cancel'?{prepared:undefined}:{}), local: { ...original.local!, intent: action as 'pause'|'cancel' } })
      else await this.store.jobs.update(id,{local:{...original.local!,intent:undefined}})
      if (link?.isReady()) await link.send({ type: 'action', id, action, expected: original.local!.state?.version ?? 0 })
      await this.changed(); return
    })
    if (!link?.isReady() && ['accept','resume'].includes(action)) throw new Error('LOCAL_RECONNECT_REQUIRED')
    return this.task(id, () => this.controlLease(original.peer.root!,async () => {
      const job = (await this.store.jobs.get(id))!, s = job.local!.state!
      if (terminal(s.state)) return
      if (Date.now() >= job.local!.expires) { await this.changeState(job, { state: 'EXPIRED', version: s.version + 1 }, link); return }
      if (action === 'accept' && s.state !== 'PENDING' || action === 'reject' && s.state !== 'PENDING' || ['pause','resume'].includes(action) && s.state !== 'ACCEPTED') throw new Error('FILE_JOB_CHANGED')
      if (action === 'pause' && s.paused || action === 'resume' && !s.paused) { if(link?.isReady())await link.send({type:'state',state:s});return }
      this.workers.get(id)?.controller.abort(); this.assemblers.delete(id)
      return this.changeState(job, { version: s.version + 1, state: action === 'accept' ? 'ACCEPTED' : action === 'reject' ? 'REJECTED' : action === 'cancel' ? 'CANCELLED' : s.state, paused: action === 'pause' ? true : action === 'resume' || action === 'accept' ? false : s.paused }, link)
    }))
  }
  private async manifest(v: Record<string, unknown>, link: LocalSession) {
    if (typeof v.id !== 'string' || typeof v.value !== 'string' || v.value.length > 8192) throw new Error('LOCAL_MESSAGE_INVALID')
    await this.task(v.id, async () => {
      const job = await this.store.jobs.get(v.id as string), s = job?.local?.state
      if (!job || job.direction !== 'receive' || job.peer.root !== link.peer!.root || !s || s.state !== 'ACCEPTED' || s.paused || s.version !== v.version) return
      const manifest = await openManifest(this.profile, v.value as string)
      if (manifest.plaintext_size !== job.plainSize || cipherSize(manifest.plaintext_size) !== s.size || safeFileName(manifest.file_name) !== job.name || (manifest.content_kind === 'TEXT') !== (job.payloadKind === 'TEXT')) throw new Error('FILE_MANIFEST_INVALID')
      await this.store.jobs.update(job.id, { local: { ...job.local!, manifest: v.value as string, disconnected: false }, error: undefined, phase: 'running', transport: { state: 'direct', proof: await link.proof() } })
      this.assemblers.set(job.id, { version: s.version, assembler: new FrameAssembler(s.size, Math.floor(s.received / wireChunkSize)) })
      await link.send({ type: 'ready', id: job.id, version: s.version, from: s.received }); await this.changed()
      if (s.received === s.size && await this.store.chunks.get([job.id,0])) this.verifyFile(job.id,link)
    })
  }
  private async ready(v: Record<string, unknown>, link: LocalSession) {
    if (typeof v.id !== 'string' || !Number.isSafeInteger(v.from) || Number(v.from) < 0) throw new Error('LOCAL_MESSAGE_INVALID')
    const job = await this.store.jobs.get(v.id), s = job?.local?.state
    if (!job || job.direction !== 'send' || job.peer.root !== link.peer!.root || !job.prepared || !s || s.state !== 'ACCEPTED' || s.paused || s.version !== v.version || v.from !== s.received || Number(v.from) > s.size || Number(v.from) !== s.size && Number(v.from) % wireChunkSize !== 0 || job.local?.intent) return
    this.workers.get(job.id)?.controller.abort()
    const controller = new AbortController(), signal = AbortSignal.any([controller.signal, link.signal]), entry = { version: s.version, controller, task: Promise.resolve(), sent: s.received }
    const work = async () => {
      const bytes = Uint8Array.from(job.id.slice(6).replaceAll('-','').match(/../g)!, x => parseInt(x,16))
      const count = Math.max(1, Math.ceil(s.size/wireChunkSize)), first = s.size>0&&s.received===s.size?count:Math.floor(s.received/wireChunkSize)
      for (let i = first; i < count; i++) {
        const deadline = AbortSignal.any([signal,AbortSignal.timeout(15000)])
        while (true) { signal.throwIfAborted(); const fresh = await this.store.jobs.get(job.id); if (!fresh || fresh.local?.state?.version !== s.version || fresh.phase === 'paused' || fresh.local.intent || !active(fresh)) return; if (entry.sent - (fresh.local.state.received ?? 0) < 4*wireChunkSize) break; await directDelay(15,deadline) }
        const payload = new Uint8Array(await job.prepared!.cipher.slice(i*wireChunkSize,(i+1)*wireChunkSize).arrayBuffer()), frame = await encodeFileFrame(i,payload)
        for (const fragment of fragmentFrame(frame,16360)) { signal.throwIfAborted(); const packet = new Uint8Array(20+fragment.length); packet.set(bytes); new DataView(packet.buffer).setUint32(16,s.version); packet.set(fragment,20); await link.sendData(packet) }
        entry.sent = Math.min(s.size,(i+1)*wireChunkSize)
      }
    }
    entry.task = work().catch(error => { if (!signal.aborted && !this.stopped) { link.close(); return this.store.jobs.update(job.id,{phase:'paused',error:error instanceof Error?error.message:'LOCAL_CONNECTION_LOST'}).then(()=>this.changed()).then(()=>undefined) } }).finally(() => { if (this.workers.get(job.id)===entry) this.workers.delete(job.id) })
    this.workers.set(job.id,entry)
  }
  private async dataMessage(buffer: ArrayBuffer, link: LocalSession) {
    if (buffer.byteLength < 29 || buffer.byteLength > 16384) throw new Error('FILE_FRAME_INVALID')
    const bytes = new Uint8Array(buffer), hex = Array.from(bytes.slice(0,16),v=>v.toString(16).padStart(2,'0')).join(''), id = localID(hex.replace(/^(.{8})(.{4})(.{4})(.{4})(.{12})$/,'$1-$2-$3-$4-$5')), version = new DataView(buffer).getUint32(16)
    await this.task(id, async () => {
      const job = await this.store.jobs.get(id), s = job?.local?.state
      if (!job || job.direction !== 'receive' || job.peer.root !== link.peer!.root || !s) throw new Error('FILE_CONSENT_REQUIRED')
      if (terminal(s.state) || s.paused || version < s.version) return
      if (s.state !== 'ACCEPTED' || version !== s.version || !job.local!.manifest) throw new Error('FILE_CONSENT_REQUIRED')
      if (Date.now() >= job.local!.expires) { await this.changeState(job,{state:'EXPIRED',version:s.version+1},link); return }
      const assembler = this.assemblers.get(id); if (!assembler || assembler.version !== version) throw new Error('FILE_FRAME_INVALID')
      const frame = assembler.assembler.push(buffer.slice(20)); if (!frame) return
      const decoded = await decodeFileFrame(frame,s.size), received = Math.min(s.size,(decoded.index+1)*wireChunkSize)
      if (decoded.index !== Math.floor(s.received/wireChunkSize) || decoded.bytes.length !== Math.min(wireChunkSize,s.size-decoded.index*wireChunkSize)) throw new Error('FILE_FRAME_INVALID')
      await this.changeState(job,{received,revision:s.revision+1},link,{index:decoded.index,bytes:decoded.bytes,hash:await digest(decoded.bytes)})
      if (received === s.size) this.verifyFile(id,link)
    })
  }
  private verifyFile(id: string, link?: LocalSession) {
    if (this.verifying.has(id)) return
    let verifyingVersion:number|undefined
    const work = (async () => {
      const job = (await this.store.jobs.get(id))!, s = job.local!.state!
      verifyingVersion=s.version
      const manifest = await openManifest(this.profile,job.local!.manifest!)
      const cipher = await this.store.receivedCipher(job); if (await hashBlob(cipher)!==s.root) throw new Error('FILE_CHECKSUM_INVALID')
      const plain = await decryptDare(cipher,manifest)
      if (job.payloadKind === 'TEXT') decodeText(new Uint8Array(await plain.arrayBuffer()))
      const receipt: LocalReceipt = await signed(this.profile,'receipt',{v:1 as const,id,sender:s.sender,recipient:s.recipient,root:s.root,plaintext_root:manifest.plaintext_root,plaintext_size:manifest.plaintext_size,delivered_at:new Date().toISOString()})
      await this.task(id,()=>this.controlLease(job.peer.root!,async () => {
        const current = await this.store.jobs.get(id), state = current?.local?.state
        if (!current || !state || state.version!==s.version || state.paused || state.state!=='ACCEPTED' || Date.now()>=current.local!.expires) return
        await this.changeState(current,{state:'COMPLETED',version:state.version+1,receipt},link,undefined,job.payloadKind === 'TEXT' ? undefined : plain,job.payloadKind === 'TEXT' ? { cipher, manifest: job.local!.manifest! } : undefined)
      }))
    })().catch(async error => {
      if(this.stopped)return
      const reason=error instanceof Error?error.message:'FILE_CHECKSUM_INVALID'
      if((reason.startsWith('FILE_')&&reason!=='FILE_JOB_CHANGED')||reason.startsWith('TEXT_')||error instanceof DOMException&&error.name==='OperationError')await this.task(id,async()=>{
        const current=await this.store.jobs.get(id),state=current?.local?.state
        if(!current||!state||state.version!==verifyingVersion||terminal(state.state))return
        await this.controlLease(current.peer.root!,()=>this.changeState(current,{state:'FAILED',version:state.version+1},link))
        await this.store.jobs.update(id,{error:reason})
      }).catch(()=>undefined)
      else await this.store.jobs.update(id,{error:reason})
      await this.changed()
    }).finally(()=>this.verifying.delete(id))
    this.verifying.set(id,work)
  }
  private async controlMessage(v: Record<string, unknown>, link: LocalSession) {
    if (v.type==='paired') return this.paired(link)
    if (v.type==='forget') { const pair='local:'+link.peer!.root; await this.store.localPeers.delete(pair); link.close(); await this.changed(); return }
    if (v.type==='offer') return this.offered(v,link)
    if (v.type==='state') return this.stateMessage(v.state as LocalState,link)
    if (v.type==='manifest') return this.manifest(v,link)
    if (v.type==='ready') return this.ready(v,link)
    if (v.type==='action') {
      if (typeof v.id!=='string' || !['pause','resume','cancel'].includes(String(v.action))) throw new Error('LOCAL_MESSAGE_INVALID')
      const job=await this.store.jobs.get(v.id)
      if (!job || job.direction!=='receive' || job.peer.root!==link.peer!.root) throw new Error('LOCAL_MESSAGE_INVALID')
      if (job.local!.state!.version!==v.expected || terminal(job.local!.state!.state)) { await link.send({type:'state',state:job.local!.state}); return }
      await this.action(job.id,v.action as 'pause'|'resume'|'cancel'); return
    }
    throw new Error('LOCAL_MESSAGE_INVALID')
  }
  private async heartbeat() {
    if (this.stopped) return
    for (const link of this.sessions) if (link.peer && this.releases.has(link)) {
      try { await this.store.transaction('rw',this.store.localLeases,async()=>{const row=await this.store.localLeases.get(link.peer!.root);if(row?.owner!==this.owner||row.session!==link.id)throw new Error('LOCAL_OTHER_TAB');await this.store.localLeases.update(row.id,{until:Date.now()+15000})}) } catch { link.close() }
    }
    const jobs=await this.store.jobs.filter(j=>active(j)&&j.local!.expires<=Date.now()).toArray()
    for(const job of jobs)if(job.direction==='receive')await this.task(job.id,()=>this.controlLease(job.peer.root!,async()=>{const fresh=(await this.store.jobs.get(job.id))!;if(!active(fresh)||Date.now()<fresh.local!.expires)return;await this.changeState(fresh,{state:'EXPIRED',version:fresh.local!.state!.version+1},this.links.get(job.peer.root!))})).catch(()=>undefined)
    else await this.store.transaction('rw',this.store.jobs,async()=>{this.life.signal.throwIfAborted();const current=await this.store.jobs.get(job.id);if(current&&active(current)&&current.local!.expires<=Date.now())await this.store.jobs.update(job.id,{phase:'stopped',error:'FILE_EXPIRED',prepared:undefined})})
    const detached=await this.store.jobs.filter(j=>active(j)&&j.direction==='receive'&&j.local!.expires>Date.now()&&j.local?.state?.state==='ACCEPTED'&&!j.local.state.paused&&!this.reachable(j.peer.root)).toArray()
    for(const job of detached){if(job.received===job.transfer!.size_bytes&&job.local!.manifest&&await this.store.chunks.get([job.id,0])){this.verifyFile(job.id);continue};await this.task(job.id,()=>this.controlLease(job.peer.root!,async()=>{const current=(await this.store.jobs.get(job.id))!;if(!active(current)||current.local!.state!.paused||this.reachable(current.peer.root))return;await this.changeState(current,{paused:true,version:current.local!.state!.version+1})})).catch(()=>undefined)}
  }
  async close() { if(this.stopped)return;const sessions=[...this.sessions];this.stopped=true;clearInterval(this.timer);this.life.abort();for(const s of sessions)s.close();for(const w of this.workers.values())w.controller.abort();await Promise.allSettled([...this.mutations,...this.verifying.values(),...Array.from(this.workers.values(),w=>w.task),...sessions.map(s=>s.drain()),...(this.pulse?[this.pulse]:[])]);this.listeners.clear() }
}
