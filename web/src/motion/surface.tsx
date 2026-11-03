import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Dialog, DialogContent, DialogTitle } from '../components/ui/dialog'
import { SheetContent } from '../components/ui/sheet'
import { MotionButton } from './components'
import { useMotionRef, useReducedMotion } from './react'

export function MotionSurface({ open, onClose, title, children, closeLabel }: { open: boolean; onClose: () => void; title: string; children: ReactNode; closeLabel: string }) {
  const [present, setPresent] = useState(open), [panel, owner] = useMotionRef<HTMLDivElement>()
  const [mobile, setMobile] = useState(() => typeof matchMedia === 'function' && matchMedia('(max-width: 639px)').matches)
  const reduced = useReducedMotion(), seen = useRef(new WeakSet<HTMLElement>())
  const trigger = useRef<HTMLElement | null>(null), wasOpen = useRef(false)
  const latestOpen = useRef(open)
  latestOpen.current = open
  const gesture = useRef<{ id: number; y: number; started: number; offset: number } | null>(null)
  useEffect(() => {
    if (typeof matchMedia !== 'function') return
    const media = matchMedia('(max-width: 639px)'), update = () => setMobile(media.matches)
    media.addEventListener('change', update); return () => media.removeEventListener('change', update)
  }, [])
  useLayoutEffect(() => {
    if (open && !wasOpen.current) {
      const focused = document.activeElement as HTMLElement
      if (!trigger.current?.isConnected || !present) trigger.current = focused
      setPresent(true)
    }
    wasOpen.current = open
  }, [open, present, panel])
  useLayoutEffect(() => {
    const element = panel.current
    if (!present || !element) return
    if (open) {
      if (!seen.current.has(element)) { seen.current.add(element); owner.set(element, { opacity: 0, y: mobile ? 24 : 8, scale: mobile ? 1 : .98 }) }
      void owner.to(element, { opacity: 1, y: 0, scale: 1 }, mobile ? 220 : 160)
    } else {
      void owner.to(element, { opacity: 0, y: mobile ? 24 : 4, scale: mobile ? 1 : .98 }, mobile ? 160 : 120)
      // Bounded teardown also runs when the browser suppresses animation events.
      const timer = setTimeout(() => setPresent(false), reduced ? 0 : mobile ? 160 : 120)
      return () => clearTimeout(timer)
    }
  }, [open, present, owner, reduced, mobile, panel])
  useLayoutEffect(() => { const element = panel.current; return () => { if (element) owner.forget(element); gesture.current = null } }, [present, mobile, owner, panel])
  const common = {
    ref: panel, forceMount: true as const, showCloseButton: false, 'aria-describedby': undefined,
    className: 'pinelink-motion-surface', 'aria-hidden': !open || undefined, inert: !open,
    onCloseAutoFocus: (event: Event) => {
      if (latestOpen.current || document.querySelector('[role="dialog"][data-state="open"]')) { event.preventDefault(); return }
      if (trigger.current?.isConnected) { event.preventDefault(); trigger.current.focus({ preventScroll: true }) }
    },
  }
  const content = <>
    {mobile && <div className="sheet-drag-target" onPointerDown={event => {
      if (event.button !== 0) return
      gesture.current = { id: event.pointerId, y: event.clientY, started: event.timeStamp, offset: 0 }; event.currentTarget.setPointerCapture(event.pointerId)
    }} onPointerMove={event => {
      const current = gesture.current
      if (!current || current.id !== event.pointerId || !panel.current) return
      current.offset = Math.min(160, Math.max(0, event.clientY - current.y)); owner.set(panel.current, { y: current.offset })
    }} onPointerUp={event => {
      const current = gesture.current
      if (!current || !panel.current) return
      gesture.current = null
      const velocity = current.offset / Math.max(1, event.timeStamp - current.started) * 1000
      if (current.offset > 64 || (current.offset > 20 && velocity > 650)) onClose()
      else void owner.to(panel.current, { y: 0 }, 360, true)
    }} onPointerCancel={() => { gesture.current = null; if (panel.current) void owner.to(panel.current, { y: 0 }, 360, true) }}><span /></div>}
    <header className="surface-heading"><DialogTitle>{title}</DialogTitle><MotionButton icon aria-label={closeLabel} onClick={onClose}><X size={22} /></MotionButton></header>
    {children}
  </>
  return <Dialog open={open} onOpenChange={value => { if (!value) onClose() }}>
    {present && (mobile ? <SheetContent {...common} side="bottom">{content}</SheetContent> : <DialogContent {...common}>{content}</DialogContent>)}
  </Dialog>
}
