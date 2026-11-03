import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { ArrowDownToLine, Check, Clock3, FilePlus2, FileText, History, Pause, Play, Plus, RefreshCw, X } from 'lucide-react'
import { authState } from '../auth/client'
import QRCode from 'qrcode'
import { MotionButton, MotionReveal, MotionTooltip } from '../motion/components'
import { MotionSurface } from '../motion/surface'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { Input } from '../components/ui/input'
import { useGuestCopy, guestErrorText } from './copy'
import { DeviceGlyph, GuestGraph } from './Graph'
import { getGuestRuntime } from './runtime'
import type { GuestRuntime } from './runtime'
import type { GuestJob } from './store'
import type { GuestPeer } from './types'
import { useI18n } from '../i18n'
import { formatBytes } from '../format'
import './guest.css'

const linkKey = 'pinelink:guest-link'
export function captureGuestLink() {
  if (location.pathname !== '/connect') return
  const token = location.hash.slice(1)
  if (/^[A-Za-z0-9_-]{22}$/.test(token)) { try { sessionStorage.setItem(linkKey, JSON.stringify({ token, expires: Date.now() + 600000 })) } catch { /* The current page still provides manual code entry. */ } }
  history.replaceState(null, '', '/connect')
}
const pendingLink = () => { try { const v = JSON.parse(sessionStorage.getItem(linkKey) ?? 'null') as { token: string; expires: number } | null; return v && v.expires > Date.now() ? v.token : '' } catch { return '' } }

export function GuestHome() {
  const c = useGuestCopy(), [runtime, setRuntime] = useState<GuestRuntime>(), [error, setError] = useState(false), [retry, setRetry] = useState(0)
  useEffect(() => { let live = true; void getGuestRuntime().then(value => { if (live) setRuntime(value) }).catch(() => { if (live) setError(true) }); return () => { live = false } }, [retry])
  if (!runtime) return error ? <section className="guest-storage"><h1>{c.storage}</h1><p>{c.storageHint}</p><MotionButton className="primary-button" onClick={() => { setError(false); setRetry(value => value + 1) }}>{c.retry}</MotionButton></section> : <div className="guest-loading" aria-busy="true" />
  return <VisitorHome runtime={runtime} />
}
function VisitorHome({ runtime }: { runtime: GuestRuntime }) {
  const c = useGuestCopy(), { locale } = useI18n(), state = useSyncExternalStore(runtime.subscribe, runtime.getSnapshot)
  const [panel, setPanel] = useState<'connect' | 'pair' | 'waiting' | 'send' | 'job' | 'history' | null>(location.pathname === '/connect' ? 'connect' : ['/history', '/local-files'].includes(location.pathname) ? 'history' : null)
  const lastPanel = useRef(panel)
  if (panel) lastPanel.current = panel
  const visualPanel = panel ?? lastPanel.current
  const [tab, setTab] = useState(pendingLink() ? 'enter' : 'show'), [code, setCode] = useState(''), [token, setToken] = useState(pendingLink)
  const [error, setError] = useState<string>(), [copied, setCopied] = useState(false), [qr, setQr] = useState(''), [dragging, setDragging] = useState(false), [now, setNow] = useState(Date.now())
  const [file, setFile] = useState<File>(), [selectedPeer, setSelectedPeer] = useState<GuestPeer>(), [jobId, setJobId] = useState<string>()
  const [candidateId, setCandidateId] = useState<string>(), [joining, setJoining] = useState<{ id: string; invite: string; expires: number }>(), seen = useRef(new Set<string>(state.jobs.filter(job => job.phase === 'complete').map(job => 'done:' + job.id))), input = useRef<HTMLInputElement>(null), dragDepth = useRef(0)
  const active = state.jobs.find(job => !job.dismissed && !['complete', 'stopped'].includes(job.phase)) ?? (state.jobs[0] && !state.jobs[0].dismissed ? state.jobs[0] : undefined)
  const selectedJob = state.jobs.find(job => job.id === jobId) ?? active
  const candidate = state.connections.candidates.find(item => item.id === candidateId)
  const maxBytes = authState().capabilities.guest_max_file_bytes ?? (64 << 20)
  const peers = state.offline || now - (state.checkedAt ?? 0) > 15000 ? state.connections.peers.map(peer => ({ ...peer, online: false })) : state.connections.peers
  const errorText = guestErrorText(error ?? state.error, c), invite = state.invite
  const expired = !!invite && (Date.parse(invite.expires_at) <= now || invite.consumed)
  const inviteURL = invite?.token ? new URL('/connect#' + invite.token, location.origin).href : ''
  const perform = async (task: () => Promise<unknown>) => { setError(undefined); try { await task() } catch (e) { setError(e instanceof Error ? e.message : 'GUEST_UNAVAILABLE') } }
  useEffect(() => { const timer = setInterval(() => setNow(runtime.now()), 1000); return () => clearInterval(timer) }, [runtime])
  useEffect(() => {
    let live = true; setQr('')
    if (inviteURL && !expired) void QRCode.toDataURL(inviteURL, { width: 216, margin: 4, errorCorrectionLevel: 'M', color: { dark: '#101010', light: '#FFFFFF' } }).then(url => { if (live) setQr(url) }).catch(() => undefined)
    return () => { live = false }
  }, [inviteURL, expired])
  useEffect(() => {
    const incoming = state.connections.candidates.find(item => item.incoming && item.status === 'WAITING' && !seen.current.has(item.id))
    if (incoming) { seen.current.add(incoming.id); setCandidateId(incoming.id); setPanel('pair'); return }
    const pending = state.jobs.find(job => job.direction === 'receive' && job.phase === 'waiting' && job.transfer?.state === 'PENDING' && Date.parse(job.transfer.expires_at) > now && !seen.current.has(job.id))
    if (pending) { seen.current.add(pending.id); setJobId(pending.id); setPanel('job'); return }
    const completed = state.jobs.find(job => job.phase === 'complete' && !seen.current.has('done:' + job.id))
    if (completed) { seen.current.add('done:' + completed.id); setJobId(completed.id); setPanel('job') }
    if (panel === 'waiting' && joining) {
      if (state.connections.peers.some(peer => peer.invite_id === joining.invite)) setPanel(null)
      else if (state.connections.candidates.find(item => item.id === joining.id)?.status === 'DECLINED') { setError('CONNECTION_DECLINED'); setTab('enter'); setPanel('connect') }
      else if (joining.expires <= now) { setError('EXPIRED'); setTab('enter'); setPanel('connect') }
    }
    if (panel === 'connect' && invite?.consumed && state.connections.peers.length) setPanel(null)
  }, [state.connections, state.jobs, panel, invite?.consumed, joining, now])
  const connect = () => { setPanel('connect'); setTab('show'); void perform(() => runtime.connect()) }
  const openJob = (job?: GuestJob) => { if (job) { setJobId(job.id); setPanel('job') } }
  const choose = (chosen?: File, peer?: GuestPeer) => {
    if (!chosen) { if (peer) setSelectedPeer(peer); input.current?.click(); return }
    setFile(chosen); setSelectedPeer(peer ?? selectedPeer ?? state.connections.peers[0])
    if (!state.connections.peers.length) connect(); else setPanel('send')
  }
  const title = visualPanel === 'connect' ? c.connect : visualPanel === 'pair' ? c.confirmPair : visualPanel === 'waiting' ? c.waiting : visualPanel === 'history' ? (location.pathname === '/local-files' ? c.localFiles : c.history) : visualPanel === 'send' ? selectedPeer?.device_name ?? c.selectDevice : selectedJob?.peer.device_name ?? c.details
  const jobStatus = (job: GuestJob) => job.phase === 'complete' ? job.direction === 'receive' && job.plain ? c.saved : c.complete : job.phase === 'paused' ? c.paused : job.phase === 'verifying' ? c.verifying : job.phase === 'stopped' ? (job.error === 'FILE_EXPIRED' || job.snapshot?.state === 'EXPIRED' ? c.fileExpired : c.stopped) : job.phase === 'preparing' ? c.preparing : job.transfer?.state === 'PENDING' ? c.waitingFile : job.error ? c.unavailable : c.running
  return <section className="guest-page" data-guest-job={active?.transfer?.id} data-phase={active?.phase}
    onDragEnter={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); dragDepth.current++; setDragging(true) } }}
    onDragOver={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); event.dataTransfer.dropEffect = 'copy' } }}
    onDragLeave={() => { if (--dragDepth.current <= 0) { dragDepth.current = 0; setDragging(false) } }}
    onDrop={event => { event.preventDefault(); dragDepth.current = 0; setDragging(false); if (event.dataTransfer.files.length === 1) choose(event.dataTransfer.files[0]); else setError('ONE_FILE_ONLY') }}>
    <input ref={input} type="file" hidden aria-label={c.file} onChange={event => { choose(event.target.files?.[0]); event.target.value = '' }} />
    <GuestGraph peers={peers} job={active} onConnect={connect} onPeer={peer => { setSelectedPeer(peer); if (file) setPanel('send'); else choose(undefined, peer) }} onDetails={() => openJob(active)} dragging={dragging} localDevice={runtime.profile.device} onFileDrop={(file, peer) => { dragDepth.current = 0; setDragging(false); choose(file, peer) }} />
    {dragging && <div className="guest-drop-label" role="status">{c.file}</div>}
    <footer className="guest-footer"><MotionTooltip label={c.history}><MotionButton icon className="guest-icon-button" aria-label={c.history} onClick={() => setPanel('history')}><History size={23} /></MotionButton></MotionTooltip>
      {state.offline && <span className="guest-offline" role="status">{c.offline}</span>}
      {state.connections.peers.length > 0 && !active && <MotionTooltip label={c.connect}><MotionButton icon className="guest-icon-button" aria-label={c.connect} onClick={connect}><Plus size={24} /></MotionButton></MotionTooltip>}
      <MotionTooltip label={c.file}><MotionButton icon className="guest-icon-button" aria-label={c.file} onClick={() => choose()} disabled={state.busy}><FilePlus2 size={23} /></MotionButton></MotionTooltip>
    </footer>
    {errorText && !panel && <MotionReveal role="alert" className="guest-error"><span>{errorText}</span><MotionButton icon className="guest-icon-button" aria-label={c.retry} onClick={() => active ? perform(() => runtime.retry(active.id)) : perform(() => runtime.ensureSession().then(() => runtime.poll()))}><RefreshCw size={18} /></MotionButton></MotionReveal>}
    <MotionSurface open={!!panel} onClose={() => { setPanel(null); setError(undefined) }} title={title} closeLabel={c.close}>
      <div className="guest-surface-body">
        {visualPanel === 'connect' && <div className="guest-connect">
          <Tabs value={tab} onValueChange={value => { setTab(value); setError(undefined); if (value === 'show') void perform(() => runtime.connect()) }}>
            <TabsList variant="line" className="guest-tabs"><TabsTrigger value="show">{c.showCode}</TabsTrigger><TabsTrigger value="enter">{c.enterCode}</TabsTrigger></TabsList>
            <TabsContent value="show"><div className="guest-code-view">
              {expired ? <><Clock3 size={32} /><h3>{c.expired}</h3><MotionButton className="primary-button" onClick={() => perform(() => runtime.connect())}>{c.newCode}</MotionButton></> : <>
                {qr ? <img width={216} height={216} src={qr} alt={c.code} /> : <div className="guest-qr-placeholder" aria-busy={state.busy} />}
                <strong className="guest-code" data-testid="connection-code">{invite?.code ? invite.code.slice(0, 4) + ' ' + invite.code.slice(4) : '···· ····'}</strong><span className="guest-muted">{c.expires}</span>
                <MotionButton className="secondary-button guest-wide" disabled={!inviteURL} onClick={() => perform(async () => { await navigator.clipboard.writeText(inviteURL); setCopied(true) })}>{copied ? c.copied : c.copyLink}</MotionButton>
              </>}
            </div></TabsContent>
            <TabsContent value="enter"><form className="guest-code-form" onSubmit={event => { event.preventDefault(); void perform(async () => { try { const candidate = await runtime.join(token || code); sessionStorage.removeItem(linkKey); setToken(''); setJoining({ id: candidate.id, invite: candidate.invite_id, expires: Date.parse(candidate.expires_at) }); setPanel('waiting') } catch (error) { if (error instanceof Error && ['EXPIRED', 'NOT_FOUND', 'INVALID_REQUEST'].includes(error.message)) { setToken(''); sessionStorage.removeItem(linkKey) }; throw error } }) }}>
              {!token && <label>{c.code}<Input value={code} onChange={event => setCode(event.target.value.toUpperCase())} autoCapitalize="characters" autoComplete="off" spellCheck={false} maxLength={16} className="guest-code-input" placeholder="K7M4 8F2Q" required /></label>}
              <MotionButton type="submit" className="primary-button guest-wide" disabled={!token && code.replace(/[ -]/g, '').length !== 8} busy={state.busy}>{c.connectAction}</MotionButton>
            </form></TabsContent>
          </Tabs>
        </div>}
        {visualPanel === 'pair' && candidate && <div className="guest-pair"><div className="guest-preview-icon"><DeviceGlyph name={candidate.device_name} /></div><div className="guest-file-copy"><h3>{candidate.device_name}</h3><span className="guest-muted">{c.device} · {candidate.device_id.slice(-4).toUpperCase()}</span></div><MotionButton className="primary-button guest-wide" onClick={() => perform(async () => { await runtime.respondCandidate(candidate.id, true); setPanel(null) })}>{c.connectAction}</MotionButton><MotionButton className="auth-text-button guest-wide" onClick={() => perform(async () => { await runtime.respondCandidate(candidate.id, false); setPanel(null) })}>{c.decline}</MotionButton></div>}
        {visualPanel === 'waiting' && <div className="guest-waiting"><Clock3 size={30} /><p>{c.keepOpen}</p><MotionButton className="secondary-button guest-wide" onClick={() => setPanel(null)}>{c.done}</MotionButton></div>}
        {visualPanel === 'send' && file && <div className="guest-send">
          <div className="guest-file-row"><FileText size={40} strokeWidth={1.3} /><span>{file.name}</span></div>
          {state.connections.peers.length > 1 && <div className="guest-peer-picker" role="group" aria-label={c.selectDevice}>{state.connections.peers.map(peer => <MotionButton className="secondary-button" aria-pressed={selectedPeer?.pair_id === peer.pair_id} key={peer.pair_id} onClick={() => setSelectedPeer(peer)}>{peer.device_name}</MotionButton>)}</div>}
          <MotionButton className="primary-button guest-wide" busy={state.busy} disabled={!selectedPeer} onClick={() => perform(async () => { if (!selectedPeer) return; const id = await runtime.send(file, selectedPeer, maxBytes); setJobId(id); setFile(undefined); setPanel(null) })}>{state.busy ? c.preparing : c.send}</MotionButton>
        </div>}
        {visualPanel === 'job' && selectedJob && <div className="guest-job-detail">
          <div className="guest-file-row"><FileText size={40} strokeWidth={1.3} /><span>{selectedJob.name}</span></div>
          <div className="guest-job-meta"><span>{jobStatus(selectedJob)}</span><span>{formatBytes(selectedJob.plainSize, locale)}</span></div>
          {selectedJob.snapshot && selectedJob.phase !== 'complete' && <dl className="guest-progress-detail"><div><dt>{c.received}</dt><dd>{formatBytes(selectedJob.snapshot.delivery.acknowledged_bytes, locale)} / {formatBytes(selectedJob.snapshot.delivery.total_bytes, locale)}</dd></div><div><dt>{c.upload}</dt><dd>{selectedJob.snapshot.completion.stored ? c.stored : formatBytes(selectedJob.snapshot.spool.acknowledged_bytes, locale)}</dd></div></dl>}
          {selectedJob.transfer?.state === 'PENDING' && selectedJob.direction === 'receive' && selectedJob.phase === 'waiting' ? <><MotionButton className="primary-button guest-wide" onClick={() => perform(async () => { await runtime.accept(selectedJob.id); setPanel(null) })}>{c.receive}</MotionButton><MotionButton className="auth-text-button guest-wide" onClick={() => perform(async () => { await runtime.reject(selectedJob.id); setPanel(null) })}>{c.decline}</MotionButton></> : <>
            {selectedJob.phase === 'complete' && selectedJob.plain && <MotionButton className="primary-button guest-wide" onClick={() => perform(() => runtime.download(selectedJob.id))}><ArrowDownToLine size={18} />{c.download}</MotionButton>}
            <div className="guest-job-controls">
              {selectedJob.snapshot?.capabilities.pause && <MotionTooltip label={c.pause}><MotionButton icon className="guest-icon-button" aria-label={c.pause} onClick={() => perform(() => runtime.control(selectedJob.id, 'pause'))}><Pause size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.snapshot?.capabilities.resume && <MotionTooltip label={c.resume}><MotionButton icon className="guest-icon-button" aria-label={c.resume} onClick={() => perform(() => runtime.control(selectedJob.id, 'resume'))}><Play size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.snapshot?.capabilities.cancel && <MotionTooltip label={c.cancel}><MotionButton icon className="guest-icon-button" aria-label={c.cancel} onClick={() => perform(() => runtime.control(selectedJob.id, 'cancel'))}><X size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.error && selectedJob.phase !== 'stopped' && <MotionTooltip label={c.retry}><MotionButton icon className="guest-icon-button" aria-label={c.retry} onClick={() => perform(() => runtime.retry(selectedJob.id))}><RefreshCw size={21} /></MotionButton></MotionTooltip>}
            </div>
            {['complete', 'stopped'].includes(selectedJob.phase) && <MotionButton className="secondary-button guest-wide" onClick={() => perform(async () => { await runtime.dismiss(selectedJob.id); setPanel(null) })}>{c.another}</MotionButton>}
          </>}
        </div>}
        {visualPanel === 'history' && <div className="guest-history">{state.jobs.length ? state.jobs.map(job => <button key={job.id} className="guest-history-file" onClick={() => openJob(job)}><span className="guest-history-icon"><FileText size={42} strokeWidth={1.2} /><MotionTooltip label={job.phase === 'complete' && job.plain ? c.saved : job.phase === 'stopped' ? (job.error === 'FILE_EXPIRED' || job.snapshot?.state === 'EXPIRED' ? c.fileExpired : c.stopped) : c.once}><span className="guest-file-badge" aria-label={jobStatus(job)}>{job.phase === 'complete' && job.plain ? <Check size={11} /> : job.phase === 'stopped' ? <Clock3 size={11} /> : '1'}</span></MotionTooltip></span><span title={job.name}>{job.name}</span></button>) : <p className="guest-muted">{c.empty}</p>}</div>}
        {errorText && <p className="guest-inline-error" role="alert">{errorText}</p>}
      </div>
    </MotionSurface>
  </section>
}
