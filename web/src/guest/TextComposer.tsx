import { FileUp, Send } from 'lucide-react'
import { useState } from 'react'
import { MotionButton, MotionTooltip } from '../motion/components'
import { textByteLimit } from './crypto'
import './TextComposer.css'

export type TextSensitivity = 'NORMAL' | 'SENSITIVE'
export type TextComposerLabels = { text: string; sensitive: string; input: string; send: string; tooLong: string; invalidText: string; unsupported: string; sendFile: string; sensitiveFileWarning: string; confirm: string; cancel: string }
function validUnicode(value: string) {
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i)
    if (c >= 0xd800 && c <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false }
    else if (c >= 0xdc00 && c <= 0xdfff) return false
  }
  return true
}
export function TextComposer({ value, sensitivity, supported, busy = false, labels, onValue, onSensitivity, onSend, onSendFile }: {
  value: string; sensitivity: TextSensitivity; supported: boolean; busy?: boolean; labels: TextComposerLabels
  onValue: (value: string) => void; onSensitivity: (value: TextSensitivity) => void; onSend: () => void | Promise<void>; onSendFile: () => void | Promise<void>
}) {
  const [confirmValue, setConfirmValue] = useState<string>()
  const valid = validUnicode(value), length = valid ? new TextEncoder().encode(value).length : 0
  const tooLong = length > textByteLimit, enabled = supported && valid && length > 0 && !tooLong && !busy
  const confirmFile = confirmValue !== undefined && confirmValue === value && sensitivity === 'SENSITIVE' && (!supported || tooLong)
  return <div className="conversation-composer">
    <div className="conversation-modes" role="group" aria-label={labels.input}>
      <button type="button" aria-pressed={sensitivity === 'NORMAL'} onClick={() => { setConfirmValue(undefined); onSensitivity('NORMAL') }}>{labels.text}</button>
      <button type="button" aria-pressed={sensitivity === 'SENSITIVE'} onClick={() => { setConfirmValue(undefined); onSensitivity('SENSITIVE') }}>{labels.sensitive}</button>
    </div>
    <textarea aria-label={labels.input} value={value} onChange={event => { setConfirmValue(undefined); onValue(event.target.value) }} />
    {(!valid || tooLong || !supported) && <span role="status" className="conversation-compose-status">{!valid ? labels.invalidText : tooLong ? labels.tooLong : labels.unsupported}</span>}
    <div className="conversation-compose-actions">
      {(!supported || tooLong) && valid && value.length > 0 && !confirmFile && <MotionButton className="secondary-button" disabled={busy} onClick={() => sensitivity === 'SENSITIVE' ? setConfirmValue(value) : onSendFile()}><FileUp size={18} />{labels.sendFile}</MotionButton>}
      <MotionTooltip label={labels.send}><MotionButton icon className="guest-icon-button" disabled={!enabled} aria-label={labels.send} onClick={onSend}><Send size={20} /></MotionButton></MotionTooltip>
    </div>
    {confirmFile && <div className="conversation-file-confirm"><span>{labels.sensitiveFileWarning}</span><MotionButton disabled={busy} onClick={onSendFile}>{labels.confirm}</MotionButton><MotionButton onClick={() => setConfirmValue(undefined)}>{labels.cancel}</MotionButton></div>}
  </div>
}
