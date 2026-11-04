import { useEffect, useState } from 'react'
import Markdown from 'react-markdown'
import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import { MotionButton, MotionTooltip } from '../motion/components'
import './TextBubble.css'

export type TextBubbleLabels = { copy: string; copied: string; copyFailed: string; sensitive: string; reveal: string; hide: string }

// The raw value is copied; Markdown only controls presentation. Sensitive text
// remains hidden by default and never causes an automatic clipboard write.
export function TextBubble({ id, value, sensitive, labels, direction = 'receive' }: { id: string; value: string; sensitive: boolean; labels: TextBubbleLabels; direction?: 'send' | 'receive' }) {
  const [revealedId, setRevealedId] = useState<string>(), [notice, setNotice] = useState<{ id: string; text: string }>()
  useEffect(() => { if (!notice) return; const timer = setTimeout(() => setNotice(undefined), 2000); return () => clearTimeout(timer) }, [notice])
  const revealed = sensitive && revealedId === id
  const copy = async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('CLIPBOARD_UNAVAILABLE')
      await navigator.clipboard.writeText(value); setNotice({ id, text: labels.copied })
    } catch { setNotice({ id, text: labels.copyFailed }) }
  }
  const shown = !sensitive || revealed
  return <div className={`conversation-message conversation-message--${direction}`}>
    <div className="conversation-text">
    {sensitive && <span className="conversation-sensitive">{labels.sensitive}</span>}
    {shown ? <div className="conversation-markdown"><Markdown skipHtml disallowedElements={['img']} components={{
      a: ({ href, children }) => {
        let safe: string | undefined
        try { const url = new URL(href ?? ''); if (['https:', 'http:'].includes(url.protocol)) safe = url.href } catch { /* Relative and malformed links are plain text. */ }
        return safe ? <a href={safe} target="_blank" rel="noopener noreferrer">{children}</a> : <span>{children}</span>
      },
    }}>{value}</Markdown></div> : <div className="conversation-masked" aria-label={labels.sensitive}>••••••••••••</div>}
    </div>
    <div className="conversation-text-actions">
      {sensitive && <MotionTooltip label={revealed ? labels.hide : labels.reveal}><MotionButton icon className="guest-icon-button" aria-label={revealed ? labels.hide : labels.reveal} onClick={() => setRevealedId(revealed ? undefined : id)}>{revealed ? <EyeOff size={18} /> : <Eye size={18} />}</MotionButton></MotionTooltip>}
      <MotionTooltip label={labels.copy}><MotionButton icon className="guest-icon-button" aria-label={labels.copy} onClick={copy}>{notice?.id === id && notice.text === labels.copied ? <Check size={18} /> : <Copy size={18} />}</MotionButton></MotionTooltip>
    </div>
    {notice?.id === id && <span className="conversation-copy-status" role="status">{notice.text}</span>}
  </div>
}
