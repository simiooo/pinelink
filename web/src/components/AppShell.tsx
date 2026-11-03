import { useLayoutEffect, useRef, useState } from 'react'
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { Bell, Ellipsis } from 'lucide-react'
import { useI18n } from '../i18n'
import type { LocalePreference } from '../i18n'
import { MotionButton, MotionTooltip } from '../motion/components'
import { useMotionRef } from '../motion/react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from './ui/dropdown-menu'

export function AppShell() {
  const pathname = useRouterState({ select: state => state.location.pathname }), navigate = useNavigate()
  const { t, preference, setPreference } = useI18n(), [menuOpen, setMenuOpen] = useState(false)
  const [page, owner] = useMotionRef<HTMLElement>(), previous = useRef(pathname)
  useLayoutEffect(() => {
    if (previous.current === pathname || !page.current) return
    if (!owner.isAnimating(page.current)) owner.set(page.current, { x: pathname === '/' ? -16 : 16, opacity: .5 })
    void owner.to(page.current, { x: 0, opacity: 1 }, 280)
    previous.current = pathname; setMenuOpen(false)
  }, [pathname, owner, page])
  return <div className="app-shell">
    <header className="topbar">
      <Link to="/" className="brand" aria-label={t('brandHome')}>Pinelink</Link>
      <nav className="topbar-actions" aria-label={t('globalActions')}>
        {pathname === '/' && <MotionTooltip label={t('notifications')}><MotionButton icon aria-label={t('notifications')} onClick={() => navigate({ to: '/notifications' })}><Bell size={22} /></MotionButton></MotionTooltip>}
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild><MotionButton icon aria-label={t('more')}><Ellipsis size={24} /></MotionButton></DropdownMenuTrigger>
          <DropdownMenuContent align="end" sideOffset={8} className="pinelink-menu">
            <DropdownMenuItem onSelect={() => { void navigate({ to: '/tasks' }) }}>{t('currentTasks')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => { void navigate({ to: '/history' }) }}>{t('history')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => { void navigate({ to: '/settings' }) }}>{t('settings')}</DropdownMenuItem>
            {pathname !== '/' && pathname !== '/notifications' && <DropdownMenuItem onSelect={() => { void navigate({ to: '/notifications' }) }}>{t('fileNotifications')}</DropdownMenuItem>}
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
    <main ref={page} className="page-content"><Outlet /></main>
  </div>
}
