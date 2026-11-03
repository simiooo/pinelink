import { useId, useLayoutEffect } from 'react'
import { Check, Cloud, Monitor, Smartphone } from 'lucide-react'
import { useMotionRef } from './react'
import { confirmedPercent, motion } from './tokens'

export function TransferGraph({ value, running, completed, cloud, incoming, localLabel, peerLabel, detailsLabel, progressLabel, statusLabel, onDetails }: {
  value: number; running: boolean; completed: boolean; cloud: boolean; incoming: boolean
  localLabel: string; peerLabel: string; detailsLabel: string; progressLabel: string; statusLabel: string; onDetails: () => void
}) {
  const [blue, owner] = useMotionRef<SVGPathElement>(), [mask] = useMotionRef<SVGPathElement>(), [sheen] = useMotionRef<SVGPathElement>()
  const maskId = useId().replaceAll(':', ''), fraction = Number.isFinite(value) ? Math.min(1, Math.max(0, value)) : 0
  const path = incoming ? 'M200 140C320 176 80 212 200 248' : 'M200 248C80 212 320 176 200 140'
  useLayoutEffect(() => {
    owner.style(blue.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? motion.quick : 0)
    owner.style(mask.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? motion.quick : 0)
  }, [fraction, running, owner, blue, mask])
  useLayoutEffect(() => {
    const element = sheen.current!
    if (running && fraction > 0 && fraction < 1) {
      element.style.opacity = '1'
      owner.loop(element, [{ strokeDashoffset: '100' }, { strokeDashoffset: '0' }], motion.sheen, { opacity: '0' })
    } else { owner.stopLoop(element); element.style.opacity = '0' }
    return () => owner.stopLoop(element)
  }, [running, fraction > 0 && fraction < 1, owner, sheen])
  useLayoutEffect(() => { const a = mask.current!, b = sheen.current!; return () => { owner.forget(a); owner.forget(b) } }, [owner, mask, sheen])
  return <div className="transfer-graph" data-running={running} aria-label={statusLabel}>
    <svg className="transfer-curve" viewBox="0 0 400 400" preserveAspectRatio="none" aria-hidden="true">
      <defs><mask id={maskId}><path ref={mask} d={path} fill="none" stroke="white" strokeWidth={8} pathLength={100} strokeDasharray="0 100" /></mask></defs>
      <path d={path} fill="none" stroke="#E5E5E5" strokeWidth={3} vectorEffect="non-scaling-stroke" />
      <path ref={blue} d={path} fill="none" stroke="#0866FF" strokeWidth={3} pathLength={100} strokeDasharray="0 100" vectorEffect="non-scaling-stroke" />
      <path ref={sheen} d={path} fill="none" stroke="#80B1FF" strokeWidth={3} pathLength={100} strokeDasharray="6 94" mask={`url(#${maskId})`} vectorEffect="non-scaling-stroke" />
    </svg>
    <div className="graph-peer"><span className="graph-node">{cloud ? <Cloud /> : <Smartphone />}</span><span className="node-label">{peerLabel}</span></div>
    <div className="graph-local"><span className="graph-node"><Monitor />{completed && incoming && <span className="graph-receipt"><Check size={14} /></span>}</span><span className="node-label">{localLabel}</span></div>
    <button type="button" className="curve-hit" aria-label={detailsLabel} onClick={onDetails} />
    <span className="sr-only" role="progressbar" aria-label={progressLabel} aria-valuemin={0} aria-valuemax={100} aria-valuenow={confirmedPercent(fraction)} aria-valuetext={`${confirmedPercent(fraction)}% · ${statusLabel}`} />
  </div>
}
