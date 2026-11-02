import { canonical, decrypt, digest, proof, rootOf, unbase64, utf8, verify } from './crypto'
import type { DeviceProfile } from './crypto'
import type { Peer, PreparedFact } from './store'

export type Envelope = { version: number; event_id: string; source_root: string; stream_id: string; source_sequence: number; aggregate_id: string; aggregate_version: number; authority: string; authority_root: string; kind: string; peer_root?: string; recipients: string[]; signing_key: string; ciphertext?: string; server_payload?: Record<string, unknown>; signature: string }
export type Page = { consumer_root: string; epoch: string; after: number; through: number; upper: number; items: { sequence: number; event: Envelope }[]; more: boolean; snapshot?: boolean; next_key?: string; signature: string }
const uuid = (s: unknown) => typeof s === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(s)
const root = (s: unknown) => typeof s === 'string' && /^[a-f0-9]{64}$/.test(s)
const integer = (n: unknown, min = 0) => typeof n === 'number' && Number.isSafeInteger(n) && n >= min
const invalid = (): never => { throw new Error('INVALID_SIGNED_HISTORY') }
const states = new Set(['PREPARING', 'PENDING', 'ACCEPTED', 'PAUSED', 'VERIFYING', 'COMPLETED', 'REJECTED', 'CANCELLED', 'EXPIRED', 'FAILED'])

async function verifyFact(profile: DeviceProfile, e: Envelope, fact: Record<string, unknown>, peers: Map<string, Peer>) {
  if (fact.transfer_id !== e.aggregate_id || fact.state_version !== e.aggregate_version || fact.authority !== e.authority || fact.authority_device_id !== e.authority_root || fact.peer_id !== e.peer_root || !integer(fact.plaintext_size) || typeof fact.file_name !== 'string' || utf8.encode(fact.file_name).length > 512 || !states.has(String(fact.state))) invalid()
  if (fact.direction !== (e.kind === 'SENT_CHANGED' ? 'SEND' : 'RECEIVE')) invalid()
  if (fact.state !== 'COMPLETED') { if (fact.receipt != null) invalid(); return }
  const receipt = fact.receipt as Record<string, unknown> | undefined
  if (!receipt) invalid()
  const r = receipt!
  const signing = e.authority_root === profile.root ? profile.signingKey : peers.get(e.authority_root)?.signing_key
  const sender = e.kind === 'SENT_CHANGED' ? e.source_root : e.peer_root
  if (!signing || r.version !== 1 || r.transfer_id !== e.aggregate_id || r.recipient_id !== e.authority_root || r.sender_id !== sender || r.cipher_root !== fact.cipher_root || r.plaintext_size !== fact.plaintext_size || !root(r.cipher_root) || !root(r.plaintext_root) || typeof r.delivered_at !== 'string' || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(r.delivered_at)) invalid()
  const at = String(r.delivered_at).replace(/(\.\d*?)0+Z$/, '$1Z').replace('.Z', 'Z')
  const receiptProof = ['pinelink-receipt-v1', r.version, r.transfer_id, r.sender_id, r.recipient_id, r.cipher_root, r.plaintext_root, r.plaintext_size, at].join('\n')
  if (!await verify(signing!, utf8.encode(receiptProof), String(r.signature))) invalid()
}

export async function preparePage(profile: DeviceProfile, page: Page, pin: string, peers: Map<string, Peer>): Promise<PreparedFact[]> {
  if (page.consumer_root !== profile.root || !uuid(page.epoch) || !integer(page.after) || !integer(page.through, page.after) || !integer(page.upper, page.through) || !Array.isArray(page.items) || page.items.length > 100 || !await verify(pin, proof('page', page), page.signature)) invalid()
  if (page.more && ((!page.snapshot && page.through === page.after) || (page.snapshot && !page.next_key))) invalid()
  let last = page.after
  const rows: PreparedFact[] = []
  for (const item of page.items) {
    const e = item.event
    if (!integer(item.sequence, 1) || item.sequence > page.upper || (!page.snapshot && (item.sequence <= last || item.sequence > page.through))) invalid()
    last = item.sequence
    if (e.version !== 1 || !uuid(e.event_id) || !uuid(e.stream_id) || !uuid(e.aggregate_id) || !integer(e.source_sequence, 1) || !integer(e.aggregate_version) || !root(e.authority_root) || await rootOf(e.signing_key) !== e.source_root || !Array.isArray(e.recipients) || e.recipients.length < 1 || e.recipients.length > 2 || !e.recipients.includes(profile.root) || new Set(e.recipients).size !== e.recipients.length || canonical(e.recipients) !== canonical([...e.recipients].sort()) || !e.recipients.every(root) || !await verify(e.signing_key, proof('envelope', e), e.signature)) invalid()
    const row: PreparedFact = { id: [e.source_root, e.stream_id, e.event_id].join('|'), hash: await digest(utf8.encode(canonical(e))), event: e }
    if (e.authority === 'SERVER') {
      if (e.signing_key !== pin || e.authority_root !== e.source_root || !e.kind.startsWith('SERVER_') || e.ciphertext || !e.server_payload || utf8.encode(canonical(e.server_payload)).length > 65536) invalid()
      row.fact = e.server_payload
    } else {
      if (e.authority !== 'LOCAL_DEVICE' || !root(e.peer_root) || !e.recipients.includes(e.source_root) || e.recipients.some(r => r !== e.source_root && r !== e.peer_root) || !e.ciphertext || unbase64(e.ciphertext).length > 65536 || e.server_payload || !['SENT_CHANGED', 'TRANSFER_CHANGED'].includes(e.kind) || e.authority_root !== (e.kind === 'SENT_CHANGED' ? e.peer_root : e.source_root)) invalid()
      const key = e.source_root === profile.root ? profile.signingKey : peers.get(e.source_root)?.signing_key
      const peer = peers.get(e.source_root)
      if (key === e.signing_key && (e.source_root === profile.root || (peer && !peer.revoked))) {
        row.trustedKey = key
        row.fact = await decrypt(profile, e.ciphertext!)
        await verifyFact(profile, e, row.fact, peers)
      }
    }
    rows.push(row)
  }
  return rows
}
