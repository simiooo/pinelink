import { FileText, FileUp, Paperclip, Send, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { InputGroup, InputGroupAddon, InputGroupTextarea } from '../components/ui/input-group'
import { Switch } from '../components/ui/switch'
import { MotionButton, MotionTooltip } from '../motion/components'
import { textByteLimit } from './crypto'
import './TextComposer.css'

export type TextSensitivity = 'NORMAL' | 'SENSITIVE'
export type TextComposerLabels = { sensitive: string; input: string; send: string; attach: string; removeAttachment: string; tooLong: string; invalidText: string; unsupported: string; sendFile: string; sensitiveFileWarning: string; confirm: string; cancel: string }
function validUnicode(value: string) {
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i)
    if (c >= 0xd800 && c <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false }
    else if (c >= 0xdc00 && c <= 0xdfff) return false
  }
  return true
}
export function TextComposer({ value, sensitivity, supported, attachment, busy = false, labels, onValue, onSensitivity, onAttachment, onSend, onSendFile }: {
  value: string; sensitivity: TextSensitivity; supported: boolean; attachment?: File; busy?: boolean; labels: TextComposerLabels
  onValue: (value: string) => void; onSensitivity: (value: TextSensitivity) => void; onAttachment: (file?: File) => void; onSend: (includeText: boolean) => void | Promise<void>; onSendFile: () => void | Promise<void>
}) {
  const [confirmValue, setConfirmValue] = useState<string>(), input = useRef<HTMLInputElement>(null)
  const valid = validUnicode(value), length = valid ? new TextEncoder().encode(value).length : 0
  const tooLong = length > textByteLimit, sendText = supported && valid && length > 0 && !tooLong
  const enabled = !busy && (!!attachment || sendText)
  const confirmFile = !attachment && confirmValue !== undefined && confirmValue === value && sensitivity === 'SENSITIVE' && (!supported || tooLong)
  return <InputGroup className="conversation-composer" aria-label={labels.input}>
    <InputGroupTextarea aria-label={labels.input} placeholder={labels.input} value={value} onChange={event => { setConfirmValue(undefined); onValue(event.target.value) }} />
    {attachment && <div className="conversation-attachment"><FileText size={18} aria-hidden="true" /><span title={attachment.name}>{attachment.name}</span><MotionTooltip label={labels.removeAttachment}><MotionButton icon className="guest-icon-button" disabled={busy} aria-label={labels.removeAttachment} onClick={() => onAttachment(undefined)}><X size={18} /></MotionButton></MotionTooltip></div>}
    {(!valid || tooLong || !supported && value.length > 0) && <span role="status" className="conversation-compose-status">{!valid ? labels.invalidText : tooLong ? labels.tooLong : labels.unsupported}</span>}
    <InputGroupAddon align="block-end" className="conversation-compose-actions">
      <input ref={input} type="file" hidden aria-label={labels.attach} onChange={event => { const chosen = event.target.files?.[0]; if (chosen) onAttachment(chosen); event.target.value = '' }} />
      <MotionTooltip label={labels.attach}><MotionButton icon className="guest-icon-button" disabled={busy} aria-label={labels.attach} onClick={() => input.current?.click()}><Paperclip size={20} /></MotionButton></MotionTooltip>
      <label className="conversation-sensitivity"><Switch checked={sensitivity === 'SENSITIVE'} disabled={busy} aria-label={labels.sensitive} onCheckedChange={checked => { setConfirmValue(undefined); onSensitivity(checked ? 'SENSITIVE' : 'NORMAL') }} /><span>{labels.sensitive}</span></label>
      {!attachment && (!supported || tooLong) && valid && value.length > 0 && !confirmFile && <MotionButton className="secondary-button conversation-fallback" disabled={busy} onClick={() => sensitivity === 'SENSITIVE' ? setConfirmValue(value) : onSendFile()}><FileUp size={18} />{labels.sendFile}</MotionButton>}
      <MotionTooltip label={labels.send}><MotionButton icon className="guest-icon-button conversation-send" disabled={!enabled} aria-label={labels.send} onClick={() => onSend(sendText)}><Send size={20} /></MotionButton></MotionTooltip>
    </InputGroupAddon>
    {confirmFile && <div className="conversation-file-confirm"><span>{labels.sensitiveFileWarning}</span><MotionButton disabled={busy} onClick={onSendFile}>{labels.confirm}</MotionButton><MotionButton onClick={() => setConfirmValue(undefined)}>{labels.cancel}</MotionButton></div>}
  </InputGroup>
}
