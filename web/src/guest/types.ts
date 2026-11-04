export type GuestLease = { id: string; owner_id: string; device_id: string; device_name: string; expires_at: string; csrf: string; offset: number }
export type GuestPeer = { pair_id: string; owner_id: string; device_id: string; device_name: string; encryption_key: string; signing_key: string; expires_at: string; online?: boolean; invite_id?: string; local_name?: string; authority?: 'LOCAL_DEVICE'; root?: string }
export type GuestInvite = { id: string; token?: string; code?: string; expires_at: string; consumed: boolean }
export type GuestCandidate = { id: string; invite_id: string; device_name: string; device_id: string; status: string; incoming: boolean; expires_at: string }
export type GuestConnections = { invites: GuestInvite[]; candidates: GuestCandidate[]; peers: GuestPeer[] }
export type GuestTransfer = { id: string; state: string; version: number; file_name: string; size_bytes: number; content_root: string; encryption_suite: string; encrypted_manifest?: string; sender_encrypted_manifest?: string; sender_owner_id: string; sender_device_id: string; recipient_owner_id: string; recipient_device_id: string; expires_at: string; received_bytes: number; created_at?: string }
export type ProgressSnapshot = {
  transfer_id: string; state: string; state_version: number; progress_revision: number; control_revision: number
  delivery: { state: string; acknowledged_bytes: number; total_bytes: number; revision: number }
  spool: { state: string; acknowledged_bytes: number; total_bytes: number }
  completion: { delivered: boolean; stored: boolean }
  capabilities: { pause: boolean; resume: boolean; cancel: boolean; restore: boolean }
  route?: { kind: string; epoch: number }
}
export type PreparedFile = { cipher: Blob; root: string; recipientManifest: string; senderManifest: string; name: string; plainSize: number }
