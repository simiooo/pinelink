import { useEffect, useRef, useState } from 'react'
import { FileUp, ScanLine } from 'lucide-react'
import { MotionButton, MotionTooltip } from '../motion/components'
import { useGuestCopy } from './copy'
import { useLocalCopy } from '../local/copy'

export function ConnectionEntry({ value, onValue, onSubmit, onError }: { value: string; onValue: (value: string) => void; onSubmit: (value: string) => Promise<void>; onError: (error: string) => void }) {
  const c = useGuestCopy(), lc = useLocalCopy(), input = useRef<HTMLInputElement>(null), video = useRef<HTMLVideoElement>(null)
  const [camera, setCamera] = useState(false), [busy, setBusy] = useState(false), alive = useRef(true), busyRef = useRef(false)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const submit = async (code: string) => {
    if (busyRef.current) return
    busyRef.current = true; setBusy(true)
    try { await onSubmit(code) } finally { busyRef.current = false; if (alive.current) setBusy(false) }
  }
  useEffect(() => {
    if (!camera) return
    let stopped = false, stream: MediaStream | undefined, timer: ReturnType<typeof setTimeout> | undefined
    const stop = () => { stopped = true; clearTimeout(timer); stream?.getTracks().forEach(track => track.stop()); if (video.current) video.current.srcObject = null }
    void (async () => {
      try {
        const scanner = await import('jsqr')
        stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } }, audio: false })
        if (stopped) { stop(); return }
        const element = video.current; if (!element) { stop(); return }
        element.srcObject = stream; await element.play()
        const canvas = document.createElement('canvas'), context = canvas.getContext('2d', { willReadFrequently: true })!
        const tick = () => {
          if (stopped) return
          if (element.readyState >= 2) {
            const scale = Math.min(1, 720 / Math.max(element.videoWidth, element.videoHeight)); canvas.width = Math.round(element.videoWidth * scale); canvas.height = Math.round(element.videoHeight * scale)
            context.drawImage(element, 0, 0, canvas.width, canvas.height)
            const result = scanner.default(context.getImageData(0, 0, canvas.width, canvas.height).data, canvas.width, canvas.height, { inversionAttempts: 'dontInvert' })
            if (result) { stop(); setCamera(false); onValue(result.data); void submit(result.data); return }
          }
          timer = setTimeout(tick, 200)
        }
        tick()
      } catch { stop(); if (alive.current) { setCamera(false); onError('LOCAL_CAMERA_UNAVAILABLE') } }
    })()
    return stop
  // The scanner captures the callbacks from the render that opened it.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [camera])
  return <div className="connection-entry">
    <input ref={input} hidden type="file" accept=".pinelink,text/plain" aria-label={lc.import} onChange={event => { const file = event.target.files?.[0]; event.target.value = ''; if (!file) return; if (file.size > 18000) { onError('LOCAL_CODE_INVALID'); return }; void file.text().then(code => { onValue(code); return submit(code) }).catch(() => onError('LOCAL_CODE_INVALID')) }} />
    {camera ? <div className="local-camera"><video ref={video} autoPlay playsInline muted aria-label={lc.scan} /><MotionButton className="secondary-button guest-wide" onClick={() => setCamera(false)}>{c.cancel}</MotionButton></div> : <form className="guest-code-form" onSubmit={event => { event.preventDefault(); void submit(value) }}>
      <label>{c.code}<textarea className="connection-input" value={value} onChange={event => onValue(event.target.value)} placeholder={c.codePlaceholder} maxLength={18000} spellCheck={false} autoComplete="off" autoCapitalize="none" required /></label>
      <MotionButton type="submit" className="primary-button guest-wide" disabled={!value.trim()} busy={busy}>{c.connectAction}</MotionButton>
      <div className="local-tools"><MotionTooltip label={lc.scan}><MotionButton type="button" icon aria-label={lc.scan} disabled={busy} onClick={() => setCamera(true)}><ScanLine /></MotionButton></MotionTooltip><MotionTooltip label={lc.import}><MotionButton type="button" icon aria-label={lc.import} disabled={busy} onClick={() => input.current?.click()}><FileUp /></MotionButton></MotionTooltip></div>
    </form>}
  </div>
}
