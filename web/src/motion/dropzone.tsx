import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useMotionRef } from './react'

// A file enters the preparation flow only on an explicit drop or picker result.
export function MotionDropzone({ children, onFile }: { children: ReactNode; onFile: (file: File) => void }) {
  const depth = useRef(0), [active, setActive] = useState(false)
  const [ref, owner] = useMotionRef<HTMLDivElement>()
  const clear = () => { depth.current = 0; setActive(false) }
  useEffect(() => {
    if (!active) return
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') clear() }
    const outside = (event: DragEvent) => { if (!ref.current?.contains(event.target as Node)) clear() }
    window.addEventListener('keydown', escape); window.addEventListener('dragend', clear); window.addEventListener('drop', outside)
    return () => { window.removeEventListener('keydown', escape); window.removeEventListener('dragend', clear); window.removeEventListener('drop', outside) }
  }, [active, ref])
  useEffect(() => { const icon = ref.current?.querySelector<HTMLElement>('.upload-icon'); return () => { if (icon) owner.forget(icon) } }, [owner, ref])
  useEffect(() => {
    const icon = ref.current?.querySelector<HTMLElement>('.upload-icon')
    if (!icon) return
    void owner.to(icon, { y: active ? -4 : 0, scale: active ? 1.04 : 1 }, 160)
  }, [active, owner, ref])
  const accepts = (types: readonly string[]) => types.includes('Files')
  return <div ref={ref} className={`dropzone ${active ? 'dragging' : ''}`} data-drop-ready={active}
    onDragEnter={event => { if (accepts(event.dataTransfer.types)) { event.preventDefault(); depth.current++; setActive(true) } }}
    onDragOver={event => { if (accepts(event.dataTransfer.types)) { event.preventDefault(); event.dataTransfer.dropEffect = 'copy'; setActive(true) } }}
    onDragLeave={() => { if (--depth.current <= 0) clear() }}
    onDrop={event => { if (accepts(event.dataTransfer.types)) { event.preventDefault(); const file = event.dataTransfer.files[0]; clear(); if (file) onFile(file) } }}
  >{children}</div>
}
