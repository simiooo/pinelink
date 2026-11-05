import type { DeviceProfile } from '../data/crypto'
import { checkStoredKeys } from './connection-diagnostics'
import type { GuestStore } from './store'
import type { GuestLease, GuestPeer } from './types'

// The signed enrollment proves the current key, not the identity described by
// an unrelated cached lease. Preserve that cache and its history for recovery;
// it must never grant authority to replay another device's transfers.
export async function saveGuestLease(store: GuestStore, profile: DeviceProfile, next: GuestLease, signal: AbortSignal) {
  if (next.device_id !== profile.device || !next.owner_id?.startsWith('guest-') || !next.csrf || !Number.isFinite(Date.parse(next.expires_at))) throw new Error('GUEST_IDENTITY_CHANGED')
  const previous = await store.leases.get('self')
  const changed = !!previous && (previous.owner_id !== next.owner_id || previous.device_id !== next.device_id)
  if (changed) {
    // An owner change for an otherwise matching device, or a known different
    // signing key, is a genuine identity change and stays blocked.
    if (previous.device_id === profile.device || previous.profile_root && previous.profile_root !== profile.root) throw new Error('GUEST_IDENTITY_CHANGED')
    const keys = await checkStoredKeys(profile)
    if (keys.signing !== 'match' || keys.encryption !== 'match' || keys.root !== 'match') throw new Error('GUEST_IDENTITY_CHANGED')
  }
  const detached: string[] = []
  await store.transaction('rw', [store.profiles, store.leases, store.guestPeers, store.jobs], async () => {
    signal.throwIfAborted()
    const current = await store.profiles.get('self')
    if (!current || current.root !== profile.root || current.device !== profile.device || current.signingKey !== profile.signingKey || current.encryptionKey !== profile.encryptionKey) throw new Error('GUEST_IDENTITY_CHANGED')
    const latest = await store.leases.get('self')
    if (latest?.owner_id !== previous?.owner_id || latest?.device_id !== previous?.device_id) throw new Error('GUEST_IDENTITY_CHANGED')
    if (changed && previous) {
      await store.leases.put({ ...previous, id: `retired:${previous.owner_id}:${previous.device_id}` })
      await store.guestPeers.toCollection().modify(peer => { peer.reconnectRequired = true; peer.online = false })
      const jobs = await store.jobs.filter(job => job.authority !== 'LOCAL_DEVICE' && job.authority !== 'ACCOUNT').toArray()
      for (const stored of jobs) {
        const job = { ...stored, peer: { ...stored.peer, reconnectRequired: true, online: false } }
        const transfer = job.transfer
        const owner = job.direction === 'send' ? transfer?.sender_owner_id : transfer?.recipient_owner_id
        const device = job.direction === 'send' ? transfer?.sender_device_id : transfer?.recipient_device_id
        if (owner !== next.owner_id || device !== next.device_id) {
          job.reconnectRequired = true
          if (!['complete', 'stopped'].includes(job.phase)) { job.phase = 'stopped'; job.error = 'CONNECTION_REPAIR_REQUIRED'; detached.push(job.id) }
        }
        await store.jobs.put(job)
      }
    }
    signal.throwIfAborted()
    await store.leases.put({ ...next, profile_root: profile.root })
  })
  return detached
}

const samePeer = (a: GuestPeer, b: GuestPeer) => a.owner_id === b.owner_id && a.device_id === b.device_id && a.signing_key === b.signing_key && a.encryption_key === b.encryption_key

// Called inside the roster transaction. Reconnecting is confirmed by the
// server and by both peer keys, never by a display name or device type.
export async function reconcileGuestPeers(store: GuestStore, peers: GuestPeer[], now: number) {
  const cached = await store.guestPeers.toArray()
  await store.peerRemovals.where('expires').below(now).delete()
  const removed = new Set(await store.peerRemovals.toCollection().primaryKeys())
  const confirmed = peers.filter(peer => !removed.has(peer.pair_id)).map(peer => {
    const old = cached.find(value => value.pair_id === peer.pair_id && samePeer(value, peer)) ?? cached.find(value => value.reconnectRequired && !value.replacedBy && samePeer(value, peer))
    return { peer: { ...peer, local_name: old?.local_name }, old }
  })
  const retained = cached.filter(peer => peer.reconnectRequired && !removed.has(peer.pair_id) && !confirmed.some(value => value.peer.pair_id === peer.pair_id))
  for (const { peer, old } of confirmed) {
    if (old?.reconnectRequired && samePeer(old, peer)) {
      if (old.pair_id !== peer.pair_id) old.replacedBy = peer.pair_id
      const jobs = await store.jobs.filter(job => job.authority !== 'LOCAL_DEVICE' && job.authority !== 'ACCOUNT' && job.peer.pair_id === old.pair_id).toArray()
      if (jobs.length) await store.jobs.bulkPut(jobs.map(job => ({ ...job, peer })))
    }
  }
  await store.guestPeers.clear()
  if (confirmed.length || retained.length) await store.guestPeers.bulkPut([...retained, ...confirmed.map(value => value.peer)])
}
