import { useLayoutEffect, useRef, useState } from 'react'
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { Bell, Ellipsis, FolderOpen, Network, UserRound } from 'lucide-react'
import { useAuthCopy } from '../auth/copy'
import { authRevision, authState, subscribeAuth } from '../auth/client'
import { useSyncExternalStore } from 'react'
import { guestContinuityRevision, guestWorkPending, openGuestSignIn, subscribeGuestContinuity } from '../guest/continuity'
import { useGuestCopy } from '../guest/copy'
import { GuestHome } from '../guest/Home'
import { workspacePaths } from '../guest/navigation'
import { clientRuntime } from '../data'
import { useI18n } from '../i18n'
import type { LocalePreference } from '../i18n'
import { MotionButton, MotionTooltip } from '../motion/components'
import { useMotionRef } from '../motion/react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from './ui/dropdown-menu'

export function AppShell() {
  const accountCopy = useAuthCopy()
  const guestCopy = useGuestCopy()
  useSyncExternalStore(subscribeAuth, authRevision)
  useSyncExternalStore(subscribeGuestContinuity, guestContinuityRevision)
  const account = authState()
  const pathname = useRouterState({ select: state => state.location.pathname }), navigate = useNavigate()
  const { t, preference, setPreference } = useI18n(), [menuOpen, setMenuOpen] = useState(false)
  const [notifications, setNotifications] = useState(0)
  const [page, owner] = useMotionRef<HTMLElement>(), previous = useRef(pathname)
  const refreshAccountPage = (to: string) => {
    if (!account.session || guestWorkPending()) return false
    let current
    try { current = clientRuntime() } catch { /* Navigation can retry account initialization. */ }
    if (current?.owner === account.session.account.id && current.profile.device === account.session.device_id && current.access.csrf === account.session.csrf) return false
    // The guest window deliberately kept its own runtime during sign-in.
    // Enter the account profile on explicit navigation after that work finishes.
    location.assign(to); return true
  }
  const go = (to: '/' | '/account' | '/files' | '/settings' | '/notifications') => { if (workspacePaths.has(to) || !refreshAccountPage(to)) void navigate({ to }) }
  useLayoutEffect(() => {
    if (previous.current === pathname || !page.current) return
    if (workspacePaths.has(previous.current) && workspacePaths.has(pathname)) { previous.current = pathname; setMenuOpen(false); return }
    if (!owner.isAnimating(page.current)) owner.set(page.current, { x: pathname === '/' ? -16 : 16, opacity: .5 })
    void owner.to(page.current, { x: 0, opacity: 1 }, 280)
    previous.current = pathname; setMenuOpen(false)
  }, [pathname, owner, page])
  return <div className="app-shell">
    <header className="topbar">
      <Link to="/" className="brand" aria-label={t('brandHome')}>Pinelink</Link>
      <nav className="topbar-actions" aria-label={t('globalActions')}>
        {workspacePaths.has(pathname) && <MotionTooltip label={t('notifications')}><MotionButton icon className="workspace-notifications" aria-label={t('notifications')} onClick={() => navigate({ to: '/notifications' })}><Bell size={22} />{notifications > 0 && <span className="workspace-unread" aria-label={guestCopy.unread} />}</MotionButton></MotionTooltip>}
        {account.capabilities.enabled && !pathname.startsWith('/auth') && !['/login', '/register', '/forgot', '/verify', '/reset', '/verify-link'].includes(pathname) && <MotionTooltip label={account.session ? accountCopy.account : accountCopy.signIn}><MotionButton icon={!!account.session} className={account.session ? 'secondary-button' : 'auth-text-button'} aria-label={account.session ? accountCopy.account : accountCopy.signIn} onClick={() => { if (!account.session) openGuestSignIn(); else if (guestWorkPending()) window.open('/account','pinelink-account'); else location.assign('/account') }}>{account.session ? <UserRound size={21} /> : accountCopy.signIn}</MotionButton></MotionTooltip>}
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild><MotionButton icon aria-label={t('more')}><Ellipsis size={24} /></MotionButton></DropdownMenuTrigger>
          <DropdownMenuContent align="end" sideOffset={8} className="pinelink-menu">
            <DropdownMenuItem onSelect={() => { if (guestWorkPending()) { if (account.session) window.open('/account','pinelink-account'); else openGuestSignIn() } else go('/account') }}><UserRound size={18} />{accountCopy.account}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => go('/')}><Network size={18} />{guestCopy.devices}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => go('/files')}><FolderOpen size={18} />{guestCopy.files}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => go('/settings')}>{t('settings')}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuLabel>{t('language')}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={preference} onValueChange={value => setPreference(value as LocalePreference)}>
              <DropdownMenuRadioItem value="system">{t('languageSystem')}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="zh-CN">{t('languageZh')}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="en-US">{t('languageEn')}</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </nav>
    </header>
    <main ref={page} className="page-content">{workspacePaths.has(pathname) ? <GuestHome onNotifications={setNotifications} /> : <Outlet />}</main>
  </div>
}
