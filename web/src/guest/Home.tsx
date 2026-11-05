import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { ArrowDownToLine, ArrowLeft, Clock3, FilePlus2, FileText, FolderOpen, Network, Route, Pause, Play, Plus, RefreshCw, X } from 'lucide-react'
import { authFetch, authState } from '../auth/client'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import QRCode from 'qrcode'
import { MotionButton, MotionReveal, MotionTooltip } from '../motion/components'
import { MotionSurface } from '../motion/surface'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { Input } from '../components/ui/input'
import { Button } from '../components/ui/button'
import { useGuestCopy, guestErrorText } from './copy'
import { displayedRoute } from './route'
import { DeviceGlyph, GuestGraph } from './Graph'
import { getWorkspaceRuntime, WorkspaceRuntime } from './workspace'
import type { GuestJob } from './store'
import type { GuestPeer } from './types'
import { useI18n } from '../i18n'
import { formatBytes } from '../format'
import { routeFilter, routePanel } from './navigation'
import type { FileFilter } from './navigation'
import { AccountRequest, AccountTaskDetails, FilesCollection, guestJobStatus, useAccountWorkspace } from './Files'
import './guest.css'
import { LocalConnect } from '../local/Connect'
import { localError, useLocalCopy } from '../local/copy'
import { Conversation } from './Conversation'
import { TextComposer } from './TextComposer'
import type { TextSensitivity } from './TextComposer'
import { ConnectionEntry } from './ConnectionEntry'
import { joinConnection, unreachable } from './automatic-connection'
import type { GuestInvite } from './types'

const linkKey = 'pinelink:guest-link'
export function captureGuestLink() {
  if (location.pathname !== '/connect') return
  const token = location.hash.slice(1)
  if (/^[A-Za-z0-9_-]{22}$/.test(token)) { try { sessionStorage.setItem(linkKey, JSON.stringify({ token, expires: Date.now() + 600000 })) } catch { /* The current page still provides manual code entry. */ } }
  history.replaceState(null, '', '/connect')
}
const pendingLink = () => { try { const v = JSON.parse(sessionStorage.getItem(linkKey) ?? 'null') as { token: string; expires: number } | null; return v && v.expires > Date.now() ? v.token : '' } catch { return '' } }
const accountDeviceName = (name: string, device: string, locale: string) => {
  if (name !== device || !/^(web|flutter)-[0-9a-f-]{36}$/i.test(device)) return name
  const mobile = device.startsWith('flutter-'), label = locale === 'zh-CN' ? mobile ? '移动设备' : '浏览器' : mobile ? 'Mobile device' : 'Browser'
  return `${label} · ${device.slice(-4).toUpperCase()}`
}

export function GuestHome({ onNotifications }: { onNotifications?: (count: number) => void } = {}) {
  const c = useGuestCopy(), {locale}=useI18n(), [runtime, setRuntime] = useState<WorkspaceRuntime>(), [error, setError] = useState(''), [retry, setRetry] = useState(0)
  useEffect(() => { let live = true; void getWorkspaceRuntime().then(value => { if (live) setRuntime(value) }).catch(reason => { if (live) setError(reason instanceof Error ? reason.message : 'STORAGE_UNAVAILABLE') }); return () => { live = false } }, [retry])
  if (!runtime) {
    if (!error) return <div className="guest-loading" aria-busy="true" />
    const unsupported = error === 'BROWSER_CRYPTO_UNAVAILABLE', insecure = error === 'BROWSER_CONTEXT_UNAVAILABLE', zh=locale==='zh-CN'
    return <section className="guest-storage"><h1>{unsupported?(zh?'请更新浏览器':'Update your browser'):insecure?(zh?'请使用安全地址':'Open Pinelink securely'):c.storage}</h1><p>{unsupported?(zh?'当前浏览器不支持安全传输。':"Secure transfer isn't supported here."):insecure?(zh?'请使用 HTTPS 或 Android 应用。':'Use HTTPS or the Android app.'):c.storageHint}</p>{(unsupported||insecure)&&<Button asChild className="primary-button guest-download"><a href="https://github.com/simiooo/pinelink/releases/tag/v0.1.0-preview.14">{zh?'Android 应用':'Android app'}</a></Button>}<MotionButton className={unsupported||insecure?'guest-startup-retry':'primary-button'} onClick={() => { setError(''); setRetry(value => value + 1) }}>{c.retry}</MotionButton></section>
  }
  return <VisitorHome runtime={runtime} onNotifications={onNotifications} />
}
type Panel = 'connect' | 'pair' | 'waiting' | 'send' | 'job' | 'files' | 'notifications' | 'rename' | 'remove' | 'account-job' | 'account-request' | null
function VisitorHome({ runtime, onNotifications }: { runtime: WorkspaceRuntime; onNotifications?: (count: number) => void }) {
  const c = useGuestCopy(), lc=useLocalCopy(), { t, locale } = useI18n(), snapshot = useSyncExternalStore(runtime.subscribe, runtime.getSnapshot)
  const state = useMemo(() => {
    const namePeer = (peer: GuestPeer) => peer.owner_id.startsWith('acct-') ? { ...peer, device_name: accountDeviceName(peer.device_name, peer.device_id, locale) } : peer
    return { ...snapshot, jobs: snapshot.jobs.map(job => ({ ...job, peer: namePeer(job.peer) })), connections: { ...snapshot.connections, peers: snapshot.connections.peers.map(namePeer), candidates: snapshot.connections.candidates.map(candidate => ({ ...candidate, device_name: accountDeviceName(candidate.device_name, candidate.device_id, locale) })) } }
  }, [snapshot, locale])
  const [localMode,setLocalMode]=useState(false), [connecting,setConnecting]=useState(false), [connectionInvite,setConnectionInvite]=useState<GuestInvite>()
  const pathname = useRouterState({ select: state => state.location.pathname }), navigate = useNavigate()
  const [panel, setPanel] = useState<Panel>(routePanel(pathname)), [filter, setFilter] = useState<FileFilter>(routeFilter(pathname)), [filePeer, setFilePeer] = useState<GuestPeer>()
  const [name, setName] = useState(''), [savingPeer, setSavingPeer] = useState(false), [recordId, setRecordId] = useState<string>(), [requestId, setRequestId] = useState<string>(), [fromFiles, setFromFiles] = useState(false)
  const account = useAccountWorkspace(), auth = authState(), session = auth.session
  useEffect(() => { void runtime.setAccount(session || auth.offlineAccount ? account.reader : undefined).catch(() => undefined) }, [runtime, account.reader, session?.csrf, auth.offlineAccount?.id])
  const accountJobs = state.jobs.filter(job => job.authority === 'ACCOUNT'), accountRefs = new Set(accountJobs.flatMap(job => [job.transfer?.id, job.transfer?.entry_id].filter((value): value is string => !!value)))
  const accountRecords = (account.records.data ?? []).filter(record => !accountRefs.has(record.reference_id)), requests = (account.notifications.data?.transfers ?? []).filter(request => !accountRefs.has(request.id))
  const selectedRecord = accountRecords.find(record => record.id === recordId), selectedRequest = requests.find(request => request.id === requestId)
  const lastPanel = useRef(panel)
  if (panel) lastPanel.current = panel
  const visualPanel = panel ?? lastPanel.current
  const [tab, setTab] = useState(pendingLink() ? 'enter' : 'show'), [code, setCode] = useState(pendingLink)
  const [error, setError] = useState<string>(), [copied, setCopied] = useState(false), [qr, setQr] = useState(''), [dragging, setDragging] = useState(false), [now, setNow] = useState(Date.now())
  const [file, setFile] = useState<File>(), [selectedPeer, setSelectedPeer] = useState<GuestPeer>(), [jobId, setJobId] = useState<string>()
  const [message, setMessage] = useState(''), [sensitivity, setSensitivity] = useState<TextSensitivity>('NORMAL'), [attachmentDraft, setAttachmentDraft] = useState<{ peer: string; file: File }>()
  useEffect(() => { if (!session && !auth.offlineAccount && filePeer?.authority === 'ACCOUNT') { setFilePeer(undefined); setPanel(null); setMessage(''); setAttachmentDraft(undefined) } }, [session, auth.offlineAccount, filePeer])
  const attachment = attachmentDraft && filePeer && attachmentDraft.peer === filePeer.pair_id ? attachmentDraft.file : undefined
  const [candidateId, setCandidateId] = useState<string>(), [joining, setJoining] = useState<{ id: string; invite: string; expires: number }>(), seen = useRef(new Set<string>(state.jobs.filter(job => job.phase === 'complete').map(job => 'done:' + job.id))), input = useRef<HTMLInputElement>(null), dragDepth = useRef(0), connectionAttempt = useRef(0)
  const active = state.jobs.find(job => !job.dismissed && !['complete', 'stopped'].includes(job.phase))
  const selectedJob = state.jobs.find(job => job.id === jobId) ?? active
  const candidate = state.connections.candidates.find(item => item.id === candidateId)
  const maxBytes = authState().capabilities.guest_max_file_bytes ?? (64 << 20)
  const peers = state.offline || now - (state.checkedAt ?? 0) > 15000 ? state.connections.peers.map(peer => peer.authority==='LOCAL_DEVICE'?peer:({ ...peer, online: undefined })) : state.connections.peers
  const graphKey = JSON.stringify([active?.id, peers.map(peer => peer.pair_id).sort()])
  const [graphChoice, setGraphChoice] = useState<{ key: string; route: boolean }>()
  const showRoute = !!active && (graphChoice?.key === graphKey ? graphChoice.route : peers.length <= 1)
  const graphJob = showRoute ? active : undefined
  const errorText = localError(error,lc) || guestErrorText(error, c) || (panel === 'connect' && localMode || !state.error || state.offline ? '' : c.serviceUnavailable), invite = connectionInvite && state.invite?.id === connectionInvite.id ? state.invite : connectionInvite
  const expired = !!invite && (Date.parse(invite.expires_at) <= now || invite.consumed)
  const inviteURL = invite?.token ? new URL('/connect#' + invite.token, location.origin).href : ''
  const pairRequests = state.connections.candidates.filter(item => item.incoming && item.status === 'WAITING' && Date.parse(item.expires_at) > now)
  const fileRequests = state.jobs.filter(job => job.direction === 'receive' && job.phase === 'waiting' && job.transfer?.state === 'PENDING' && Date.parse(job.transfer.expires_at) > (job.authority==='LOCAL_DEVICE'?Date.now():now))
  const notificationCount = pairRequests.length + fileRequests.length + requests.length
  const peerBusy = state.jobs.find(job => job.peer.pair_id === selectedPeer?.pair_id && !['complete', 'stopped'].includes(job.phase) && (!job.transfer || Date.parse(job.transfer.expires_at) > (job.authority==='LOCAL_DEVICE'?Date.now():now)))
  const close = () => { connectionAttempt.current++; if (panel === 'connect' && runtime.local.getSnapshot().phase !== 'connected') runtime.local.cancelPairing(); setPanel(null); setError(undefined); if (pathname !== '/') void navigate({ to: '/', replace: true }) }
  const openFiles = (peer?: GuestPeer, value: FileFilter = 'all') => { setError(undefined); setFilePeer(peer); setFilter(value); setPanel('files'); void navigate({ to: '/files' }) }
  useEffect(() => { setMessage(''); setSensitivity('NORMAL'); setAttachmentDraft(current => current?.peer === filePeer?.pair_id ? current : undefined) }, [filePeer?.pair_id])
  useEffect(() => { setPanel(routePanel(pathname)); setError(undefined); if (['/tasks', '/history', '/local-files'].includes(pathname)) { setFilter(routeFilter(pathname)); setFilePeer(undefined) } }, [pathname])
  useEffect(() => { onNotifications?.(notificationCount) }, [onNotifications, notificationCount])
  const perform = async (task: () => Promise<unknown>) => { setError(undefined); try { await task() } catch (e) { setError(e instanceof Error ? e.message : 'GUEST_UNAVAILABLE') } }
  const retryService = async () => { setError(undefined); try { await runtime.ensureSession(); await runtime.poll() } catch { setError('SERVICE_UNAVAILABLE') } }
  useEffect(() => { const timer = setInterval(() => setNow(runtime.now()), 1000); return () => clearInterval(timer) }, [runtime])
  useEffect(() => {
    let live = true; setQr('')
    if (inviteURL && !expired) void QRCode.toDataURL(inviteURL, { width: 216, margin: 4, errorCorrectionLevel: 'M', color: { dark: '#101010', light: '#FFFFFF' } }).then(url => { if (live) setQr(url) }).catch(() => undefined)
    return () => { live = false }
  }, [inviteURL, expired])
  useEffect(() => {
    const incoming = state.connections.candidates.find(item => item.incoming && item.status === 'WAITING' && !seen.current.has(item.id))
    if (incoming && (!panel || panel === 'connect')) { seen.current.add(incoming.id); setCandidateId(incoming.id); setPanel('pair'); return }
    const pending = state.jobs.find(job => job.direction === 'receive' && job.phase === 'waiting' && job.transfer?.state === 'PENDING' && Date.parse(job.transfer.expires_at) > (job.authority==='LOCAL_DEVICE'?Date.now():now) && !seen.current.has(job.id))
    if (pending && !panel) { seen.current.add(pending.id); setJobId(pending.id); setFromFiles(false); setPanel('job'); return }
    if (panel === 'files') for (const job of state.jobs) if (job.phase === 'complete') seen.current.add('done:' + job.id)
    const completed = state.jobs.find(job => job.phase === 'complete' && !seen.current.has('done:' + job.id))
    if (completed && (!panel || panel === 'job' && completed.id === jobId)) { seen.current.add('done:' + completed.id); setJobId(completed.id); setPanel('job') }
    if (panel === 'waiting' && joining) {
      if (state.connections.peers.some(peer => peer.invite_id === joining.invite)) setPanel(null)
      else if (state.connections.candidates.find(item => item.id === joining.id)?.status === 'DECLINED') { setError('CONNECTION_DECLINED'); setTab('enter'); setPanel('connect') }
      else if (joining.expires <= now) { setError('EXPIRED'); setTab('enter'); setPanel('connect') }
    }
  }, [state.connections, state.jobs, panel, joining, now, jobId])
  const startConnection = (expected?: string) => {
    const attempt = ++connectionAttempt.current; setLocalMode(!!globalThis.RTCPeerConnection); setConnecting(true); setConnectionInvite(undefined); setCopied(false); setError(undefined)
    void (async () => {
      try {
        let online: GuestInvite | undefined
        if (!expected && navigator.onLine !== false) {
          try {
            const localOnly = authState().capabilities.local_only && !authState().capabilities.enabled && (await authFetch<{ local_only?: boolean }>('/v1/auth/capabilities')).local_only
            if (!localOnly) online = await runtime.connect()
          } catch (reason) { if (!unreachable(reason)) throw reason }
        }
        if (attempt !== connectionAttempt.current) return
        setConnectionInvite(online)
        if (globalThis.RTCPeerConnection) {
          try { await runtime.local.invite(expected, online?.token ? { origin: location.origin, token: online.token, expires_at: online.expires_at } : undefined) }
          catch (reason) { if (!online) throw reason; if (attempt === connectionAttempt.current) setLocalMode(false) }
        } else if (!online) throw new Error('LOCAL_RTC_UNAVAILABLE')
      } catch (reason) {
        if (attempt !== connectionAttempt.current) return
        setError(reason instanceof Error ? reason.message : 'GUEST_UNAVAILABLE')
      } finally { if (attempt === connectionAttempt.current) setConnecting(false) }
    })()
  }
  const connect = () => { runtime.local.cancelPairing(); setPanel('connect'); setTab('show'); startConnection() }
  const reconnect=(peer:GuestPeer)=>{setPanel('connect');setTab('show');startConnection(peer.root)}
  const joinCode = async (input: string) => {
    const attempt = ++connectionAttempt.current
    setError(undefined)
    try {
      const result = await joinConnection(input, location.origin, { online: navigator.onLine !== false && !runtime.local.getSnapshot().expected, joinOnline: value => runtime.join(value), joinLocal: value => runtime.local.join(value, runtime.local.getSnapshot().expected) })
      if (attempt !== connectionAttempt.current) return
      setConnecting(false)
      if (result.kind === 'local') {
        setCode(''); setLocalMode(true); setTab('show'); return
      }
      const candidate = result.candidate
      runtime.local.cancelPairing(); connectionAttempt.current++; sessionStorage.removeItem(linkKey); setCode(''); setJoining({ id: candidate.id, invite: candidate.invite_id, expires: Date.parse(candidate.expires_at) }); setPanel('waiting')
    } catch (error) {
      if (error instanceof Error && ['EXPIRED', 'NOT_FOUND', 'INVALID_REQUEST'].includes(error.message)) sessionStorage.removeItem(linkKey)
      if (attempt === connectionAttempt.current) setError(error instanceof Error ? error.message : 'GUEST_UNAVAILABLE')
    } finally { if (attempt === connectionAttempt.current) setConnecting(false) }
  }
  const openJob = (job?: GuestJob) => { if (job) { setFromFiles(panel === 'files'); setJobId(job.id); setPanel('job') } }
  const choose = (chosen?: File, peer?: GuestPeer) => {
    if (!chosen) { if (peer) setSelectedPeer(peer); input.current?.click(); return }
    setFile(chosen); setSelectedPeer(peer ?? state.connections.peers.find(peer => peer.pair_id === selectedPeer?.pair_id) ?? state.connections.peers[0])
    if (!state.connections.peers.length) connect(); else setPanel('send')
  }
  const detailPeer = peers.find(peer => peer.pair_id === selectedJob?.peer.pair_id) ?? selectedJob?.peer
  const detailTitle = detailPeer ? `${detailPeer.local_name || detailPeer.device_name} · ${detailPeer.device_id.slice(-8).toUpperCase()}` : c.details
  const title = visualPanel === 'connect' ? c.connect : visualPanel === 'pair' ? c.confirmPair : visualPanel === 'waiting' ? c.waiting : visualPanel === 'files' ? filePeer?.local_name || filePeer?.device_name || c.files : visualPanel === 'notifications' ? c.notifications : visualPanel === 'remove' ? c.removeTitle : visualPanel === 'rename' || visualPanel === 'send' ? selectedPeer?.local_name || selectedPeer?.device_name || c.selectDevice : visualPanel === 'account-job' || visualPanel === 'account-request' ? c.details : detailTitle
  return <section className="guest-page" data-guest-job={active?.transfer?.id} data-phase={active?.phase}
    onDragEnter={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); dragDepth.current++; setDragging(true) } }}
    onDragOver={event => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); event.dataTransfer.dropEffect = 'copy' } }}
    onDragLeave={() => { if (--dragDepth.current <= 0) { dragDepth.current = 0; setDragging(false) } }}
    onDrop={event => { event.preventDefault(); dragDepth.current = 0; setDragging(false); if (event.dataTransfer.files.length === 1) { if (panel === 'files' && filePeer) setAttachmentDraft({ peer: filePeer.pair_id, file: event.dataTransfer.files[0] }); else choose(event.dataTransfer.files[0]) } else setError('ONE_FILE_ONLY') }}>
    <input ref={input} type="file" hidden aria-label={c.file} onChange={event => { choose(event.target.files?.[0]); event.target.value = '' }} />
    <GuestGraph peers={peers} job={graphJob} onConnect={connect} onConversation={peer => openFiles(peer)} onDetails={() => openJob(graphJob)} dragging={dragging} localDevice={graphJob ? graphJob.authority === 'ACCOUNT' ? runtime.accountProfile?.device ?? runtime.profile.device : runtime.profile.device : runtime.accountProfile?.device ?? runtime.profile.device} localAccount={graphJob ? graphJob.authority === 'ACCOUNT' : runtime.hasAccount} onFiles={() => openFiles(undefined, 'saved')} onRename={peer => { setSelectedPeer(peer); setName(peer.local_name || peer.device_name); setError(undefined); setPanel('rename') }} onRemove={peer => { setSelectedPeer(peer); setError(undefined); setPanel('remove') }} onFileDrop={(file, peer) => { dragDepth.current = 0; setDragging(false); openFiles(peer); setAttachmentDraft({ peer: peer.pair_id, file }) }} />
    {active && <div className="guest-graph-tools"><MotionTooltip label={showRoute ? c.devices : c.viewTransfer}><MotionButton icon className="guest-icon-button" aria-label={showRoute ? c.devices : c.viewTransfer} onClick={() => setGraphChoice({ key: graphKey, route: !showRoute })}>{showRoute ? <Network size={21} /> : <Route size={21} />}</MotionButton></MotionTooltip></div>}
    {dragging && <div className="guest-drop-label" role="status">{c.file}</div>}
    <footer className="guest-footer">
      {state.offline && <span className="guest-offline" role="status">{lc.noInternet}</span>}
      <div className="guest-toolbar" role="group" aria-label={c.quickActions}>
        <MotionTooltip label={c.files}><MotionButton icon className="guest-icon-button" aria-label={c.files} onClick={() => openFiles()}><FolderOpen size={24} /></MotionButton></MotionTooltip>
        <MotionTooltip label={c.connect}><MotionButton icon className="guest-icon-button workspace-add-device" aria-label={c.connect} onClick={connect}><Plus size={24} /></MotionButton></MotionTooltip>
        <MotionTooltip label={c.file}><MotionButton icon className="guest-icon-button" aria-label={c.file} onClick={() => choose()} disabled={state.busy}><FilePlus2 size={24} /></MotionButton></MotionTooltip>
      </div>
    </footer>
    {errorText && !panel && <MotionReveal role="alert" className="guest-error"><span>{errorText}</span><MotionButton icon className="guest-icon-button" aria-label={c.retry} onClick={retryService}><RefreshCw size={18} /></MotionButton></MotionReveal>}
    <MotionSurface open={!!panel} onClose={close} title={title} closeLabel={c.close}>
      <div className="guest-surface-body">
        {visualPanel === 'connect' && <div className="guest-connect">
          <Tabs value={tab} onValueChange={value => { setTab(value); setError(undefined); if (value === 'show' && !connecting && !invite && runtime.local.getSnapshot().phase === 'idle') startConnection() }}>
            <TabsList variant="line" className="guest-tabs"><TabsTrigger value="show">{c.showCode}</TabsTrigger><TabsTrigger value="enter">{c.enterCode}</TabsTrigger></TabsList>
            <TabsContent value="show">{localMode ? <LocalConnect runtime={runtime.local} onConnected={close} onJoin={joinCode} onEnter={()=>setTab('enter')} onNewCode={()=>startConnection(runtime.local.getSnapshot().expected)} preparing={connecting} onlineCode={!expired?invite?.code:undefined}/> : <div className="guest-code-view">
              {expired || !invite && !connecting && error ? <>{expired && <><Clock3 size={32} /><h3>{c.expired}</h3></>}<MotionButton className="primary-button" onClick={() => startConnection()}>{expired ? c.newCode : c.retry}</MotionButton></> : <>
                {qr ? <img width={216} height={216} src={qr} alt={c.code} /> : <div className="guest-qr-placeholder" aria-busy={state.busy} />}
                <strong className="guest-code" data-testid="connection-code">{invite?.code ? invite.code.slice(0, 4) + ' ' + invite.code.slice(4) : '···· ····'}</strong><span className="guest-muted">{c.expires}</span>
                <MotionButton className="secondary-button guest-wide" disabled={!inviteURL} onClick={() => perform(async () => { await navigator.clipboard.writeText(inviteURL); setCopied(true) })}>{copied ? c.copied : c.copyLink}</MotionButton>
              </>}
            </div>}</TabsContent>
            <TabsContent value="enter"><ConnectionEntry value={code} onValue={setCode} onSubmit={joinCode} onError={setError} /></TabsContent>
          </Tabs>
        </div>}
        {visualPanel === 'pair' && candidate && <div className="guest-pair"><div className="guest-preview-icon"><DeviceGlyph name={candidate.device_name} /></div><div className="guest-file-copy"><h3>{candidate.device_name}</h3><span className="guest-muted">{c.device} · {candidate.device_id.slice(-4).toUpperCase()}</span></div><MotionButton className="primary-button guest-wide" onClick={() => perform(async () => { await runtime.respondCandidate(candidate.id, true); runtime.local.cancelPairing(); setPanel(null) })}>{c.connectAction}</MotionButton><MotionButton className="auth-text-button guest-wide" onClick={() => perform(async () => { await runtime.respondCandidate(candidate.id, false); setPanel(null) })}>{c.decline}</MotionButton></div>}
        {visualPanel === 'waiting' && <div className="guest-waiting"><Clock3 size={30} /><p>{c.keepOpen}</p><MotionButton className="secondary-button guest-wide" onClick={() => setPanel(null)}>{c.done}</MotionButton></div>}
        {visualPanel === 'send' && file && <div className="guest-send">
          <div className="guest-file-row"><FileText size={40} strokeWidth={1.3} /><span>{file.name}</span></div>
          {state.connections.peers.length > 1 && <div className="guest-peer-picker" role="group" aria-label={c.selectDevice}>{state.connections.peers.map(peer => <MotionButton className="secondary-button" aria-pressed={selectedPeer?.pair_id === peer.pair_id} key={peer.pair_id} onClick={() => setSelectedPeer(peer)}>{peer.local_name || peer.device_name}</MotionButton>)}</div>}
          <MotionButton className="primary-button guest-wide" busy={state.busy} disabled={!selectedPeer} onClick={() => perform(async () => { if (!selectedPeer) return; const id = await runtime.send(file, selectedPeer, maxBytes); setJobId(id); setFile(undefined); setPanel(null) })}>{state.busy ? c.preparing : c.send}</MotionButton>
        </div>}
        {visualPanel === 'job' && selectedJob && <div className="guest-job-detail">
          {detailPeer && <div className="guest-connection-identity"><span>{c.device}</span><code>{detailPeer.device_id}</code></div>}
          <div className="guest-file-row"><FileText size={40} strokeWidth={1.3} /><span>{selectedJob.payloadKind === 'TEXT' ? locale === 'zh-CN' ? '文本' : 'Text' : selectedJob.name}</span></div>
          {fromFiles && <MotionTooltip label={c.backFiles}><MotionButton icon className="guest-icon-button workspace-back" aria-label={c.backFiles} onClick={() => setPanel('files')}><ArrowLeft size={20} /></MotionButton></MotionTooltip>}
          <div className="guest-job-meta"><span>{guestJobStatus(selectedJob, c)}</span><span>{formatBytes(selectedJob.plainSize, locale)}</span></div>
          {selectedJob.snapshot && <dl className="guest-progress-detail"><div><dt>{c.connection}</dt><dd>{selectedJob.authority==='LOCAL_DEVICE'?lc.local:displayedRoute(selectedJob) === 'direct' ? c.direct : displayedRoute(selectedJob) === 'relay' ? c.cloud : displayedRoute(selectedJob) === 'switching' ? c.switching : c.connecting}</dd></div>{selectedJob.phase !== 'complete' && <><div><dt>{c.received}</dt><dd>{formatBytes(selectedJob.snapshot.delivery.acknowledged_bytes, locale)} / {formatBytes(selectedJob.snapshot.delivery.total_bytes, locale)}</dd></div>{displayedRoute(selectedJob) === 'relay' && <div><dt>{c.upload}</dt><dd>{selectedJob.snapshot.completion.stored ? c.stored : formatBytes(selectedJob.snapshot.spool.acknowledged_bytes, locale)}</dd></div>}</>}</dl>}
          {selectedJob.transfer?.state === 'PENDING' && selectedJob.direction === 'receive' && selectedJob.phase === 'waiting' ? <><MotionButton className="primary-button guest-wide" onClick={() => perform(async () => { await runtime.accept(selectedJob.id); setPanel(fromFiles ? 'files' : null) })}>{c.receive}</MotionButton><MotionButton className="auth-text-button guest-wide" onClick={() => perform(async () => { await runtime.reject(selectedJob.id); setPanel(fromFiles ? 'files' : null) })}>{c.decline}</MotionButton></> : <>
            {selectedJob.phase === 'complete' && selectedJob.plain && <MotionButton className="primary-button guest-wide" onClick={() => perform(() => runtime.download(selectedJob.id))}><ArrowDownToLine size={18} />{c.download}</MotionButton>}
            <div className="guest-job-controls">
              {selectedJob.snapshot?.capabilities.pause && <MotionTooltip label={c.pause}><MotionButton icon className="guest-icon-button" aria-label={c.pause} onClick={() => perform(() => runtime.control(selectedJob.id, 'pause'))}><Pause size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.snapshot?.capabilities.resume && !(selectedJob.authority==='LOCAL_DEVICE'&&!runtime.local.reachable(selectedJob.peer.root)) && <MotionTooltip label={c.resume}><MotionButton icon className="guest-icon-button" aria-label={c.resume} onClick={() => perform(() => runtime.control(selectedJob.id, 'resume'))}><Play size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.snapshot?.capabilities.cancel && <MotionTooltip label={c.cancel}><MotionButton icon className="guest-icon-button" aria-label={c.cancel} onClick={() => perform(() => runtime.control(selectedJob.id, 'cancel'))}><X size={21} /></MotionButton></MotionTooltip>}
              {selectedJob.authority==='LOCAL_DEVICE'&&!runtime.local.reachable(selectedJob.peer.root)&&!['complete','stopped'].includes(selectedJob.phase)&&<MotionButton className="primary-button guest-wide" onClick={()=>reconnect(selectedJob.peer)}>{lc.reconnect}</MotionButton>}
              {selectedJob.error && selectedJob.authority!=='LOCAL_DEVICE' && selectedJob.phase !== 'stopped' && <MotionTooltip label={c.retry}><MotionButton icon className="guest-icon-button" aria-label={c.retry} onClick={() => perform(() => runtime.retry(selectedJob.id))}><RefreshCw size={21} /></MotionButton></MotionTooltip>}
            </div>
            {['complete', 'stopped'].includes(selectedJob.phase) && <MotionButton className="secondary-button guest-wide" onClick={() => { close(); void perform(() => runtime.dismiss(selectedJob.id)) }}>{c.another}</MotionButton>}
          </>}
        </div>}
        {visualPanel === 'files' && (filePeer ? (filePeer.authority !== 'ACCOUNT' || runtime.accountProfile) && <div className="conversation-panel">
          <Conversation jobs={state.jobs.filter(job => job.peer.pair_id === filePeer.pair_id)} records={filePeer.authority === 'ACCOUNT' ? accountRecords.filter(record => record.peer === filePeer.device_id) : []} profile={filePeer.authority === 'LOCAL_DEVICE' ? runtime.local.profile : filePeer.authority === 'ACCOUNT' ? runtime.accountProfile! : runtime.profile} locale={locale} onJob={openJob} onRecord={record => { setRecordId(record.id); setPanel('account-job') }} labels={locale === 'zh-CN' ? { copy: '复制', copied: '已复制', copyFailed: '请选择文字复制', sensitive: '敏感信息', reveal: '显示', hide: '隐藏', unavailable: '不可用', waiting: '等待中', sent: '已发送', received: '已接收', stored: '已存储', file: '文件' } : { copy: 'Copy', copied: 'Copied', copyFailed: 'Select text to copy', sensitive: 'Sensitive', reveal: 'Reveal', hide: 'Hide', unavailable: 'Unavailable', waiting: 'Waiting', sent: 'Sent', received: 'Received', stored: 'Stored', file: 'File' }} />
          {errorText && <p className="guest-inline-error" role="alert">{errorText}</p>}
          <TextComposer value={message} sensitivity={sensitivity} attachment={attachment} supported={filePeer.authority !== 'LOCAL_DEVICE' || runtime.local.supportsText(filePeer.root)} busy={state.busy} onValue={setMessage} onSensitivity={setSensitivity} onAttachment={file => setAttachmentDraft(file ? { peer: filePeer.pair_id, file } : undefined)} onSend={includeText => perform(async () => {
            if (attachment) { await runtime.send(attachment, filePeer, maxBytes); setAttachmentDraft(current => current?.file === attachment && current.peer === filePeer.pair_id ? undefined : current) }
            if (includeText) { await runtime.send(new File([message], 'Text', { type: 'text/plain;charset=utf-8' }), filePeer, maxBytes, { sensitivity }); setMessage(current => current === message ? '' : current) }
          })} onSendFile={() => { setAttachmentDraft({ peer: filePeer.pair_id, file: new File([message], 'message.txt', { type: 'text/plain;charset=utf-8' }) }); setMessage('') }} labels={locale === 'zh-CN' ? { sensitive: '敏感信息', input: '输入文字', send: '发送', attach: '添加文件', removeAttachment: '移除附件', tooLong: '文字过长', invalidText: '文字格式无效', unsupported: '设备不支持文本', sendFile: '作为文件发送', sensitiveFileWarning: '文件可以被保存和显示。', confirm: '确认', cancel: '取消' } : { sensitive: 'Sensitive', input: 'Write a message', send: 'Send', attach: 'Attach file', removeAttachment: 'Remove attachment', tooLong: 'Text is too long', invalidText: 'Invalid text', unsupported: 'Device does not support text', sendFile: 'Send as file', sensitiveFileWarning: 'The file can be saved and displayed.', confirm: 'Confirm', cancel: 'Cancel' }} />
        </div> : <FilesCollection jobs={state.jobs.filter(job => job.payloadKind !== 'TEXT')} records={accountRecords} filter={filter} onFilter={setFilter} onJob={openJob} onRecord={record => { setRecordId(record.id); setPanel('account-job') }} error={account.ready && account.records.isError} loading={account.ready && account.records.isPending} />)}
        {visualPanel === 'account-job' && selectedRecord && <><MotionTooltip label={c.backFiles}><MotionButton icon className="guest-icon-button" aria-label={c.backFiles} onClick={() => setPanel('files')}><ArrowLeft size={20} /></MotionButton></MotionTooltip><AccountTaskDetails record={selectedRecord} queryKey={account.recordKey} reader={account.reader} /></>}
        {visualPanel === 'account-request' && selectedRequest && <AccountRequest request={selectedRequest} onDone={() => openFiles(undefined, 'active')} reader={account.reader} />}
        {visualPanel === 'notifications' && <div className="workspace-notifications-list">
          {pairRequests.map(request => <MotionButton key={request.id} className="workspace-notification" onClick={() => { seen.current.add(request.id); setCandidateId(request.id); setPanel('pair') }}><DeviceGlyph name={request.device_name} /><span><strong>{request.device_name}</strong><small>{c.wantsConnect}</small></span></MotionButton>)}
          {fileRequests.map(job => <MotionButton key={job.id} className="workspace-notification" onClick={() => { seen.current.add(job.id); openJob(job) }}><FileText /><span><strong>{job.name}</strong><small>{c.waitingFile}</small></span></MotionButton>)}
          {requests.map(request => <MotionButton key={request.id} className="workspace-notification" onClick={() => { setRequestId(request.id); setPanel('account-request') }}><FileText /><span><strong>{request.file_name === 'encrypted' ? t('encryptedFile') : request.file_name || c.file}</strong><small>{request.sender_device_id}</small></span></MotionButton>)}
          {!notificationCount && (!account.ready || !account.notifications.isError && !account.notifications.isPending) && <p className="guest-muted" role="status">{c.noNotifications}</p>}
          {account.ready && account.notifications.isError && <p role="alert" className="guest-inline-error">{c.accountNotificationsError}</p>}
        </div>}
        {visualPanel === 'rename' && selectedPeer && <form className="workspace-rename" onSubmit={event => { event.preventDefault(); if (savingPeer) return; setSavingPeer(true); void perform(async () => { try { await runtime.renamePeer(selectedPeer.pair_id, name); close() } finally { setSavingPeer(false) } }) }}><label>{c.nameLabel}<Input value={name} maxLength={128} onChange={event => setName(event.target.value)} autoComplete="off" /></label><MotionButton className="primary-button guest-wide" type="submit" busy={savingPeer} disabled={!name.trim()}>{c.save}</MotionButton></form>}
        {visualPanel === 'remove' && selectedPeer && <div className="workspace-remove"><p className="guest-muted">{peerBusy ? c.connectionBusy : state.offline && selectedPeer.authority!=='LOCAL_DEVICE' ? c.removeOffline : c.removeHint}</p>{peerBusy ? <MotionButton className="primary-button guest-wide" onClick={() => openJob(peerBusy)}>{c.viewTransfer}</MotionButton> : <MotionButton className="primary-button guest-wide" disabled={state.offline&&selectedPeer.authority!=='LOCAL_DEVICE'} busy={savingPeer} onClick={() => { if (savingPeer) return; setSavingPeer(true); void perform(async () => { try { await runtime.removePeer(selectedPeer.pair_id); setSelectedPeer(undefined); close() } finally { setSavingPeer(false) } }) }}>{c.remove}</MotionButton>}</div>}
        {errorText && !(visualPanel === 'files' && filePeer) && <p className="guest-inline-error" role="alert">{errorText}</p>}
      </div>
    </MotionSurface>
  </section>
}
