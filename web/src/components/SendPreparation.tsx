import { Input } from './ui/input'
import { MotionButton, MotionReveal } from '../motion/components'
import { useEffect, useRef, useState } from 'react'
import { formatBytes } from '../format'
import { useI18n } from '../i18n'
import { FileIcon } from './FileIcon'

const identityPattern = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/
const quote = (value: string) => `'${value.replaceAll("'", "''")}'`

export function SendPreparation({ name, size }: { name: string; size: number }) {
  const { t, locale } = useI18n()
  const [owner, setOwner] = useState('')
  const [device, setDevice] = useState('')
  const [path, setPath] = useState(name)
  const [notice, setNotice] = useState('')
  const [copied, setCopied] = useState(false)
  const valid = identityPattern.test(owner) && identityPattern.test(device) && path.trim().length > 0 && !/[\r\n\0]/.test(path)
  const command = valid ? `pinelink p2p send ${quote(path.trim())} --to-owner ${quote(owner)} --to-device ${quote(device)} --path auto` : ''
  const current = useRef(command), alive = useRef(true)
  current.current = command
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const edit = (update: () => void) => { update(); setCopied(false); setNotice('') }

  async function copy() {
    if (!valid) { setNotice(t('prepareInvalid')); return }
    const captured = command
    try {
      await navigator.clipboard.writeText(captured)
      if (!alive.current || current.current !== captured) return
      setCopied(true)
      setNotice(t('copySuccess'))
    } catch {
      if (!alive.current || current.current !== captured) return
      setNotice(t('copyFailed'))
    }
  }

  return (
    <section aria-labelledby="prepare-title">
      <div className="page-heading"><h1 id="prepare-title">{t('prepareTitle')}</h1><p>{t('prepareSubtitle')}</p></div>
      <div className="file-summary preparation-file"><FileIcon extension={name.split('.').pop()} /><div className="file-summary-copy"><strong>{name}</strong><span>{formatBytes(size, locale)}</span></div></div>
      <div className="preparation-form">
        <label>{t('recipientOwner')}<Input value={owner} onChange={event => edit(() => setOwner(event.target.value))} autoComplete="off" placeholder={t('recipientOwnerHint')} /></label>
        <label>{t('recipientDevice')}<Input value={device} onChange={event => edit(() => setDevice(event.target.value))} autoComplete="off" placeholder={t('recipientDeviceHint')} /></label>
        <label>{t('filePath')}<Input value={path} onChange={event => edit(() => setPath(event.target.value))} autoComplete="off" /></label>
        <p>{t('prepareNote')}</p>
        <MotionButton className="primary-button" onClick={copy}>{t('copyCommand')}</MotionButton>
        {notice && <MotionReveal className="feedback" role="status" identity={notice}>{notice}</MotionReveal>}
        {(copied || command) && <textarea className="command-preview" aria-label={t('commandLabel')} readOnly value={command} rows={3} onFocus={event => event.target.select()} />}
      </div>
    </section>
  )
}
