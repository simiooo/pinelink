import { useId, useLayoutEffect, useState } from 'react'
import { Check, Cloud, Laptop, Plus, Smartphone, UserRound } from 'lucide-react'
import { MotionButton, MotionTooltip } from '../motion/components'
import { useMotionRef } from '../motion/react'
import type { GuestPeer } from './types'
import type { GuestJob } from './store'
import { useGuestCopy } from './copy'

export function GuestBadge({ device }: { device?: string }) { const c = useGuestCopy(), label = c.guest + (device ? ` · ${c.device} ${device.slice(-4).toUpperCase()}` : ''); return <MotionTooltip label={label}><span className="guest-badge" tabIndex={0} aria-label={label}><UserRound size={11} /></span></MotionTooltip> }
export function DeviceGlyph({ name }: { name: string }) { return /iPhone|Android|phone/i.test(name) ? <Smartphone /> : <Laptop /> }
function Curve({ value, incoming, running, onDetails }: { value: number; incoming: boolean; running: boolean; onDetails: () => void }) {
  const c = useGuestCopy(), id = useId().replaceAll(':', ''), [blue, owner] = useMotionRef<SVGPathElement>(), [mask] = useMotionRef<SVGPathElement>(), [sheen] = useMotionRef<SVGPathElement>()
  const fraction = Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : 0
  const path = incoming ? 'M200 108C300 160 300 350 200 456' : 'M200 456C300 350 300 160 200 108'
  useLayoutEffect(() => { owner.style(blue.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? 180 : 0); owner.style(mask.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? 180 : 0) }, [fraction, running, blue, mask, owner])
  useLayoutEffect(() => {
    const element = sheen.current!
    if (running && fraction > 0 && fraction < 1) { element.style.opacity = '1'; owner.loop(element, [{ strokeDashoffset: '100' }, { strokeDashoffset: '0' }], 1800, { opacity: '0' }) }
    else { owner.stopLoop(element); element.style.opacity = '0' }
    return () => owner.stopLoop(element)
  }, [running, fraction > 0 && fraction < 1, sheen, owner])
  return <><svg className="guest-curve" viewBox="0 0 400 600" preserveAspectRatio="none" aria-hidden="true" data-blue={fraction} data-running={running}>
    <defs><mask id={id}><path ref={mask} d={path} fill="none" stroke="white" strokeWidth={10} pathLength={100} strokeDasharray="0 100" /></mask></defs>
    <path d={path} fill="none" stroke="#E5E5E5" strokeWidth={3} vectorEffect="non-scaling-stroke" />
    <path ref={blue} d={path} fill="none" stroke="#0866FF" strokeWidth={4} pathLength={100} strokeDasharray="0 100" vectorEffect="non-scaling-stroke" />
    <path ref={sheen} d={path} fill="none" stroke="#b8d2ff" strokeWidth={4} pathLength={100} strokeDasharray="8 92" mask={`url(#${id})`} vectorEffect="non-scaling-stroke" />
  </svg><button className="guest-curve-hit" aria-label={c.details} onClick={onDetails} /><span className="sr-only" role="progressbar" aria-label={c.received} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(fraction * 100)} /></>
}
export function GuestGraph({ peers, job, onConnect, onPeer, onDetails, dragging, onFileDrop, localDevice }: { peers: GuestPeer[]; job?: GuestJob; onConnect: () => void; onPeer: (peer: GuestPeer) => void; onDetails: () => void; dragging: boolean; onFileDrop: (file: File, peer: GuestPeer) => void; localDevice: string }) {
  const c = useGuestCopy(), mobile = /Android|iPhone|iPad/.test(navigator.userAgent)
  const [dropTarget, setDropTarget] = useState<string>()
  const cloud = job?.direction === 'send' && !!job.transfer && !job.snapshot?.completion.stored && job.phase !== 'complete' && job.phase !== 'stopped'
  const value = cloud ? (job?.snapshot?.spool.acknowledged_bytes ?? 0) / (job?.snapshot?.spool.total_bytes || job?.transfer?.size_bytes || 1) : (job?.snapshot?.delivery.acknowledged_bytes ?? 0) / (job?.snapshot?.delivery.total_bytes || job?.transfer?.size_bytes || 1)
  const shown = job ? [{ ...job.peer, online: peers.find(peer => peer.pair_id === job.peer.pair_id)?.online ?? false }] : peers.slice(0, 2)
  return <div className="guest-stage" data-dragging={dragging || undefined}>
    <div className="guest-network">
      {job && <Curve value={job.phase === 'complete' ? 1 : value} incoming={job.direction === 'receive'} running={!job.error && (job.phase === 'running' || cloud)} onDetails={onDetails} />}
      {shown.length ? <div className="guest-peers" data-count={shown.length}>{shown.map(peer => <div className="guest-peer" key={peer.pair_id} data-online={cloud || peer.online} data-drop-target={dragging && dropTarget === peer.pair_id || undefined}
        onDragOver={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); setDropTarget(peer.pair_id) } }}
        onDragLeave={event => { if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) setDropTarget(undefined) }}
        onDrop={event => { if (event.dataTransfer.files.length === 1) { event.preventDefault(); event.stopPropagation(); setDropTarget(undefined); onFileDrop(event.dataTransfer.files[0], peer) } }}>
        <MotionTooltip label={job ? c.details : peer.device_name}><MotionButton className="guest-node" aria-label={job ? c.details : peer.device_name} onClick={() => job ? onDetails() : onPeer(peer)}>{cloud ? <Cloud /> : <DeviceGlyph name={peer.device_name} />}</MotionButton></MotionTooltip>
        {!cloud && <GuestBadge device={peer.device_id} />}<span className="guest-dot" /><span className="guest-node-label">{cloud ? c.cloud : peer.device_name}</span>
      </div>)}</div> : <div className="guest-connect-node"><MotionTooltip label={c.connect}><MotionButton className="guest-node guest-add" aria-label={c.connect} onClick={onConnect}><Plus /></MotionButton></MotionTooltip><span className="guest-node-label">{c.connect}</span></div>}
      <div className="guest-local"><MotionTooltip label={job ? c.details : c.local}><MotionButton className="guest-node" aria-label={job ? c.details : c.local} onClick={job ? onDetails : onConnect}>{mobile ? <Smartphone /> : <Laptop />}</MotionButton></MotionTooltip>
        <GuestBadge device={localDevice} /><span className="guest-dot">{job?.phase === 'complete' && job.direction === 'receive' ? <Check size={13} /> : null}</span><span className="guest-node-label">{c.local}</span>
      </div>
    </div>
  </div>
}
