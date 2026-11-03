import { useEffect, useId, useLayoutEffect, useState } from 'react'
import { Check, Cloud, FolderOpen, Laptop, Pencil, Plus, Send, Smartphone, Trash2, UserRound } from 'lucide-react'
import { MotionButton, MotionList, MotionReveal, MotionTooltip } from '../motion/components'
import { useMotionRef } from '../motion/react'
import { confirmedPercent } from '../motion/tokens'
import type { GuestPeer } from './types'
import type { GuestJob } from './store'
import { useGuestCopy } from './copy'

export function GuestBadge({ device }: { device?: string }) { const c = useGuestCopy(), label = c.guest + (device ? ` · ${c.device} ${device.slice(-4).toUpperCase()}` : ''); return <MotionTooltip label={label}><span className="guest-badge" tabIndex={0} aria-label={label}><UserRound size={11} /></span></MotionTooltip> }
export function DeviceGlyph({ name }: { name: string }) { return /iPhone|Android|phone/i.test(name) ? <Smartphone /> : <Laptop /> }
function Curve({ value, incoming, running, onDetails, ledger, lower }: { value: number; incoming: boolean; running: boolean; onDetails: () => void; ledger: 'spool' | 'delivery'; lower: boolean }) {
  const c = useGuestCopy(), id = useId().replaceAll(':', ''), [blue, owner] = useMotionRef<SVGPathElement>(), [mask] = useMotionRef<SVGPathElement>(), [sheen] = useMotionRef<SVGPathElement>()
  const fraction = Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : 0
  const path = lower ? incoming ? 'M200 270C290 310 290 420 200 456' : 'M200 456C290 420 290 310 200 270' : incoming ? 'M200 108C110 145 110 235 200 270' : 'M200 270C110 235 110 145 200 108'
  useLayoutEffect(() => { owner.style(blue.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? 180 : 0); owner.style(mask.current!, 'stroke-dasharray', `${fraction * 100} 100`, running ? 180 : 0) }, [fraction, running, blue, mask, owner])
  useLayoutEffect(() => {
    const element = sheen.current!
    if (running && fraction > 0 && fraction < 1) { element.style.opacity = '1'; owner.loop(element, [{ strokeDashoffset: '100' }, { strokeDashoffset: '0' }], 1800, { opacity: '0' }) }
    else { owner.stopLoop(element); element.style.opacity = '0' }
    return () => owner.stopLoop(element)
  }, [running, fraction > 0 && fraction < 1, sheen, owner])
  return <><svg className="guest-curve" viewBox="0 0 400 600" preserveAspectRatio="none" aria-hidden="true" data-blue={fraction} data-running={running} data-ledger={ledger}>
    <defs><mask id={id}><path ref={mask} d={path} fill="none" stroke="white" strokeWidth={10} pathLength={100} strokeDasharray="0 100" /></mask></defs>
    <path d={path} fill="none" stroke="#E5E5E5" strokeWidth={3} vectorEffect="non-scaling-stroke" />
    <path ref={blue} d={path} fill="none" stroke="#0866FF" strokeWidth={4} pathLength={100} strokeDasharray="0 100" vectorEffect="non-scaling-stroke" />
    <path ref={sheen} d={path} fill="none" stroke="#b8d2ff" strokeWidth={4} pathLength={100} strokeDasharray="8 92" mask={`url(#${id})`} vectorEffect="non-scaling-stroke" />
  </svg><button className="guest-curve-hit" data-segment={lower ? 'lower' : 'upper'} aria-label={c.details} onClick={onDetails} /><span className="sr-only" role="progressbar" aria-label={ledger === 'spool' ? c.upload : c.received} aria-valuemin={0} aria-valuemax={100} aria-valuenow={confirmedPercent(fraction)} /></>
}
export function GuestGraph({ peers, job, onConnect, onPeer, onDetails, dragging, onFileDrop, localDevice, onRename, onRemove, onFiles }: { peers: GuestPeer[]; job?: GuestJob; onConnect: () => void; onPeer: (peer: GuestPeer) => void; onDetails: () => void; dragging: boolean; onFileDrop: (file: File, peer: GuestPeer) => void; localDevice: string; onRename?: (peer: GuestPeer) => void; onRemove?: (peer: GuestPeer) => void; onFiles?: (peer?: GuestPeer) => void }) {
  const c = useGuestCopy(), mobile = /Android|iPhone|iPad/.test(navigator.userAgent)
  const [dropTarget, setDropTarget] = useState<string>()
  const [selected, setSelected] = useState<string>()
  useEffect(() => { if (job) setSelected(undefined) }, [job?.id])
  // GuestRuntime currently selects the durable relay. Keep both endpoint
  // devices visible; cloud storage and recipient delivery are separate legs.
  const relay = !!job?.transfer, stored = !!job?.snapshot?.completion.stored
  const upload = job?.phase === 'complete' ? 1 : (job?.snapshot?.spool.acknowledged_bytes ?? 0) / (job?.snapshot?.spool.total_bytes || job?.transfer?.size_bytes || 1)
  const delivery = job?.phase === 'complete' ? 1 : (job?.snapshot?.delivery.acknowledged_bytes ?? 0) / (job?.snapshot?.delivery.total_bytes || job?.transfer?.size_bytes || 1)
  const uploadRunning = !job?.error && !stored && !['complete', 'stopped'].includes(job?.phase ?? '') && ['OPEN', 'SPOOLING', 'VERIFYING'].includes(job?.snapshot?.spool.state ?? '')
  const deliveryRunning = !job?.error && job?.phase === 'running' && job.snapshot?.delivery.state === 'RUNNING'
  const shown = job ? [{ ...job.peer, ...peers.find(peer => peer.pair_id === job.peer.pair_id), online: peers.find(peer => peer.pair_id === job.peer.pair_id)?.online ?? false }] : peers
  const choosing = !job && shown.some(peer => peer.pair_id === selected)
  return <div className="guest-stage" data-dragging={dragging || undefined}>
    <div className="guest-network" data-route={relay ? 'relay' : 'idle'} data-selected={choosing || undefined} onKeyDown={event => { if (event.key === 'Escape' && choosing) { event.preventDefault(); const node = event.currentTarget.querySelector<HTMLButtonElement>('.guest-node[aria-expanded=true]'); setSelected(undefined); node?.focus({ preventScroll: true }) } }} onClick={event => { if (event.target === event.currentTarget) setSelected(undefined) }}>
      {relay && <><Curve value={upload} incoming={job.direction === 'receive'} running={uploadRunning} onDetails={onDetails} ledger="spool" lower={job.direction === 'send'} /><Curve value={delivery} incoming={job.direction === 'receive'} running={deliveryRunning} onDetails={onDetails} ledger="delivery" lower={job.direction === 'receive'} /></>}
      {shown.length ? <MotionList className="guest-peers" count={shown.length} identity={(job ? 'relay:' : 'idle:') + shown.map(peer => peer.pair_id).join(':')}>{shown.map(peer => <div className="guest-peer" key={peer.pair_id} data-motion-key={peer.pair_id} data-online={peer.online} data-selected={choosing && peer.pair_id === selected || undefined} data-drop-target={dragging && dropTarget === peer.pair_id || undefined}
        onDragOver={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); setDropTarget(peer.pair_id) } }}
        onDragLeave={event => { if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) setDropTarget(undefined) }}
        onDrop={event => { if (event.dataTransfer.files.length === 1) { event.preventDefault(); event.stopPropagation(); setDropTarget(undefined); onFileDrop(event.dataTransfer.files[0], peer) } }}>
        <MotionTooltip label={job ? c.details : peer.local_name || peer.device_name}><MotionButton className="guest-node" aria-label={job ? c.details : peer.local_name || peer.device_name} aria-expanded={!job ? choosing && peer.pair_id === selected : undefined} onClick={() => job ? onDetails() : setSelected(value => value === peer.pair_id ? undefined : peer.pair_id)} onKeyDown={event => { if (event.key === 'Escape') setSelected(undefined) }}><DeviceGlyph name={peer.device_name} /></MotionButton></MotionTooltip>
        <GuestBadge device={peer.device_id} /><span className="guest-dot" /><span className="guest-node-label">{peer.local_name || peer.device_name}</span>
        {choosing && peer.pair_id === selected && <div className="guest-node-actions" role="group" aria-label={c.deviceActions}><MotionReveal identity={peer.pair_id} distance={4}>
          {([{ label: c.send, icon: Send, run: () => onPeer(peer) }, { label: c.files, icon: FolderOpen, run: () => onFiles?.(peer) }, { label: c.rename, icon: Pencil, run: () => onRename?.(peer) }, { label: c.remove, icon: Trash2, run: () => onRemove?.(peer) }]).map(action => <MotionTooltip key={action.label} label={action.label}><MotionButton icon className="guest-icon-button" aria-label={action.label} onClick={action.run}><action.icon size={20} /></MotionButton></MotionTooltip>)}
        </MotionReveal></div>}
      </div>)}</MotionList> : <div className="guest-connect-node"><MotionTooltip label={c.connect}><MotionButton className="guest-node guest-add" aria-label={c.connect} onClick={onConnect}><Plus /></MotionButton></MotionTooltip><span className="guest-node-label">{c.connect}</span></div>}
      {relay && <div className="guest-cloud"><MotionReveal className="guest-cloud-content" identity={job.transfer!.id} distance={4}><MotionTooltip label={stored ? c.stored : c.upload}><MotionButton className="guest-node" aria-label={c.cloud + ' · ' + c.details} onClick={onDetails}><Cloud /></MotionButton></MotionTooltip>{stored && <span className="guest-cloud-stored" aria-label={c.stored}><Check size={11} /></span>}<span className="guest-node-label">{c.cloud}</span></MotionReveal></div>}
      <div className="guest-local"><MotionTooltip label={job ? c.details : c.local}><MotionButton className="guest-node" aria-label={job ? c.details : c.local} onClick={job ? onDetails : () => onFiles?.()}>{mobile ? <Smartphone /> : <Laptop />}</MotionButton></MotionTooltip>
        <GuestBadge device={localDevice} /><span className="guest-dot">{job?.phase === 'complete' && job.direction === 'receive' ? <Check size={13} /> : null}</span><span className="guest-node-label">{c.local}</span>
      </div>
    </div>
  </div>
}
