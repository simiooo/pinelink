import { wireChunkSize } from './crypto'

export class DirectUnavailable extends Error { constructor() { super('DIRECT_UNAVAILABLE') } }
export type DirectProof = { local: string; remote: string; protocol: string; sent: number; received: number }
type Signal = { sequence: number; channel: string; payload: string; sender_role: string; reply_to?: number }
export type DirectRequest = <T>(method: string, path: string, input?: unknown, signal?: AbortSignal) => Promise<T>
type Ready = { type: 'ready'; transfer: string; root: string; from: number }
const label = 'pinelink-web-file-v1', protocol = 'pinelink-file-v1'
const failureChannel = (role: 'SENDER' | 'RECEIVER') => role === 'SENDER' ? 'web-direct-failed-sender' : 'web-direct-failed'
export const fallbackSignal = (signal: Pick<Signal, 'channel'>) => signal.channel === 'web-direct-failed' || signal.channel === 'web-direct-failed-sender'

export function directDelay(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal.aborted) { reject(signal.reason); return }
    const abort = () => { clearTimeout(timer); reject(signal.reason) }
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, ms)
    signal.addEventListener('abort', abort, { once: true })
  })
}

// Preserve the existing authenticated 64 KiB file frame, but fragment it below
// conservative SCTP message limits. At most one incomplete frame is retained.
export function fragmentFrame(frame: Uint8Array, limit = 16384) {
  if (limit < 256 || limit > 16384 || frame.byteLength < 36 || frame.byteLength > wireChunkSize + 36) throw new Error('FILE_FRAME_INVALID')
  const index = new DataView(frame.buffer, frame.byteOffset, frame.byteLength).getUint32(0), parts: Uint8Array<ArrayBuffer>[] = []
  for (let offset = 0; offset < frame.length; offset += limit - 8) {
    const bytes = frame.slice(offset, offset + limit - 8), packet = new Uint8Array(8 + bytes.length), view = new DataView(packet.buffer)
    view.setUint32(0, index); view.setUint32(4, offset); packet.set(bytes, 8); parts.push(packet)
  }
  return parts
}
export class FrameAssembler {
  private pending?: { index: number; bytes: Uint8Array<ArrayBuffer>; offset: number }
  constructor(private size: number, private next = 0) { if (!Number.isSafeInteger(size) || size < 0) throw new Error('FILE_FRAME_INVALID') }
  push(buffer: ArrayBuffer) {
    if (buffer.byteLength < 9 || buffer.byteLength > 16384) throw new Error('FILE_FRAME_INVALID')
    const view = new DataView(buffer), index = view.getUint32(0), offset = view.getUint32(4), bytes = new Uint8Array(buffer, 8)
    if (index !== this.next || index >= Math.max(1, Math.ceil(this.size / wireChunkSize))) throw new Error('FILE_FRAME_INVALID')
    const length = Math.min(wireChunkSize, this.size - index * wireChunkSize) + 36
    if (!this.pending) { if (offset !== 0) throw new Error('FILE_FRAME_INVALID'); this.pending = { index, bytes: new Uint8Array(length), offset: 0 } }
    const pending = this.pending
    if (pending.index !== index || pending.offset !== offset || offset + bytes.length > length) throw new Error('FILE_FRAME_INVALID')
    pending.bytes.set(bytes, offset); pending.offset += bytes.length
    if (pending.offset !== length) return undefined
    if (new DataView(pending.bytes.buffer).getUint32(0) !== index) throw new Error('FILE_FRAME_INVALID')
    this.pending = undefined; this.next++; return pending.bytes.buffer
  }
}

export class DirectLink {
  readonly signal: AbortSignal
  private pc: RTCPeerConnection
  private controller = new AbortController()
  private channel?: RTCDataChannel
  private ready?: Ready
  private acknowledged = 0
  private sent = 0
  private queue = Promise.resolve()
  private frameHandler?: (frame: ArrayBuffer, signal: AbortSignal) => Promise<boolean>
  private completed?: () => void
  private assembler: FrameAssembler
  private closing = false
  private disconnectTimer?: ReturnType<typeof setTimeout>
  private lastProof?: DirectProof
  private lastMessage = Date.now()
  private allFrames = false
  offerSequence = 0
  constructor(readonly role: 'SENDER' | 'RECEIVER', private transfer: string, private root: string, private size: number, private request: DirectRequest, lifetime: AbortSignal) {
    if (typeof RTCPeerConnection === 'undefined') throw new DirectUnavailable()
    this.signal = AbortSignal.any([lifetime, this.controller.signal]); this.assembler = new FrameAssembler(size)
    // Host ICE candidates try the LAN without depending on STUN/TURN or a
    // vendor endpoint. The durable encrypted cloud path is the fallback.
    try { this.pc = new RTCPeerConnection({ iceServers: [] }) } catch { throw new DirectUnavailable() }
    this.pc.onconnectionstatechange = () => {
      clearTimeout(this.disconnectTimer)
      if (['failed', 'closed'].includes(this.pc.connectionState)) this.fail(new DirectUnavailable())
      else if (this.pc.connectionState === 'disconnected') this.disconnectTimer = setTimeout(() => this.fail(new DirectUnavailable()), 1500)
    }
    this.pc.ondatachannel = event => { if (role !== 'RECEIVER' || this.channel) { event.channel.close(); this.fail(new DirectUnavailable()); return }; this.attach(event.channel) }
    this.signal.addEventListener('abort', this.shutdown, { once: true })
    if (this.signal.aborted) this.shutdown()
  }
  private shutdown = () => {
    clearTimeout(this.disconnectTimer); this.pc.onconnectionstatechange = null; this.pc.ondatachannel = null
    if (this.channel) { this.channel.onmessage = null; this.channel.onerror = null; this.channel.onclose = null; this.channel.close() }
    this.pc.close()
  }
  private fail(error: unknown) { if (!this.closing && !this.signal.aborted) this.controller.abort(error) }
  private check() { this.signal.throwIfAborted(); if (this.channel && ['closing', 'closed'].includes(this.channel.readyState)) throw new DirectUnavailable() }
  private attach(channel: RTCDataChannel) {
    if (channel.label !== label || channel.protocol !== protocol || !channel.ordered || channel.maxRetransmits !== null || channel.maxPacketLifeTime !== null) { channel.close(); this.fail(new DirectUnavailable()); return }
    this.channel = channel; channel.binaryType = 'arraybuffer'; channel.bufferedAmountLowThreshold = 128 << 10
    channel.onerror = channel.onclose = () => this.fail(new DirectUnavailable())
    channel.onmessage = event => {
      if (this.signal.aborted) return
      if (this.role === 'SENDER') {
        try {
          if (typeof event.data !== 'string' || event.data.length > 512) throw new Error('FILE_FRAME_INVALID')
          const value = JSON.parse(event.data)
          if (value?.type === 'ready' && !this.ready && value.transfer === this.transfer && value.root === this.root && Number.isSafeInteger(value.from) && value.from >= 0 && value.from <= this.size && value.from % wireChunkSize === 0) {
            this.ready = value; this.acknowledged = this.sent = value.from
          } else if (value?.type === 'ack' && this.ready && Number.isSafeInteger(value.received) && value.received >= this.acknowledged && value.received <= this.sent) this.acknowledged = value.received
          else throw new Error('FILE_FRAME_INVALID')
        } catch (error) { this.fail(error) }
      } else {
        // A paired connection is not per-file consent. The receiver only
        // installs this handler and sends Ready after Receive commits.
        if (!this.frameHandler || !(event.data instanceof ArrayBuffer)) { this.fail(new Error('FILE_CONSENT_REQUIRED')); return }
        this.lastMessage = Date.now()
        this.queue = this.queue.then(async () => {
          this.check(); const frame = this.assembler.push(event.data); if (!frame) return
          if ((new DataView(frame).getUint32(0) + 1) * wireChunkSize >= this.size) this.allFrames = true
          if (await this.frameHandler!(frame, this.signal)) this.completed?.()
        }).catch(error => this.fail(error))
      }
    }
  }
  async signals(signal = this.signal) {
    const result = await this.request<{ signals: Signal[] | null }>('GET', '/signals?after=0', undefined, signal)
    if (result.signals && (!Array.isArray(result.signals) || result.signals.length > 32)) throw new DirectUnavailable()
    return result.signals ?? []
  }
  async markFallback() { await this.request('POST', '/signals', { channel: failureChannel(this.role), role: this.role, payload: 'unavailable' }) }
  private async localDescription(type: 'offer' | 'answer', signal: AbortSignal) {
    try {
      await this.pc.setLocalDescription(type === 'offer' ? await this.pc.createOffer() : await this.pc.createAnswer())
      while (this.pc.iceGatheringState !== 'complete') { this.check(); await directDelay(20, signal) }
      const sdp = this.pc.localDescription?.sdp
      if (!sdp || sdp.length > 32 << 10) throw new DirectUnavailable()
      return sdp
    } catch (error) { if (signal.aborted) throw signal.reason; if (error instanceof DirectUnavailable) throw error; throw new DirectUnavailable() }
  }
  async connect(timeout = 8000) {
    const deadline = AbortSignal.any([this.signal, AbortSignal.timeout(timeout)])
    try {
      if (this.role === 'SENDER') {
        this.attach(this.pc.createDataChannel(label, { ordered: true, protocol }))
        const payload = await this.localDescription('offer', deadline)
        const sent = await this.request<Signal>('POST', '/signals', { channel: 'web-direct-offer', role: this.role, payload }, deadline)
        if (!Number.isSafeInteger(sent.sequence) || sent.sequence <= 0) throw new DirectUnavailable()
        this.offerSequence = sent.sequence
        while (true) {
          const signals = await this.signals(deadline); if (signals.some(fallbackSignal)) throw new DirectUnavailable()
          const answer = signals.find(s => s.channel === 'web-direct-answer' && s.sender_role === 'RECEIVER' && s.reply_to === sent.sequence)
          if (answer) { await this.pc.setRemoteDescription({ type: 'answer', sdp: answer.payload }); break }
          await directDelay(250, deadline)
        }
      } else {
        let offer: Signal | undefined
        while (!offer) {
          const signals = await this.signals(deadline); if (signals.some(fallbackSignal)) throw new DirectUnavailable()
          offer = signals.filter(s => s.channel === 'web-direct-offer' && s.sender_role === 'SENDER').at(-1)
          if (!offer) await directDelay(250, deadline)
        }
        this.offerSequence = offer.sequence
        await this.pc.setRemoteDescription({ type: 'offer', sdp: offer.payload })
        const payload = await this.localDescription('answer', deadline)
        await this.request('POST', '/signals', { channel: 'web-direct-answer', role: this.role, payload, reply_to: offer.sequence }, deadline)
      }
      while (true) {
        this.check(); deadline.throwIfAborted()
        if (this.channel?.readyState === 'open') { const proof = await this.proof(); if (proof) return proof }
        await directDelay(25, deadline)
      }
    } catch (error) {
      if (this.signal.aborted) throw this.signal.reason
      // Preserve HTTP authentication/state failures; these are not permission
      // to switch transport. Only RTC/timeout failures authorize cloud fallback.
      if (deadline.aborted || error instanceof DOMException || error instanceof TypeError) throw new DirectUnavailable()
      throw error
    }
  }
  async proof(): Promise<DirectProof | undefined> {
    if (this.signal.aborted) return this.lastProof
    let report: RTCStatsReport
    try { report = await this.pc.getStats() } catch { if (this.lastProof) return this.lastProof; throw new DirectUnavailable() }
    let pair: RTCStats | undefined, sent = 0, received = 0
    report.forEach(row => { if (row.type === 'transport' && row.selectedCandidatePairId) pair = report.get(row.selectedCandidatePairId); if (row.type === 'data-channel' && row.label === label) { sent += Number(row.bytesSent) || 0; received += Number(row.bytesReceived) || 0 } })
    if (!pair) report.forEach(row => { if (row.type === 'candidate-pair' && row.nominated && row.state === 'succeeded') pair = row })
    const selected = pair as (RTCStats & { localCandidateId?: string; remoteCandidateId?: string }) | undefined
    const local = selected?.localCandidateId && report.get(selected.localCandidateId), remote = selected?.remoteCandidateId && report.get(selected.remoteCandidateId)
    if (local && remote && ['host', 'prflx', 'srflx'].includes(local.candidateType) && ['host', 'prflx', 'srflx'].includes(remote.candidateType)) this.lastProof = { local: local.candidateType, remote: remote.candidateType, protocol: local.protocol, sent, received }
    return this.lastProof
  }
  async waitReady() { const until = Date.now() + 15000; while (!this.ready) { this.check(); if (Date.now() > until) throw new DirectUnavailable(); await directDelay(25, this.signal) }; return this.ready.from }
  async send(frame: Uint8Array, end: number) {
    const until = Date.now() + 8000
    while (this.sent - this.acknowledged >= 4 * wireChunkSize) { this.check(); if (Date.now() > until) throw new DirectUnavailable(); await directDelay(20, this.signal) }
    const limit = Math.min(16384, this.pc.sctp?.maxMessageSize || 16384)
    for (const packet of fragmentFrame(frame, limit)) {
      while (this.channel!.bufferedAmount > 256 << 10) { this.check(); if (Date.now() > until) throw new DirectUnavailable(); await directDelay(20, this.signal) }
      this.check(); this.channel!.send(packet.buffer)
    }
    this.sent = end
  }
  async receive(from: number, handler: (frame: ArrayBuffer, signal: AbortSignal) => Promise<boolean>) {
    this.check(); this.frameHandler = handler; this.assembler = new FrameAssembler(this.size, Math.floor(from / wireChunkSize))
    this.lastMessage = Date.now()
    const watchdog = setInterval(() => { if (!this.allFrames && Date.now() - this.lastMessage > 10000) this.fail(new DirectUnavailable()) }, 1000)
    let abort!: () => void
    try {
      await new Promise<void>((resolve, reject) => {
        abort = () => reject(this.signal.reason); this.signal.addEventListener('abort', abort, { once: true }); this.completed = resolve
        if (this.signal.aborted) { reject(this.signal.reason); return }
        this.channel!.send(JSON.stringify({ type: 'ready', transfer: this.transfer, root: this.root, from } satisfies Ready))
      })
    } finally { clearInterval(watchdog); this.signal.removeEventListener('abort', abort); this.completed = undefined }
  }
  acknowledge(received: number) { this.check(); this.channel!.send(JSON.stringify({ type: 'ack', received })) }
  async close() {
    this.closing = true; this.controller.abort(new DOMException('Direct connection closed', 'AbortError')); this.shutdown()
    await this.queue.catch(() => undefined)
  }
}
