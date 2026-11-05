import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { Check, CircleAlert, Eye, EyeOff, Mail } from 'lucide-react'
import { Input } from '../components/ui/input'
import { MotionButton, MotionReveal, MotionSpinner, MotionTooltip } from '../motion/components'
import { authFetch, authReturnError, authRevision, authState, confirmLink, continueOffline, currentPending, googleLogin, inspectLink, login, resetPassword, signOut, startEmail, subscribeAuth, updateChallenge, verifyCode } from '../auth/client'
import type { Pending } from '../auth/client'
import { authErrorText, useAuthCopy } from '../auth/copy'
import { useGuestCopy } from '../guest/copy'
import '../styles/auth.css'

const errorCode = (error: unknown) => error instanceof Error ? error.message : 'AUTH_UNAVAILABLE'
export function AuthPage() {
  const path = useRouterState({ select: state => state.location.pathname }), navigate = useNavigate(), base = useAuthCopy(), guestCopy = useGuestCopy()
  const c = { ...base, offline: authState().capabilities.guests ? guestCopy.continueGuest : base.offline }
  useSyncExternalStore(subscribeAuth, authRevision)
  const [email, setEmail] = useState(''), [password, setPassword] = useState(''), [code, setCode] = useState(''), [visible, setVisible] = useState(false)
  const [busy, setBusy] = useState(false), [error, setError] = useState<string>(), [pending, setPending] = useState<Pending>(), [ready, setReady] = useState(false), [done, setDone] = useState('')
  const [link, setLink] = useState<Awaited<ReturnType<typeof inspectLink>>>(), [native, setNative] = useState<{ id: string; device_name: string; code: string; csrf: string }>(), [now, setNow] = useState(Date.now())
  const gate = useRef(false), lifetime = useRef(0), alert = useRef<HTMLParagraphElement>(null)
  useEffect(() => { lifetime.current++; return () => { lifetime.current++ } }, [])
  useEffect(() => {
    let active = true; setReady(false); setError(undefined); setDone(''); setPassword(''); setCode(''); setLink(undefined); setNative(undefined)
    const load = async () => {
      if (path === '/verify' || path === '/reset') { const p = await currentPending(); if (active) { setPending(p); if (!p) setError('AUTH_EXPIRED') } }
      if (path === '/verify-link') { const info = await inspectLink(); if (active) setLink(info) }
      if (path === '/auth/device') { const id = new URLSearchParams(location.search).get('request_id'); const review = await authFetch<typeof native>('/v1/auth/native-review?request_id=' + encodeURIComponent(id ?? '')); if (active) setNative(review) }
      if (path === '/auth/return') { const reason = authReturnError(); if (active) setError(reason ?? 'AUTH_NOT_READY') }
      if (path === '/login') { const reason = new URLSearchParams(location.search).get('error'); if (active && reason) setError(reason) }
    }
    void load().catch(e => { if (active) setError(errorCode(e)) }).finally(() => { if (active) setReady(true) })
    return () => { active = false }
  }, [path])
  useEffect(() => { if (error) alert.current?.focus() }, [error])
  useEffect(() => {
    if (!pending?.challenge || path !== '/verify') return
    let active = true
    const timer = window.setInterval(() => {
      setNow(Date.now())
      if (document.visibilityState !== 'hidden' && ['pending', 'retry'].includes(pending.challenge?.delivery_state ?? '')) {
        void updateChallenge(pending).then(p => { if (active) setPending(p) }).catch(() => undefined)
      }
    }, 2000)
    return () => { active = false; clearInterval(timer) }
  }, [pending, path])
  const run = async (work: () => Promise<unknown>) => {
    if (gate.current) return
    gate.current = true; const epoch = lifetime.current; setBusy(true); setError(undefined)
    try { await work() } catch (e) { if (lifetime.current === epoch) setError(errorCode(e)) }
    finally { gate.current = false; if (lifetime.current === epoch) setBusy(false) }
  }
  const nav = (to: string) => navigate({ to })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    void run(async () => {
      if (path === '/login') { await login(email, password); setPassword('') }
      else if (path === '/register' || path === '/forgot') { const p = await startEmail(path === '/register' ? 'signup' : 'reset', email, password); setPassword(''); setPending(p); await nav('/verify') }
      else if (path === '/verify' && pending) { const state = await verifyCode(pending, code); if (state === 'reset') await nav('/reset'); else if (state === 'existing_account') setDone('existing') }
      else if (path === '/reset' && pending) { await resetPassword(pending, password); setPassword(''); setDone('reset') }
      else if (path === '/verify-link' && link) { const state = await confirmLink(link, password); setPassword(''); if (state === 'existing_account') setDone('existing'); if (state === 'reset') setDone('reset') }
    })
  }
  const passwordField = (fresh: boolean) => <div className="auth-field"><label htmlFor="auth-password">{fresh ? c.newPassword : c.password}</label><span className="auth-password"><Input id="auth-password" type={visible ? 'text' : 'password'} autoComplete={fresh ? 'new-password' : 'current-password'} value={password} required maxLength={256} onChange={e => setPassword(e.target.value)} aria-describedby={fresh ? 'password-hint' : undefined} /><MotionTooltip label={visible ? c.hide : c.show}><MotionButton icon aria-label={visible ? c.hide : c.show} aria-pressed={visible} onClick={() => setVisible(v => !v)}>{visible ? <EyeOff size={20} /> : <Eye size={20} />}</MotionButton></MotionTooltip></span>{fresh && <small id="password-hint">{c.minimum}</small>}</div>
  const emailField = <label className="auth-field"><span>{c.email}</span><Input type="email" autoComplete="username" value={email} required maxLength={254} onChange={e => setEmail(e.target.value)} inputMode="email" autoCapitalize="none" spellCheck={false} /></label>
  const footer = <footer className="auth-footer"><Link to="/terms">{c.terms}</Link><Link to="/privacy">{c.privacy}</Link></footer>
  const { capabilities, session, offlineAccount } = authState()
  let body: ReactNode, title = c.signIn
  if (path === '/terms' || path === '/privacy') {
    title = path === '/terms' ? c.terms : c.privacy; body = <><p className="auth-legal">{path === '/terms' ? c.termsText : c.privacyText}</p>{path === '/privacy' && <p className="auth-legal">{c.passwordCheck}</p>}<Link to="/login" className="auth-link">{c.back}</Link></>
  } else if (done) {
    title = done === 'reset' ? c.resetDone : done === 'approved' ? c.approved : done === 'cancelled' ? c.cancelled : c.existing; body = <>{done !== 'cancelled' && <span className="auth-result"><Check size={28} /></span>}{done === 'cancelled' && <p className="auth-note">{c.approved}</p>}{['reset', 'existing'].includes(done) && <><p>{done === 'reset' ? c.signInAgain : c.existingHint}</p><Link to="/login" className="auth-link">{c.signIn}</Link></>}</>
  } else if (path === '/account') {
    title = c.account; body = <><div className="auth-account"><strong>{session?.account.name || session?.account.email || offlineAccount?.email || c.noAccount}</strong>{session?.account.name ? <span>{session.account.email}</span> : !session ? <span>{offlineAccount ? c.accountExpired : c.local}</span> : null}{session && <div className="auth-methods">{session.account.methods.map(m => <span key={m}>{m === 'google' ? c.googleMethod : c.passwordMethod}</span>)}</div>}</div>{session ? <MotionButton busy={busy} onClick={() => run(signOut)}>{c.signOut}</MotionButton> : <><Link to="/login" className="auth-link">{c.signIn}</Link>{offlineAccount && <p className="auth-note">{c.removeOffline}</p>}</>}</>
  } else if (path === '/auth/complete') {
    { const reason = new URLSearchParams(location.search).get('error'); title = reason ? reason === 'GOOGLE_CANCELLED' ? c.cancelled : c.unavailable : c.approved; body = reason ? <p className="auth-note">{c.approved}</p> : <span className="auth-result"><Check size={28} /></span> }
  } else if (path === '/auth/device') {
    title = c.nativeTitle; body = !ready ? <MotionSpinner /> : native ? <><p className="auth-note">{native.device_name || 'Pinelink'}</p><p className="auth-device-code">{native.code.slice(0, 4)} {native.code.slice(4)}</p><p className="auth-note">{c.nativeHint}</p>{done ? <p>{done === 'approved' ? c.approved : c.cancelled}</p> : <div className="auth-actions"><MotionButton className="primary-button" busy={busy} onClick={() => run(async () => { await nativeDecision(native, true); setNative(undefined); setDone('approved') })}>{c.allow}</MotionButton><MotionButton busy={busy} onClick={() => run(async () => { await nativeDecision(native, false); setNative(undefined); setDone('cancelled') })}>{c.deny}</MotionButton></div>}</> : null
  } else if (path === '/verify' && pending?.challenge) {
    title = pending.challenge.purpose === 'reset' ? c.reset : c.verify
    const delivery = pending.challenge.delivery_state, seconds = Math.max(0, Math.ceil((Date.parse(pending.challenge.resend_at) - now) / 1000))
    body = <><div className="auth-mail"><Mail size={28} /><strong>{pending.email}</strong><span role="status">{['pending', 'retry'].includes(delivery) ? c.sending : delivery === 'failed' ? c.mailFailed : delivery === 'conditional' ? c.conditional : c.sent}</span></div><form onSubmit={submit} className="auth-form"><label className="auth-field"><span>{c.code}</span><Input className="auth-code" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 8))} required minLength={8} maxLength={8} autoFocus /></label><MotionButton className="primary-button" type="submit" busy={busy}>{c.confirm}</MotionButton></form><MotionButton className="auth-text-button" disabled={seconds > 0} busy={busy} onClick={() => run(async () => { setPending(await updateChallenge(pending, true)); setNow(Date.now()) })}>{seconds > 0 ? `${c.wait} ${seconds}${c.seconds}` : c.resend}</MotionButton><Link to={pending.challenge.purpose === 'reset' ? '/forgot' : '/register'} className="auth-link">{c.emailChange}</Link></>
  } else if (path === '/verify-link') {
    title = link?.purpose === 'reset' ? c.reset : link?.sameDevice ? c.verify : c.finishHere
    body = !ready ? <MotionSpinner /> : link ? <><form className="auth-form" onSubmit={submit}>{(link.purpose === 'reset' || !link.sameDevice) && passwordField(true)}<MotionButton className="primary-button" type="submit" busy={busy}>{link.purpose === 'reset' ? c.reset : c.confirm}</MotionButton></form>{!link.sameDevice && <p className="auth-note">{c.otherDevice}</p>}</> : <Link to="/login" className="auth-link">{c.back}</Link>
  } else if (path === '/reset') {
    title = c.reset; body = pending?.resetGrant ? <form onSubmit={submit} className="auth-form">{passwordField(true)}<MotionButton className="primary-button" type="submit" busy={busy}>{c.reset}</MotionButton></form> : <Link to="/forgot" className="auth-link">{c.forgot}</Link>
  } else if (path === '/auth/return' || path === '/verify') {
    body = <Link to="/login" className="auth-link">{c.back}</Link>
  } else {
    const register = path === '/register', forgot = path === '/forgot'; title = register ? c.create : forgot ? c.reset : c.signIn
    body = <>{!forgot && capabilities.google && <><MotionButton className="secondary-button auth-google" busy={busy} aria-label={c.googleLabel} onClick={() => run(googleLogin)}><img className="auth-google-mark" src="/assets/google-g.png" alt="" width="20" height="20" /><span className="auth-google-label">{c.google}</span></MotionButton><div className="auth-or"><span>{c.or}</span></div></>}<form className="auth-form" onSubmit={submit}>{emailField}{!forgot && passwordField(register)}<MotionButton className="primary-button" type="submit" busy={busy} disabled={capabilities.enabled && (forgot || register) && !capabilities.email}>{forgot ? c.send : register ? c.create : c.signIn}</MotionButton></form><div className="auth-links"><Link to={register || forgot ? '/login' : '/register'}>{register || forgot ? c.back : c.create}</Link>{!forgot && <Link to="/forgot">{c.forgot}</Link>}</div>{(register || forgot) && capabilities.enabled && !capabilities.email && <p className="auth-note" role="status">{c.unavailable}</p>}</>
  }
  return <section className="auth-page"><MotionReveal identity={path} className="auth-panel"><h1>{title}</h1>{error && <MotionReveal identity={error} role="alert"><p ref={alert} tabIndex={-1} className="auth-error"><CircleAlert size={18} aria-hidden="true" /><span>{authErrorText(error, c)}</span></p></MotionReveal>}{body}{!['/account', '/terms', '/privacy', '/auth/device', '/auth/complete'].includes(path) && <MotionButton className="auth-text-button auth-offline" onClick={continueOffline}>{c.offline}</MotionButton>}{footer}</MotionReveal></section>
}
async function nativeDecision(review: { id: string; csrf: string }, accept: boolean) {
  // This consent uses its external-browser binding, never the native session.
  const { API_URL } = await import('../api/client')
  const response = await fetch(API_URL + '/v1/auth/native-confirm', { method: 'POST', cache: 'no-store', credentials: 'include', headers: { 'Content-Type': 'application/json', 'X-Pinelink-CSRF': review.csrf }, body: JSON.stringify({ request_id: review.id, accept }) })
  if (!response.ok) { const error = await response.json().catch(() => ({})) as { code?: string }; throw new Error(error.code ?? 'AUTH_UNAVAILABLE') }
  history.replaceState(null, '', accept ? '/auth/complete' : '/auth/complete?error=GOOGLE_CANCELLED')
}
