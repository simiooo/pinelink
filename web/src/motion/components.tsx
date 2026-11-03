import { useEffect, useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { ComponentProps, MouseEvent, ReactNode } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useEnter, useMotionRef } from './react'
import { motion } from './tokens'
import { Button } from '../components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../components/ui/tooltip'
import { Progress } from '../components/ui/progress'

type ButtonProps = Omit<ComponentProps<typeof Button>, 'onClick' | 'asChild'> & {
  onClick?: (event: MouseEvent<HTMLButtonElement>) => unknown
  busy?: boolean
  icon?: boolean
}
export function MotionButton({ children, className = 'secondary-button', disabled, busy = false, icon = false, onClick, ref: forwardedRef, variant, ...props }: ButtonProps) {
  const [face, owner] = useMotionRef<HTMLSpanElement>(), button = useRef<HTMLButtonElement>(null)
  const composedRef = useCallback((node: HTMLButtonElement | null) => {
    button.current = node
    if (typeof forwardedRef === 'function') {
      const cleanup = forwardedRef(node)
      return () => { button.current = null; if (typeof cleanup === 'function') cleanup(); else forwardedRef(null) }
    }
    if (forwardedRef) forwardedRef.current = node
    return () => { button.current = null; if (forwardedRef) forwardedRef.current = null }
  }, [forwardedRef])
  const [pending, setPending] = useState(false), [lockedWidth, setLockedWidth] = useState<number>()
  const gate = useRef(false), lifetime = useRef(0)
  useEffect(() => { lifetime.current++; return () => { lifetime.current++ } }, [])
  const pressed = (value: boolean) => {
    if (!face.current) return
    void owner.to(face.current, { scale: value && !disabled && !busy && !pending ? .97 : 1 }, value ? motion.press : motion.settle, !value)
  }
  useLayoutEffect(() => { if (disabled || busy || pending) pressed(false) }, [disabled, busy, pending])
  return <Button {...props} ref={composedRef} variant={variant ?? (className.includes('primary-button') ? 'default' : 'secondary')} size={icon ? 'icon' : 'default'} type={props.type ?? 'button'} className={`${className} motion-button${icon ? ' icon-button' : ''}`} disabled={disabled || busy || pending} aria-busy={busy || pending || undefined}
    style={{ ...props.style, width: lockedWidth ?? props.style?.width }}
    onPointerDown={event => { props.onPointerDown?.(event); if (event.button === 0) pressed(true) }}
    onPointerUp={event => { props.onPointerUp?.(event); pressed(false) }}
    onPointerCancel={event => { props.onPointerCancel?.(event); pressed(false) }}
    onPointerLeave={event => { props.onPointerLeave?.(event); pressed(false) }}
    onKeyDown={event => { props.onKeyDown?.(event); if (!event.repeat && [' ', 'Enter'].includes(event.key)) pressed(true) }}
    onKeyUp={event => { props.onKeyUp?.(event); pressed(false) }}
    onBlur={event => { props.onBlur?.(event); pressed(false) }}
    onClick={event => {
      if (gate.current || disabled || busy) return
      gate.current = true
      const epoch = lifetime.current
      try {
        const result = onClick?.(event)
        if (result && typeof (result as Promise<unknown>).then === 'function') {
          setLockedWidth(button.current?.getBoundingClientRect().width || undefined); setPending(true)
          void Promise.resolve(result).catch(() => undefined).finally(() => {
            gate.current = false
            if (lifetime.current === epoch) { setPending(false); setLockedWidth(undefined) }
          })
        } else gate.current = false
      } catch (error) { gate.current = false; throw error }
    }}>
    <span className="action-face" ref={face}>
      {(busy || pending) && <span className="action-busy"><MotionSpinner /></span>}
      <span className="action-label">{children}</span>
    </span>
  </Button>
}

export function MotionSpinner() {
  const [ref, owner] = useMotionRef<HTMLSpanElement>()
  useLayoutEffect(() => {
    const node = ref.current!
    owner.loop(node, [{ transform: 'rotate(0deg)' }, { transform: 'rotate(360deg)' }], 1200, { transform: 'none' })
    return () => owner.stopLoop(node)
  }, [owner, ref])
  return <span ref={ref} className="motion-spinner" aria-hidden="true"><LoaderCircle size={16} /></span>
}

export function MotionReveal({ children, identity, className = '', role, distance = 8 }: { children: ReactNode; identity?: unknown; className?: string; role?: 'status' | 'alert'; distance?: number }) {
  const [ref] = useMotionRef<HTMLDivElement>()
  useEnter(ref, identity ?? 'mounted', distance)
  return <div ref={ref} className={className} role={role}>{children}</div>
}

export function MotionTooltip({ label, children }: { label: string; children: ReactNode }) {
  return <TooltipProvider delayDuration={350}><Tooltip><TooltipTrigger asChild>{children}</TooltipTrigger>
    <TooltipContent sideOffset={8} className="pinelink-tooltip">{label}</TooltipContent>
  </Tooltip></TooltipProvider>
}

// Stable keys belong to data identity. Polling or a locale render must not replay entrances.
export function MotionList({ children, identity }: { children: ReactNode; identity: string }) {
  const [ref, owner] = useMotionRef<HTMLDivElement>(), before = useRef(new Map<string, DOMRect>())
  const tracked = useRef(new Map<string, HTMLElement>())
  useLayoutEffect(() => () => { for (const element of tracked.current.values()) owner.forget(element); tracked.current.clear(); before.current.clear() }, [owner])
  useLayoutEffect(() => {
    const nodes = Array.from(ref.current!.querySelectorAll<HTMLElement>(':scope > [data-motion-key]')).filter(node => !node.hidden)
    const next = new Map<string, DOMRect>()
    const measurements = nodes.map(node => {
      const key = node.dataset.motionKey!, box = node.getBoundingClientRect(), old = before.current.get(key), current = owner.current(node)
      const finalBox = new DOMRect(box.x - current.x, box.y - current.y, box.width, box.height)
      next.set(key, finalBox)
      return { node, key, old, current, finalBox }
    })
    const visibleNodes = new Set(nodes)
    for (const element of tracked.current.values()) if (!visibleNodes.has(element)) owner.forget(element)
    tracked.current = new Map(nodes.map(node => [node.dataset.motionKey!, node]))
    let newItems = 0
    for (const { node, old, current, finalBox } of measurements) {
      if (finalBox.bottom < 0 || finalBox.top > window.innerHeight) continue
      if (old && (old.left !== finalBox.left || old.top !== finalBox.top)) {
        owner.set(node, { x: old.left + current.x - finalBox.left, y: old.top + current.y - finalBox.top }); void owner.to(node, { x: 0, y: 0 }, 180)
      } else if (!old && newItems < 3) { owner.set(node, { opacity: .25, y: 8 }); void owner.to(node, { opacity: 1, y: 0 }, 180, false, newItems++ * 24) }
    }
    before.current = next
  }, [identity, owner, ref])
  return <div ref={ref} className="motion-list">{children}</div>
}

export function MotionMeter({ value, label }: { value: number; label: string }) {
  const [ref, owner] = useMotionRef<HTMLDivElement>(), bounded = Number.isFinite(value) ? Math.min(1, Math.max(0, value)) : 0
  useLayoutEffect(() => { owner.style(ref.current!, 'transform', `translateX(-${100 - bounded * 100}%)`) }, [owner, ref, bounded])
  return <Progress value={Math.round(bounded * 100)} indicatorRef={ref} aria-label={label} />
}

export function MotionStateIcon({ state, children, label }: { state: string; children: ReactNode; label: string }) {
  return <MotionTooltip label={label}><span className="motion-state-icon" role="img" aria-label={label} tabIndex={0}><MotionReveal identity={state} distance={0}>{children}</MotionReveal></span></MotionTooltip>
}
