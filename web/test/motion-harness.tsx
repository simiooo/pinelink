// Development-only entry; Vite's production build includes index.html only.
import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { MoreHorizontal } from 'lucide-react'
import { MotionProvider, useMotion } from '../src/motion/react'
import { MotionButton, MotionList, MotionMeter, MotionTooltip } from '../src/motion/components'
import { MotionDropzone } from '../src/motion/dropzone'
import { TransferGraph } from '../src/motion/graph'
import { MotionSurface } from '../src/motion/surface'
import { Input } from '../src/components/ui/input'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '../src/components/ui/dropdown-menu'
import '../src/styles/ui.css'
import '../src/styles/app.css'
import '../src/motion/motion.css'

let actions = 0, drops = 0, release: (() => void) | undefined
function Harness() {
  const owner = useMotion(), [open, setOpen] = useState(false), [value, setValue] = useState(.42)
  const [running, setRunning] = useState(true), [completed, setCompleted] = useState(false), [rows, setRows] = useState(['alpha', 'beta', 'gamma'])
  useEffect(() => {
    Object.assign(window, { motionFixture: {
      owner, open: setOpen, value: setValue, running: setRunning, complete: setCompleted,
      rows: setRows, actions: () => actions, drops: () => drops, release: () => release?.(),
    } })
  }, [owner])
  return <main style={{ maxWidth: 600, margin: '32px auto', padding: 20 }}>
    <h1>Motion integration</h1>
    <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
      <MotionButton id="async-action" className="primary-button" onClick={() => { actions++; return new Promise<void>(resolve => { release = resolve }) }}>Send</MotionButton>
      <MotionButton id="open-surface" onClick={() => setOpen(true)}>Details</MotionButton>
      <DropdownMenu><DropdownMenuTrigger asChild><MotionButton icon aria-label="More"><MoreHorizontal /></MotionButton></DropdownMenuTrigger>
        <DropdownMenuContent className="pinelink-menu"><DropdownMenuItem>History</DropdownMenuItem><DropdownMenuItem>Settings</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
      <MotionTooltip label="More information"><MotionButton icon aria-label="Info">i</MotionButton></MotionTooltip>
    </div>
    <TransferGraph value={value} running={running} completed={completed} cloud={false} incoming localLabel="This device" peerLabel="Alex" detailsLabel="Transfer details" progressLabel="Confirmed bytes" statusLabel={completed ? 'Delivered' : running ? 'Receiving' : 'Paused'} onDetails={() => setOpen(true)} />
    <MotionMeter value={value} label="Quota" />
    <MotionDropzone onFile={() => { drops++ }}><div className="upload-icon">↓</div><span id="drop-child">Drop files</span></MotionDropzone>
    <MotionList identity={rows.join(',')}>{rows.map(row => <label key={row} data-motion-key={row}>{row}<Input aria-label={row} defaultValue={row} /></label>)}</MotionList>
    <MotionSurface open={open} onClose={() => setOpen(false)} title="Alex" closeLabel="Close"><Input aria-label="Draft" defaultValue="Keep this draft" /><p style={{ marginTop: 24 }}>Transfer details</p></MotionSurface>
  </main>
}
const root = createRoot(document.getElementById('root')!)
root.render(<StrictMode><MotionProvider><Harness /></MotionProvider></StrictMode>)
Object.assign(window, { unmountMotionFixture: () => root.unmount() })
