import type { Table } from 'dexie'
import { LocalStore } from '../data/store'
import type { PreparedFile, GuestLease, GuestPeer, GuestTransfer, ProgressSnapshot } from './types'
import { digest } from '../data/crypto'
import { wireChunkSize } from './crypto'
import type { DirectProof } from './direct'
import type { LocalState } from '../local/protocol'
import type { StoredText } from './text-storage'

export type GuestJob = {
  id: string; created: number; direction: 'send' | 'receive'; peer: GuestPeer; transfer?: GuestTransfer
  prepared?: PreparedFile; received: number; snapshot?: ProgressSnapshot; plain?: Blob; name: string; plainSize: number
  phase: 'preparing' | 'waiting' | 'running' | 'paused' | 'verifying' | 'complete' | 'stopped'; error?: string; dismissed?: boolean; retryAt?: number; failures?: number; controlRevision?: number
  directAttempted?: boolean
  payloadKind?: 'TEXT'; text?: StoredText
  transport?: { state: 'connecting' | 'direct' | 'switching' | 'cloud'; proof?: DirectProof }
  authority?: 'LOCAL_DEVICE' | 'ACCOUNT'
  reconnectRequired?: boolean
  local?: { expires: number; state?: LocalState; manifest?: string; plainRoot?: string; intent?: 'pause' | 'cancel'; disconnected?: boolean }
}
export class GuestStore extends LocalStore {
  leases!: Table<GuestLease, string>
  jobs!: Table<GuestJob, string>
  chunks!: Table<{ job: string; index: number; bytes: Blob; hash: string }, [string, number]>
  guestPeers!: Table<GuestPeer, string>
  peerRemovals!: Table<{ pair: string; expires: number }, string>
  localPeers!: Table<GuestPeer, string>
  localUsed!: Table<{ id: string; expires: number }, string>
  localLeases!: Table<{ id: string; owner: string; until: number; session?:string }, string>
  localJournal!: Table<{ sequence?: number; event_id: string; authority: 'LOCAL_DEVICE'; authority_device_id: string; transfer: string; snapshot: LocalState; uploaded: number }, number>
  constructor(name: string) {
    super(name)
    this.version(3).stores({ leases: 'id', jobs: 'id,created,transfer.id,direction', chunks: '[job+index],job' })
    this.version(4).stores({ guestPeers: 'pair_id', peerRemovals: 'pair,expires' })
    this.version(5).stores({ localPeers: 'pair_id,root', localUsed: 'id,expires', localLeases: 'id,until', localJournal: '++sequence,&event_id,transfer,uploaded' })
  }
  async storeFrame(id: string, index: number, bytes: Uint8Array, signal?: AbortSignal) {
    const hash = await digest(bytes)
    return this.transaction('rw', this.jobs, this.chunks, async () => {
      signal?.throwIfAborted()
      const job = await this.jobs.get(id)
      if (!job?.transfer || job.direction !== 'receive') throw new Error('FILE_JOB_CHANGED')
      const old = await this.chunks.get([id, index])
      if (old && old.hash !== hash) throw new Error('FILE_FRAME_CHANGED')
      if (!old) await this.chunks.add({ job: id, index, bytes: new Blob([bytes]), hash })
      let next = Math.floor(job.received / wireChunkSize), received = job.received
      while (received < job.transfer.size_bytes && await this.chunks.get([id, next])) { received = Math.min(job.transfer.size_bytes, (next + 1) * wireChunkSize); next++ }
      if (received > job.received) await this.jobs.update(id, { received })
      return received
    })
  }
  async receivedCipher(job: GuestJob) {
    if (!job.transfer) throw new Error('FILE_JOB_CHANGED')
    const chunks = await this.chunks.where('job').equals(job.id).sortBy('index')
    const expected = Math.max(1, Math.ceil(job.transfer.size_bytes / wireChunkSize))
    if (chunks.length !== expected || chunks.some((part, index) => part.index !== index)) throw new Error('FILE_TRUNCATED')
    return new Blob(chunks.map(part => part.bytes))
  }
}
