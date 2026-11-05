import { useEffect, useState, useSyncExternalStore } from 'react'
import { X } from 'lucide-react'
import { applyAppUpdate, dismissUpdateNotice, pwaSnapshot, subscribePwa } from '../pwa'
import { safeToUpdate } from '../pwa/guard'
import { useI18n } from '../i18n'
import { MotionButton, MotionTooltip } from '../motion/components'
import { Toast, ToastClose, ToastDescription, ToastProvider, ToastViewport } from './ui/toast'
import './AppUpdate.css'

export function useUpdateCopy() {
  const { locale } = useI18n()
  return locale === 'zh-CN' ? {
    check: '检查更新', version: '版本', update: '更新', close: '关闭', available: '有新版本', busy: '完成当前操作后可更新', other: '其他页面尚未就绪，请稍后重试', legacy: '请先关闭其他旧页面，再重试更新', applying: '正在更新…', checking: '正在检查更新…', latest: '已是最新版本', offline: '联网后可检查更新', error: '暂时无法更新，请重试',
  } : {
    check: 'Check for updates', version: 'Version', update: 'Update', close: 'Close', available: 'New version available', busy: 'Finish the current action to update', other: 'Another page is busy. Try again later.', legacy: 'Close other older pages, then try again.', applying: 'Updating…', checking: 'Checking for updates…', latest: 'You’re up to date', offline: 'Go online to check for updates', error: 'Could not update. Try again.',
  }
}
export function AppUpdate() {
  const state = useSyncExternalStore(subscribePwa, pwaSnapshot), c = useUpdateCopy(), [page, setPage] = useState(() => ({ safe: safeToUpdate(), dialog: false, selected: false }))
  useEffect(() => {
    const refresh = () => {
      const next = { safe: safeToUpdate(), dialog: !!document.querySelector('[role=dialog]:not([data-state=closed]):not([aria-hidden=true]):not([hidden])'), selected: !!document.querySelector('.guest-network[data-selected]') }
      setPage(previous => previous.safe === next.safe && previous.dialog === next.dialog && previous.selected === next.selected ? previous : next)
    }
    let frame = 0
    const interacted = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(refresh) }
    const timer = setInterval(refresh, 500)
    const observer = new MutationObserver(refresh)
    observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['data-selected', 'data-state', 'aria-hidden', 'hidden'] })
    refresh()
    document.addEventListener('click', interacted); document.addEventListener('focusin', interacted)
    return () => { observer.disconnect(); clearInterval(timer); cancelAnimationFrame(frame); document.removeEventListener('click', interacted); document.removeEventListener('focusin', interacted) }
  }, [])
  const safe = page.safe
  if (state.status === 'idle' || !state.locked && (page.dialog || page.selected && !state.explicit)) return null
  const text = state.status === 'available' ? safe ? c.available : c.busy : state.status === 'busy' ? safe ? c.other : c.busy : c[state.status]
  return <ToastProvider label={c.check} duration={8000}>
    <Toast key={state.notice} className="app-update-notice" aria-label={c.check} open
      duration={['latest', 'offline', 'error'].includes(state.status) ? 8000 : Infinity}
      onOpenChange={open => { if (!open) dismissUpdateNotice(state.notice) }}
      onEscapeKeyDown={event => { if (state.locked) event.preventDefault() }}
      onSwipeEnd={event => { if (state.locked) event.preventDefault() }}>
    <ToastDescription asChild><span>{text}</span></ToastDescription>
    {state.waiting && state.status !== 'applying' && <MotionButton className="primary-button app-update-action" disabled={!safe} onClick={() => void applyAppUpdate()}>{c.update}</MotionButton>}
    {!state.locked && <MotionTooltip label={c.close}><ToastClose asChild><MotionButton icon className="app-update-close" aria-label={c.close}><X size={16} /></MotionButton></ToastClose></MotionTooltip>}
    </Toast>
    <ToastViewport className="app-update-viewport" label={c.check} />
  </ToastProvider>
}
