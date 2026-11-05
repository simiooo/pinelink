import { useEffect, useRef, useState } from 'react'
import { Check, Copy, Info, RefreshCw } from 'lucide-react'
import { Button } from '../components/ui/button'
import { MotionButton, MotionReveal, MotionTooltip } from '../motion/components'
import { MotionSurface } from '../motion/surface'
import { appVersion } from '../pwa'
import { useI18n } from '../i18n'
import { browserDiagnostics, connectionReason } from './connection-diagnostics'
import type { ConnectionFault } from './connection-diagnostics'

export function ConnectionNotice({ message, visible, faults, retry, diagnostics }: { message: string; visible: boolean; faults: ConnectionFault[]; retry: () => Promise<void>; diagnostics: () => Promise<unknown> }) {
  const zh = useI18n().locale === 'zh-CN', [open, setOpen] = useState(false), [selected, setSelected] = useState<ConnectionFault[]>([]), [report, setReport] = useState(''), [copied, setCopied] = useState(false), [copyFailed, setCopyFailed] = useState(false)
  const read = useRef(diagnostics); read.current = diagnostics
  const failures = useRef(selected); failures.current = selected
  const c = zh ? { title: '连接状态', details: '查看连接状态', copy: '复制诊断', copied: '已复制', retry: '重试', close: '关闭', raw: '诊断信息', fallback: '请选择诊断文字复制', visitor: '访客连接', account: '账号连接' } : { title: 'Connection status', details: 'View connection status', copy: 'Copy diagnostics', copied: 'Copied', retry: 'Try again', close: 'Close', raw: 'Diagnostics', fallback: 'Select the diagnostic text to copy', visitor: 'Visitor connection', account: 'Account connection' }
  const stages = zh ? { session: '恢复会话', challenge: '申请会话', enroll: '验证会话', sign: '请求签名', connections: '设备列表', transfers: '传输列表', progress: '传输历史', request: '服务请求', 'local-data': '本机数据', initialization: '初始化' } : { session: 'Session recovery', challenge: 'Session challenge', enroll: 'Session enrollment', sign: 'Request signing', connections: 'Device list', transfers: 'Transfer list', progress: 'Transfer history', request: 'Service request', 'local-data': 'Local data', initialization: 'Initialization' }
  useEffect(() => {
    if (!open) return
    let disposed = false
    setReport(''); setCopied(false); setCopyFailed(false)
    const base = { schema: 'pinelink-connection-v1', version: appVersion, browser: browserDiagnostics(), failures: failures.current }
    const timer = setTimeout(() => { if (!disposed) setReport(JSON.stringify({ ...base, diagnosticRead: 'timeout' }, null, 2)) }, 3000)
    void read.current().then(result => { if (!disposed) setReport(JSON.stringify({ ...base, ...result as object }, null, 2)) }).catch(() => { if (!disposed) setReport(JSON.stringify({ ...base, diagnosticRead: 'unavailable' }, null, 2)) }).finally(() => clearTimeout(timer))
    return () => { disposed = true; clearTimeout(timer) }
  }, [open])
  const copy = async () => { try { await navigator.clipboard.writeText(report); setCopied(true) } catch { setCopyFailed(true) } }
  return <>
    {message && visible && !open && <MotionReveal role="alert" className="guest-error connection-notice" distance={0}>
      <Button variant="ghost" className="connection-notice-label" aria-label={c.details} onClick={() => { setSelected(faults); setOpen(true) }}><span>{message}</span><Info size={14} aria-hidden="true" /></Button>
      <MotionTooltip label={c.retry}><MotionButton icon className="guest-icon-button connection-notice-retry" aria-label={c.retry} onClick={retry}><RefreshCw size={18} /></MotionButton></MotionTooltip>
    </MotionReveal>}
    <MotionSurface open={open} onClose={() => setOpen(false)} title={c.title} closeLabel={c.close}>
      <div className="connection-details">
        {selected.map((fault, i) => <div className="connection-fault" key={i}><span>{c[fault.channel]} · {stages[fault.stage]}</span><strong>{connectionReason(fault, zh)}</strong><code>{fault.code}{fault.status ? ` · HTTP ${fault.status}` : ''}</code></div>)}
        <details className="connection-raw" open={copyFailed || undefined}><summary>{c.raw}</summary><pre tabIndex={0}>{report}</pre></details>
        {copyFailed && <p role="status">{c.fallback}</p>}
        <div className="connection-details-actions"><MotionTooltip label={copied ? c.copied : c.copy}><MotionButton icon disabled={!report} aria-label={copied ? c.copied : c.copy} onClick={copy}>{copied ? <Check size={20} /> : <Copy size={20} />}</MotionButton></MotionTooltip><MotionTooltip label={c.retry}><MotionButton icon aria-label={c.retry} onClick={retry}><RefreshCw size={20} /></MotionButton></MotionTooltip></div>
      </div>
    </MotionSurface>
  </>
}
