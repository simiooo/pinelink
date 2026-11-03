import { createContext, useContext, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react'
import type { ReactNode, RefObject } from 'react'
import { MotionOwner } from './owner'

const MotionContext = createContext<MotionOwner | null>(null)
export function MotionProvider({ children }: { children: ReactNode }) {
  const [owner] = useState(() => new MotionOwner())
  useLayoutEffect(() => owner.attach(), [owner])
  return <MotionContext.Provider value={owner}>{children}</MotionContext.Provider>
}
export function useMotion() {
  const shared = useContext(MotionContext)
  const [owner] = useState(() => shared ?? new MotionOwner())
  useLayoutEffect(() => shared ? undefined : owner.attach(), [shared, owner])
  return shared ?? owner
}
export function useReducedMotion() {
  const owner = useMotion()
  return useSyncExternalStore(owner.subscribe, owner.snapshot, () => true)
}
const subscribeOnline = (listener: () => void) => {
  window.addEventListener('online', listener); window.addEventListener('offline', listener)
  return () => { window.removeEventListener('online', listener); window.removeEventListener('offline', listener) }
}
export function useOnline() { return useSyncExternalStore(subscribeOnline, () => navigator.onLine !== false, () => false) }
export function useMotionRef<T extends HTMLElement | SVGElement>() {
  const ref = useRef<T>(null), owner = useMotion()
  useLayoutEffect(() => { const element = ref.current; return () => { if (element) owner.forget(element) } }, [owner])
  return [ref, owner] as const
}
export function useEnter(ref: RefObject<HTMLElement | null>, identity: unknown, distance = 8, duration = 160) {
  const owner = useMotion(), previous = useRef<unknown>(Symbol())
  useLayoutEffect(() => {
    if (identity === previous.current || !ref.current) return
    previous.current = identity
    if (!owner.isAnimating(ref.current)) owner.set(ref.current, { opacity: .25, y: distance })
    void owner.to(ref.current, { opacity: 1, y: 0 }, duration)
    return () => { previous.current = Symbol() }
  }, [identity, ref, owner, distance, duration])
}
