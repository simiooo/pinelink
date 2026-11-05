import type { DeviceProfile } from '../data/crypto'
import { directDelay } from '../guest/direct'
import type { DirectProof } from '../guest/direct'
import { comparison, identity, makePackage, packageHash, signed, validatePackage, verified } from './protocol'
import type { ConnectionPackage, LocalIdentity, Signed, Rendezvous } from './protocol'

const label = 'pinelink-local-v1'
type Control = Signed<{ session: string; sequence: number; payload: Record<string, unknown> }>
export class LocalSession {
  readonly id: string
  readonly signal: AbortSignal
  readonly controller = new AbortController()
  private pc: RTCPeerConnection
  private channel?: RTCDataChannel
  private offer?: ConnectionPackage
  private answer?: ConnectionPackage
  private transcript = ''
  private sequence = 0
  private received = 0
  private queue = Promise.resolve()
  private sends = Promise.resolve()
  private timer?: ReturnType<typeof setTimeout>
  private disconnected?: ReturnType<typeof setTimeout>
  private opening?: Promise<void>
  private heartbeat?:ReturnType<typeof setInterval>
  private lastMessage=performance.now()
  private closed = false
  private closing?: Promise<void>
  private confirmed = false
  selfConfirmed = false
  peerConfirmed = false
  peer?: LocalIdentity
  code = ''
  package = ''
  error='LOCAL_CONNECTION_LOST'
  constructor(private profile: DeviceProfile, private name: string, lifetime: AbortSignal, readonly incoming: boolean,
    readonly events: { opened: (link: LocalSession) => Promise<void>; control: (payload: Record<string, unknown>, link: LocalSession) => Promise<void>; data: (bytes: ArrayBuffer, link: LocalSession) => Promise<void>; changed: () => void; closed: (link: LocalSession) => Promise<void> }, id: string = crypto.randomUUID()) {
    if (!globalThis.RTCPeerConnection) throw new Error('LOCAL_RTC_UNAVAILABLE')
    this.id = id; this.signal = AbortSignal.any([lifetime, this.controller.signal]); this.pc = new RTCPeerConnection({ iceServers: [] })
    this.pc.ondatachannel = e => { if (!incoming || this.channel) { e.channel.close(); this.close(); return }; this.attach(e.channel) }
    this.pc.onconnectionstatechange = () => { clearTimeout(this.disconnected); if (['closed', 'failed'].includes(this.pc.connectionState)) this.close(); else if (this.pc.connectionState === 'disconnected') this.disconnected = setTimeout(() => this.close(), 3000) }
    this.signal.addEventListener('abort', this.close, { once: true }); this.timer = setTimeout(this.close, 120000)
  }
  private attach(channel: RTCDataChannel) {
    if (channel.label !== label || channel.protocol !== label || !channel.ordered || channel.maxRetransmits !== null || channel.maxPacketLifeTime !== null) { channel.close(); this.close(); return }
    this.channel = channel; channel.binaryType = 'arraybuffer'; channel.bufferedAmountLowThreshold = 128 << 10
    channel.onerror = channel.onclose = this.close
    channel.onopen = () => { this.opening ??= this.opened().catch(error => {this.error=error instanceof Error?error.message:'LOCAL_CONNECTION_LOST';this.close()}) }
    channel.onmessage = e => {
      if (this.closed) return
      this.lastMessage=performance.now()
      this.queue = this.queue.then(async () => {
        this.signal.throwIfAborted(); await this.opening
        if (typeof e.data === 'string') {
          if (e.data.length > 32000) throw new Error('LOCAL_MESSAGE_INVALID')
          const m = JSON.parse(e.data) as Control
          if (!this.peer || m.session !== this.transcript || !Number.isSafeInteger(m.sequence) || m.sequence !== this.received + 1 || !m.payload || typeof m.payload !== 'object' || Array.isArray(m.payload) || !await verified(this.peer.signing_key, 'control', m)) throw new Error('LOCAL_MESSAGE_INVALID')
          this.received = m.sequence
          if (m.payload.type === 'confirm') { this.peerConfirmed = true; await this.finishConfirmation(); return }
          if (!this.confirmed) throw new Error('LOCAL_PAIR_REQUIRED')
          if(m.payload.type==='ping'){await this.send({type:'pong'});return}
          if(m.payload.type==='pong')return
          await this.events.control(m.payload, this)
        } else {
          if (!this.confirmed || !(e.data instanceof ArrayBuffer) || e.data.byteLength > 16384) throw new Error('LOCAL_PAIR_REQUIRED')
          await this.events.data(e.data, this)
        }
      }).catch(error => {this.error=error instanceof Error?error.message:'LOCAL_CONNECTION_LOST';this.close()})
    }
  }
  private async description(type: 'offer' | 'answer') {
    const signal = AbortSignal.any([this.signal, AbortSignal.timeout(12000)])
    await this.pc.setLocalDescription(type === 'offer' ? await this.pc.createOffer() : await this.pc.createAnswer())
    while (this.pc.iceGatheringState !== 'complete') await directDelay(25, signal)
    signal.throwIfAborted(); return this.pc.localDescription!.sdp
  }
  async createOffer(rendezvous?: Rendezvous) {
    this.attach(this.pc.createDataChannel(label, { ordered: true, protocol: label }))
    this.offer = await makePackage(this.profile, { ...identity(this.profile, this.name), capabilities: ['text-v1'] }, 'offer', this.id, await this.description('offer'), '', rendezvous); return this.offer
  }
  async createAnswer(offer: ConnectionPackage) {
    this.offer = await validatePackage(offer); this.peer = offer.peer
    await this.pc.setRemoteDescription({ type: 'offer', sdp: offer.sdp })
    this.answer = await makePackage(this.profile, { ...identity(this.profile, this.name), capabilities: ['text-v1'] }, 'answer', this.id, await this.description('answer'), await packageHash(offer)); return this.answer
  }
  async setAnswer(answer: ConnectionPackage) {
    await validatePackage(answer)
    if (!this.offer || answer.type !== 'answer' || answer.id !== this.id || answer.reply !== await packageHash(this.offer)) throw new Error('LOCAL_CODE_INVALID')
    this.peer = answer.peer; this.answer = answer
    await this.pc.setRemoteDescription({ type: 'answer', sdp: answer.sdp })
  }
  private async opened() {
    const deadline = AbortSignal.any([this.signal, AbortSignal.timeout(12000)])
    while (!this.offer || !this.answer) await directDelay(25, deadline)
    while (!await this.proof()) await directDelay(25, deadline)
    this.transcript = await packageHash(this.answer); this.code = await comparison(this.offer, this.answer)
    this.signal.throwIfAborted(); this.events.changed()
    await this.events.opened(this)
  }
  async confirm() { this.signal.throwIfAborted(); if (!this.code || !this.peer) throw new Error('LOCAL_CONNECTION_LOST'); if (!this.selfConfirmed) { await this.send({ type: 'confirm' }, true); this.selfConfirmed = true }; await this.finishConfirmation() }
  private async finishConfirmation() { this.signal.throwIfAborted(); if (this.selfConfirmed && this.peerConfirmed && !this.confirmed) { this.confirmed = true; clearTimeout(this.timer);this.lastMessage=performance.now();this.heartbeat=setInterval(()=>{if(performance.now()-this.lastMessage>12000){this.close();return};void this.send({type:'ping'}).catch(()=>this.close())},2000); await this.events.control({ type: 'paired' }, this) }; this.events.changed() }
  isReady() { return !this.closed && this.confirmed && this.channel?.readyState === 'open' }
  async send(payload: Record<string, unknown>, unconfirmed = false) {
    const task = this.sends.then(async () => {
      this.signal.throwIfAborted(); if ((!unconfirmed && !this.isReady()) || !this.transcript) throw new Error('LOCAL_CONNECTION_LOST')
      const m = await signed(this.profile, 'control', { session: this.transcript, sequence: this.sequence + 1, payload }), value = JSON.stringify(m)
      if (value.length > 32000) throw new Error('LOCAL_MESSAGE_INVALID')
      this.signal.throwIfAborted(); this.channel!.send(value); this.sequence++
    }); this.sends = task.catch(() => undefined); return task
  }
  async sendData(bytes: Uint8Array) {
    const deadline = AbortSignal.any([this.signal, AbortSignal.timeout(12000)])
    while (this.channel && this.channel.bufferedAmount > 256 << 10) await directDelay(15, deadline)
    this.signal.throwIfAborted(); if (!this.isReady()) throw new Error('LOCAL_CONNECTION_LOST'); this.channel!.send(bytes)
  }
  async proof(): Promise<DirectProof | undefined> {
    const report = await this.pc.getStats(); let pair: RTCStats | undefined, sent = 0, received = 0
    report.forEach(row => { if (row.type === 'transport' && row.selectedCandidatePairId) pair = report.get(row.selectedCandidatePairId); if (row.type === 'data-channel' && row.label === label) { sent += Number(row.bytesSent) || 0; received += Number(row.bytesReceived) || 0 } })
    if (!pair) report.forEach(row => { if (row.type === 'candidate-pair' && row.nominated && row.state === 'succeeded') pair = row })
    const selected = pair as (RTCStats & { localCandidateId?: string; remoteCandidateId?: string }) | undefined
    const local = selected?.localCandidateId && report.get(selected.localCandidateId), remote = selected?.remoteCandidateId && report.get(selected.remoteCandidateId)
    if (local && remote && ['host', 'prflx'].includes(local.candidateType) && ['host', 'prflx'].includes(remote.candidateType)) return { local: local.candidateType, remote: remote.candidateType, protocol: local.protocol, sent, received }
  }
  close = () => {
    if (this.closed) return; this.closed = true; this.controller.abort(new Error(this.error)); clearTimeout(this.timer); clearTimeout(this.disconnected);clearInterval(this.heartbeat)
    this.pc.ondatachannel = null; this.pc.onconnectionstatechange = null
    if (this.channel) { this.channel.onopen = null; this.channel.onmessage = null; this.channel.onclose = null; this.channel.onerror = null; this.channel.close() }
    this.pc.close(); this.closing=this.events.closed(this).catch(() => undefined)
  }
  async drain() { await Promise.allSettled([this.queue, this.sends, ...(this.opening ? [this.opening] : []),...(this.closing?[this.closing]:[])]) }
}
