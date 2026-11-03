// Account broadcasts must not replace a visitor's running identity or route.
let pending = 0
let historyAvailable = false, revision = 0
const listeners = new Set<() => void>()
const changed = () => { revision++; for (const listener of listeners) listener() }
export const guestContinuityRevision = () => revision
export const subscribeGuestContinuity = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
export const guestHistoryAvailable = () => historyAvailable
export const setGuestHistoryAvailable = (value: boolean) => { if (value !== historyAvailable) { historyAvailable = value; changed() } }
export const guestWorkPending = () => pending > 0
export const setGuestWorkPending = (count: number) => { if (count !== pending) { pending = count; changed() } }
export function openGuestSignIn() {
  if (!guestWorkPending()) { location.assign('/login'); return }
  const popup = window.open('/login', 'pinelink-account', 'popup,width=480,height=760')
  if (!popup) location.assign('/login') // The persisted visitor job resumes on return.
}
