import { useEffect, useRef, useState } from 'react'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { useI18n } from '../i18n'
import type { LocalePreference } from '../i18n'

export function AppShell() {
  const pathname = useRouterState({ select: state => state.location.pathname })
  const home = pathname === '/'
  const menu = useRef<HTMLDetailsElement>(null)
  const { t, preference, setPreference } = useI18n()
  const [languageSaved, setLanguageSaved] = useState(false)
  useEffect(() => { menu.current?.removeAttribute('open') }, [pathname])
  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (menu.current?.open && !menu.current.contains(event.target as Node)) menu.current.removeAttribute('open')
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && menu.current?.open) {
        menu.current.removeAttribute('open')
        menu.current.querySelector('summary')?.focus()
      }
    }
    document.addEventListener('click', close)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('click', close); document.removeEventListener('keydown', escape) }
  }, [])
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link to="/" className="brand" aria-label={t('brandHome')}>Pinelink</Link>
        <nav className="topbar-actions" aria-label={t('globalActions')}>
          {home && <Link to="/notifications" className="chip">{t('notifications')}</Link>}
          <details className="more-menu" ref={menu}>
            <summary className="chip" role="button">{t('more')}</summary>
            <div className="menu-popover">
              <Link to="/tasks">{t('currentTasks')}</Link>
              <Link to="/history">{t('history')}</Link>
              <Link to="/settings">{t('settings')}</Link>
              {!home && <Link to="/notifications">{t('fileNotifications')}</Link>}
              <div className="menu-divider" />
              <label className="language-picker">{t('language')}
                <select aria-label={t('language')} value={preference} onChange={event => { setPreference(event.target.value as LocalePreference); setLanguageSaved(true) }}>
                  <option value="system">{t('languageSystem')}</option>
                  <option value="zh-CN">{t('languageZh')}</option>
                  <option value="en-US">{t('languageEn')}</option>
                </select>
              </label>
              {languageSaved && <p className="language-note" role="status">{t('languageSaved')}</p>}
            </div>
          </details>
        </nav>
      </header>
      <main className="page-content"><Outlet /></main>
    </div>
  )
}
