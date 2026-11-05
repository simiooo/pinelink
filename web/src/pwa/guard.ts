import { guestWorkPending } from '../guest/continuity'
const guards = new Set<() => boolean>()
let frozen = false, starting = 0
export const updateFrozen = () => frozen
export function holdUpdate() { starting++; let released = false; return () => { if (!released) { released = true; starting-- } } }
export function registerUpdateGuard(ready: () => boolean) { guards.add(ready); return () => { guards.delete(ready) } }
export function safeToUpdate() {
  if (frozen || starting || guestWorkPending() || [...guards].some(ready => { try { return !ready() } catch { return true } })) return false
  if (typeof document === 'undefined') return true
  if (document.querySelector('main [aria-busy="true"], main .conversation-attachment, main .guest-send, main .guest-connect, main .guest-pair, main .guest-waiting')) return false
  return ![...document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('textarea, input:not([type]), input[type=text], input[type=email], input[type=password], input[type=file]')].some(input => !input.disabled && !input.readOnly && (input.type === 'file' ? !!(input as HTMLInputElement).files?.length : !!input.value))
}
export function freezeForUpdate() { if (!safeToUpdate()) return false; frozen = true; return true }
export function releaseUpdate() { frozen = false }
