import { clientRuntime } from '../data'
import { DataError } from '../data/runtime'

export const API_URL = import.meta.env.VITE_PINELINK_API_URL ?? 'http://127.0.0.1:8080'
export const IDENTITY = {
  owner: import.meta.env.VITE_PINELINK_OWNER ?? 'dev-owner',
  device: import.meta.env.VITE_PINELINK_DEVICE ?? 'dev-device',
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, public resource?: string) {
    super(code)
  }
}

export type Task = {
  id: string
  kind: 'UPLOAD' | 'DOWNLOAD'
  reference_id: string
  state: string
  progress_bytes: number
  total_bytes: number
  version: number
  error_code?: string
  error_message?: string
  updated_at: string
}

export type Transfer = {
  id: string
  entry_id?: string
  state: string
  version: number
  delivery_mode: string
  sender_owner_id: string
  sender_device_id: string
  recipient_owner_id: string
  recipient_device_id: string
  file_name?: string
  size_bytes?: number
  created_at: string
  expires_at: string
  failure_reason?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const runtime = clientRuntime()
  if (runtime) {
    try { return await runtime.request<T>(init?.method ?? 'GET', path, init?.body ? JSON.parse(String(init.body)) : undefined) }
    catch (error) { if (error instanceof DataError) throw new ApiError(error.status, error.code); throw error }
  }
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { code?: string; resource?: string } | null
    throw new ApiError(response.status, body?.code ?? 'REQUEST_FAILED', body?.resource)
  }
  return response.json() as Promise<T>
}

export async function getTasks(owner = IDENTITY.owner, device = IDENTITY.device, includeDeleted = false) {
  const result = await request<{ tasks: Task[] | null }>(`/v1/tasks?owner_id=${encodeURIComponent(owner)}&device_id=${encodeURIComponent(device)}&include_deleted=${includeDeleted}`)
  return { tasks: result.tasks ?? [] }
}

export async function getPendingNotifications(owner = IDENTITY.owner, device = IDENTITY.device) {
  const result = await request<{ transfers: Transfer[] | null }>(`/v1/notifications?owner_id=${encodeURIComponent(owner)}&device_id=${encodeURIComponent(device)}&state=PENDING`)
  return { transfers: result.transfers ?? [] }
}

export function getTransfers() {
  return request<{ transfers: Transfer[] | null }>(`/v1/transfers?owner_id=${encodeURIComponent(IDENTITY.owner)}&device_id=${encodeURIComponent(IDENTITY.device)}`).then(result => result.transfers ?? [])
}

export type Pull = { id: string; task_id: string; transfer_id?: string; file_name: string; size_bytes: number }

export function getPull(id: string) {
  return request<Pull>(`/v1/pulls/${encodeURIComponent(id)}?owner_id=${encodeURIComponent(IDENTITY.owner)}&device_id=${encodeURIComponent(IDENTITY.device)}`)
}

export function controlTask(id: string, action: 'pause' | 'resume' | 'cancel' | 'delete', version: number) {
  return request<Task>(`/v1/tasks/${id}/${action}`, {
    method: 'POST',
    body: JSON.stringify({ owner_id: IDENTITY.owner, device_id: IDENTITY.device, expected_version: version }),
  })
}

export type Device = { owner_id: string; device_id: string; status: string; last_seen_at: string }
export type Quota = { scope_id: string; quota_bytes: number; used_bytes: number }
export type FileSnapshot = { id: string; file_name: string; state: string; version: number; retention_class: string; expires_at?: string }

export async function getDevices() {
  const result = await request<{ devices: Device[] | null }>(`/v1/devices?owner_id=${encodeURIComponent(IDENTITY.owner)}`)
  return result.devices ?? []
}

export function getQuota() {
  const scope = import.meta.env.VITE_PINELINK_SCOPE ?? 'dev-scope'
  return request<Quota>(`/v1/quota?scope_id=${encodeURIComponent(scope)}`)
}

export function getFile(id: string) {
  return request<FileSnapshot>(`/v1/files/${encodeURIComponent(id)}?owner_id=${encodeURIComponent(IDENTITY.owner)}`)
}

export function promoteFile(id: string, version: number) {
  return request<FileSnapshot>(`/v1/files/${id}/promote`, { method: 'POST', body: JSON.stringify({ owner_id: IDENTITY.owner, device_id: IDENTITY.device, expected_version: version }) })
}

export function revokeDevice(id: string) {
  return request<Device>('/v1/devices/revoke', { method: 'POST', body: JSON.stringify({ owner_id: IDENTITY.owner, device_id: id }) })
}

export function respondTransfer(id: string, action: 'accept' | 'reject', owner = 'dev-owner', device = 'dev-device', version = 1) {
  return request<Transfer>(`/v1/transfers/${id}/${action}`, {
    method: 'POST',
    body: JSON.stringify({ owner_id: owner, device_id: device, expected_version: version }),
  })
}
