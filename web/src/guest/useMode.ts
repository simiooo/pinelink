import { useSyncExternalStore } from 'react'
import { authRevision, authState, subscribeAuth } from '../auth/client'
import { guestContinuityRevision, guestWorkPending, subscribeGuestContinuity } from './continuity'
export function useVisitorMode() { useSyncExternalStore(subscribeAuth, authRevision); useSyncExternalStore(subscribeGuestContinuity, guestContinuityRevision); const state = authState(); return !!state.capabilities.guests && ((!state.session && !state.offlineAccount) || guestWorkPending()) }
