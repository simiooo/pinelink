import { useRef, useSyncExternalStore } from 'react'
import { authRevision, authState, subscribeAuth } from '../auth/client'
import { guestContinuityRevision, guestWorkPending, subscribeGuestContinuity } from './continuity'
export function useVisitorMode() {
  useSyncExternalStore(subscribeAuth, authRevision); useSyncExternalStore(subscribeGuestContinuity, guestContinuityRevision)
  const state = authState(), guestView = useRef(false)
  if (state.capabilities.guests && ((!state.session && !state.offlineAccount) || guestWorkPending())) guestView.current = true
  return !!state.capabilities.guests && guestView.current
}
