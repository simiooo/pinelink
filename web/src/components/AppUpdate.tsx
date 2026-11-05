import { useEffect, useState, useSyncExternalStore } from 'react'
import { X } from 'lucide-react'
import { applyAppUpdate, dismissUpdateNotice, pwaSnapshot, subscribePwa } from '../pwa'
import { safeToUpdate } from '../pwa/guard'
import { useI18n } from '../i18n'
import { MotionButton, MotionTooltip } from '../motion/components'
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
  const state = useSyncExternalStore(subscribePwa, pwaSnapshot), c = useUpdateCopy(), [page, setPage] = useState(() => ({ safe: safeToUpdate(), immersed: false }))
  useEffect(() => {
    const refresh = () => { const next = { safe: safeToUpdate(), immersed: !!document.querySelector('[role=dialog],.guest-network[data-selected]') }; setPage(previous => previous.safe === next.safe && previous.immersed === next.immersed ? previous : next) }
    let frame = 0
    const interacted = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(refresh) }
    const timer = setInterval(refresh, 500)
    document.addEventListener('click', interacted); document.addEventListener('focusin', interacted)
    return () => { clearInterval(timer); cancelAnimationFrame(frame); document.removeEventListener('click', interacted); document.removeEventListener('focusin', interacted) }
  }, [])
  useEffect(() => {
    if (!['latest', 'offline', 'error'].includes(state.status)) return
    const timer = setTimeout(dismissUpdateNotice, 8000); return () => clearTimeout(timer)
  }, [state.status])
  const safe = page.safe
  if (state.status === 'idle' || page.immersed && !state.locked) return null
  const text = state.status === 'available' ? safe ? c.available : c.busy : state.status === 'busy' ? safe ? c.other : c.busy : c[state.status]
  return <aside className="app-update-notice" aria-label={c.check}>
    <span role="status">{text}</span>
    {state.waiting && state.status !== 'applying' && <MotionButton className="primary-button app-update-action" disabled={!safe} onClick={() => void applyAppUpdate()}>{c.update}</MotionButton>}
    {!state.locked && <MotionTooltip label={c.close}><MotionButton icon className="app-update-close" aria-label={c.close} onClick={dismissUpdateNotice}><X size={16} /></MotionButton></MotionTooltip>}
  </aside>
}
