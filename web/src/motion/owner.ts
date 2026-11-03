import { motion } from './tokens'

type Element = HTMLElement | SVGElement
export type Pose = { x: number; y: number; scale: number; opacity: number; rotate: number }
type Record = { element: Element; from: Pose; to: Pose; velocity: Partial<Pose>; duration: number; delay: number; spring: boolean; animation?: Animation }
type StyleRecord = { value: string; animation?: Animation }
type Loop = { element: Element; frames: Keyframe[]; duration: number; resting: Partial<CSSStyleDeclaration>; active: Partial<CSSStyleDeclaration>; visible: boolean; animation?: Animation }
const rest: Pose = { x: 0, y: 0, scale: 1, opacity: 1, rotate: 0 }
const keys = Object.keys(rest) as (keyof Pose)[]
const css = (p: Pose) => ({ transform: `translate(${p.x}px, ${p.y}px) scale(${p.scale}) rotate(${p.rotate}deg)`, opacity: String(p.opacity) })
const ease = (p: number) => 1 - (1 - p) ** 3

// The analytic solution preserves velocity when a gesture or a new target interrupts.
export function springSample(from: number, to: number, velocity: number, seconds: number) {
  const damping = motion.spring.damping / 2
  const frequency = Math.sqrt(motion.spring.stiffness - damping * damping)
  const a = from - to, b = (velocity + damping * a) / frequency
  const sin = Math.sin(frequency * seconds), cos = Math.cos(frequency * seconds), decay = Math.exp(-damping * seconds)
  return { value: to + decay * (a * cos + b * sin), velocity: decay * ((b * frequency - damping * a) * cos - (a * frequency + damping * b) * sin) }
}

export class MotionOwner {
  private records = new Map<Element, Record>()
  private styles = new Map<Element, Map<string, StyleRecord>>()
  private loops = new Map<Element, Loop>()
  private animations = new Set<Animation>()
  private observer?: IntersectionObserver
  private preferences?: MediaQueryList
  private subscriptions = new Set<() => void>()
  private attached = 0
  reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
  visible = typeof document === 'undefined' || !document.hidden

  subscribe = (listener: () => void) => { this.subscriptions.add(listener); return () => { this.subscriptions.delete(listener) } }
  snapshot = () => this.reduced
  attach() {
    if (++this.attached > 1) return () => this.detach()
    if (typeof matchMedia === 'function') {
      this.preferences = matchMedia('(prefers-reduced-motion: reduce)')
      this.setReduced(this.preferences.matches)
      this.preferences.addEventListener('change', this.preferenceChanged)
    }
    document.addEventListener('visibilitychange', this.visibilityChanged)
    this.visibilityChanged()
    if (typeof IntersectionObserver === 'function') {
      this.observer = new IntersectionObserver(entries => {
        for (const entry of entries) { const loop = this.loops.get(entry.target as Element); if (loop) loop.visible = entry.isIntersecting }
        this.syncLoops()
      })
      for (const element of this.loops.keys()) this.observer.observe(element)
    }
    return () => this.detach()
  }
  private preferenceChanged = (event: MediaQueryListEvent) => this.setReduced(event.matches)
  private visibilityChanged = () => { this.visible = !document.hidden; this.syncLoops() }
  private detach() {
    if (--this.attached > 0) return
    this.preferences?.removeEventListener('change', this.preferenceChanged)
    document.removeEventListener('visibilitychange', this.visibilityChanged)
    this.observer?.disconnect(); this.observer = undefined
    this.clear()
  }
  setReduced(value: boolean) {
    this.reduced = value
    if (value) {
      for (const rec of this.records.values()) { rec.animation?.cancel(); rec.animation = undefined; Object.assign(rec.element.style, css(rec.to)) }
      for (const [element, props] of this.styles) for (const [property, rec] of props) { rec.animation?.cancel(); rec.animation = undefined; element.style.setProperty(property, rec.value) }
      this.animations.clear()
    }
    this.syncLoops()
    for (const listener of this.subscriptions) listener()
  }
  private sample(rec: Record) {
    if (!rec.duration || !rec.animation) return { value: { ...rec.to }, velocity: {} }
    const elapsed = Math.max(0, typeof rec.animation?.currentTime === 'number' ? rec.animation.currentTime - rec.delay : rec.duration)
    const progress = Math.min(1, Math.max(0, elapsed / rec.duration)), value = { ...rec.to }, velocity: Partial<Pose> = {}
    for (const key of keys) {
      if (rec.spring && key !== 'opacity' && progress < 1) {
        const current = springSample(rec.from[key], rec.to[key], rec.velocity[key] ?? 0, elapsed / 1000)
        value[key] = current.value; velocity[key] = current.velocity
      } else value[key] = rec.from[key] + (rec.to[key] - rec.from[key]) * ease(progress)
    }
    return { value, velocity }
  }
  set(element: Element, pose: Partial<Pose>) {
    const old = this.records.get(element), current = old ? this.sample(old).value : rest
    if (old?.animation) { this.animations.delete(old.animation); old.animation.cancel() }
    const value = { ...current, ...pose }
    Object.assign(element.style, css(value))
    this.records.set(element, { element, from: value, to: value, velocity: {}, duration: 0, delay: 0, spring: false })
  }
  to(element: Element, pose: Partial<Pose>, duration: number = motion.control, spring = false, delay = 0) {
    const old = this.records.get(element), sampled = old ? this.sample(old) : { value: { ...rest }, velocity: {} }
    const target = { ...sampled.value, ...pose }
    if (old?.animation) { this.animations.delete(old.animation); old.animation.cancel() }
    if (this.reduced || duration === 0 || typeof element.animate !== 'function') { this.set(element, target); return Promise.resolve() }
    const rec: Record = { element, from: sampled.value, to: target, velocity: sampled.velocity, duration, delay, spring }
    const frames = Array.from({ length: 37 }, (_, index) => {
      const t = index / 36, value = { ...target }
      for (const key of keys) value[key] = spring && key !== 'opacity' ? springSample(rec.from[key], target[key], rec.velocity[key] ?? 0, t * duration / 1000).value : rec.from[key] + (target[key] - rec.from[key]) * ease(t)
      return { ...css(index === 36 ? target : value), offset: t }
    })
    const animation = element.animate(frames, { duration, delay, easing: 'linear', fill: 'both' })
    rec.animation = animation; this.records.set(element, rec); this.animations.add(animation)
    return animation.finished.then(() => {
      if (this.records.get(element) === rec) { Object.assign(element.style, css(target)); rec.animation = undefined }
      this.animations.delete(animation); animation.cancel()
    }).catch(() => { this.animations.delete(animation) })
  }
  style(element: Element, property: string, value: string, duration: number = motion.quick) {
    let props = this.styles.get(element)
    if (!props) { props = new Map(); this.styles.set(element, props) }
    const old = props.get(property), from = old?.animation ? getComputedStyle(element).getPropertyValue(property) : old?.value ?? getComputedStyle(element).getPropertyValue(property)
    if (old?.animation) { this.animations.delete(old.animation); old.animation.cancel() }
    element.style.setProperty(property, value)
    const rec: StyleRecord = { value }; props.set(property, rec)
    if (this.reduced || !duration || from === value || typeof element.animate !== 'function') return
    const animationProperty = property.replace(/-([a-z])/g, (_, letter: string) => letter.toUpperCase())
    const animation = element.animate([{ [animationProperty]: from }, { [animationProperty]: value }], { duration, easing: motion.state, fill: 'both' })
    rec.animation = animation; this.animations.add(animation)
    void animation.finished.then(() => {
      if (props?.get(property) === rec) { element.style.setProperty(property, value); rec.animation = undefined }
      this.animations.delete(animation); animation.cancel()
    }).catch(() => this.animations.delete(animation))
  }
  loop(element: Element, frames: Keyframe[], duration: number, resting: Partial<CSSStyleDeclaration> = {}) {
    if (this.loops.has(element)) return
    const active: Partial<CSSStyleDeclaration> = {}
    for (const key of Object.keys(resting) as (keyof CSSStyleDeclaration)[]) Object.assign(active, { [key]: element.style[key] })
    const box = element.getBoundingClientRect()
    this.loops.set(element, { element, frames, duration, resting, active, visible: box.bottom >= 0 && box.top <= window.innerHeight, animation: undefined })
    this.observer?.observe(element); this.syncLoops()
  }
  stopLoop(element: Element) {
    const loop = this.loops.get(element)
    if (loop) { loop.animation?.cancel(); Object.assign(element.style, loop.resting); this.observer?.unobserve(element); this.loops.delete(element) }
  }
  private syncLoops() {
    for (const loop of this.loops.values()) {
      if (this.reduced || typeof loop.element.animate !== 'function') { loop.animation?.cancel(); loop.animation = undefined; Object.assign(loop.element.style, loop.resting); continue }
      Object.assign(loop.element.style, loop.active)
      loop.animation ??= loop.element.animate(loop.frames, { duration: loop.duration, iterations: Infinity, easing: 'linear' })
      if (this.visible && loop.visible) loop.animation.play(); else loop.animation.pause()
    }
  }
  forget(root: Element) {
    for (const [element, rec] of this.records) if (element === root || root.contains(element)) { if (rec.animation) { rec.animation.cancel(); this.animations.delete(rec.animation) }; this.records.delete(element) }
    for (const [element, props] of this.styles) if (element === root || root.contains(element)) { for (const rec of props.values()) if (rec.animation) { rec.animation.cancel(); this.animations.delete(rec.animation) }; this.styles.delete(element) }
    for (const element of this.loops.keys()) if (element === root || root.contains(element)) this.stopLoop(element)
  }
  clear() {
    for (const rec of this.records.values()) Object.assign(rec.element.style, css(rec.to))
    for (const animation of this.animations) animation.cancel()
    for (const element of this.loops.keys()) this.stopLoop(element)
    this.animations.clear(); this.records.clear(); this.styles.clear()
  }
  current(element: Element) { const record = this.records.get(element); return record ? this.sample(record).value : { ...rest } }
  isAnimating(element: Element) { return this.records.get(element)?.animation?.playState === 'running' }
  diagnostics() { return { animations: this.animations.size, poses: this.records.size, styles: this.styles.size, loops: this.loops.size, reduced: this.reduced, attached: this.attached } }
}
