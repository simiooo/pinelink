import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { Clock3, Copy, Download, FileUp, ScanLine, ShieldCheck } from 'lucide-react'
import QRCode from 'qrcode'
import { MotionButton, MotionSpinner, MotionTooltip } from '../motion/components'
import type { LocalRuntime } from './runtime'
import { localError, useLocalCopy } from './copy'

export function LocalConnect({runtime,onConnected,onJoin,onEnter,onNewCode,preparing=false,onlineCode}:{runtime:LocalRuntime;onConnected:()=>void;onJoin?:(code:string)=>Promise<void>;onEnter?:()=>void;onNewCode?:()=>void;preparing?:boolean;onlineCode?:string}){
 const c=useLocalCopy(),state=useSyncExternalStore(runtime.subscribe,runtime.getSnapshot),[enter,setEnter]=useState(false),[value,setValue]=useState(''),[qr,setQr]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState(''),[camera,setCamera]=useState(false),video=useRef<HTMLVideoElement>(null),file=useRef<HTMLInputElement>(null),alive=useRef(true)
 useEffect(()=>{alive.current=true;return()=>{alive.current=false}},[])
 useEffect(()=>{let active=true;setQr('');if(state.value)void QRCode.toDataURL(state.value,{width:864,margin:4,errorCorrectionLevel:'M'}).then(result=>{if(active)setQr(result)}).catch(()=>{if(active)setQr('unavailable')});return()=>{active=false}},[state.value])
 useEffect(()=>{if(state.phase==='connected')onConnected()},[state.phase,onConnected])
 const perform=async(work:()=>Promise<unknown>)=>{if(busy)return;setBusy(true);setError('');try{await work()}catch(e){if(alive.current)setError(e instanceof Error?e.message:'LOCAL_CONNECTION_LOST')}finally{if(alive.current)setBusy(false)}}
 const join=async(code:string)=>{setCamera(false);if(onJoin)await onJoin(code);else await runtime.join(code,state.expected);setValue('');setEnter(false)}
 useEffect(()=>{
  if(!camera)return
  let stopped=false,stream:MediaStream|undefined,timer:ReturnType<typeof setTimeout>|undefined
  const stop=()=>{stopped=true;clearTimeout(timer);stream?.getTracks().forEach(track=>track.stop());if(video.current)video.current.srcObject=null}
  void (async()=>{
   try{
    const scanner=await import('jsqr');stream=await navigator.mediaDevices.getUserMedia({video:{facingMode:{ideal:'environment'}},audio:false});if(stopped){stop();return}
    const element=video.current;if(!element){stop();return};element.srcObject=stream;await element.play()
    const canvas=document.createElement('canvas'),context=canvas.getContext('2d',{willReadFrequently:true})!
    const tick=()=>{if(stopped)return;if(element.readyState>=2){const scale=Math.min(1,720/Math.max(element.videoWidth,element.videoHeight));canvas.width=Math.round(element.videoWidth*scale);canvas.height=Math.round(element.videoHeight*scale);context.drawImage(element,0,0,canvas.width,canvas.height);const frame=context.getImageData(0,0,canvas.width,canvas.height),result=scanner.default(frame.data,canvas.width,canvas.height,{inversionAttempts:'dontInvert'});if(result){stop();setCamera(false);void perform(()=>join(result.data));return}}timer=setTimeout(tick,200)};tick()
   }catch{stop();if(alive.current){setCamera(false);setError('LOCAL_CAMERA_UNAVAILABLE');setEnter(true)}}
  })()
  return stop
 },[camera])
 const save=()=>{if(!state.value)return;const blob=new Blob([state.value],{type:'text/plain'}),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='Pinelink-connection.pinelink';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
 const importCode=async(chosen:File)=>{if(chosen.size>18000)throw new Error('LOCAL_CODE_INVALID');await join(await chosen.text())}
 const message=error==='LOCAL_CAMERA_UNAVAILABLE'?c.camera:localError(error||(state.phase==='expired'?undefined:state.error),c)
 return <div className="local-connect" data-local-phase={state.phase}>
  <input ref={file} hidden type="file" accept=".pinelink,text/plain" aria-label={c.import} onChange={e=>{const f=e.target.files?.[0];e.target.value='';if(f)void perform(()=>importCode(f))}}/>
  {camera?<div className="local-camera"><video ref={video} autoPlay playsInline muted aria-label={c.scan}/><MotionButton className="secondary-button guest-wide" onClick={()=>{setCamera(false);setEnter(true)}}>{c.enter}</MotionButton></div>:
   preparing||state.phase==='preparing'?<div className="local-code"><MotionSpinner/></div>:
   ['verify','waiting'].includes(state.phase)?<div className="local-compare"><ShieldCheck size={36}/><strong data-testid="local-comparison">{state.code}</strong><span className="guest-muted">{c.compare}</span>{state.phase==='waiting'?<><MotionSpinner/><span className="guest-muted">{c.waiting}</span></>:<MotionButton className="primary-button guest-wide" busy={busy} onClick={()=>perform(()=>runtime.confirm())}>{c.match}</MotionButton>}</div>:
   state.phase==='expired'?<div className="local-code"><Clock3 size={36}/><h3>{c.expired}</h3><MotionButton className="primary-button guest-wide" busy={busy} onClick={()=>onNewCode?onNewCode():perform(()=>runtime.invite(state.expected))}>{c.newCode}</MotionButton></div>:
   enter?<form className="local-enter" onSubmit={e=>{e.preventDefault();void perform(()=>join(value.trim()))}}><label>{c.package}<textarea value={value} onChange={e=>setValue(e.target.value)} maxLength={18000} placeholder={c.paste} spellCheck={false} autoComplete="off" autoCapitalize="none" required/></label><MotionButton className="primary-button guest-wide" type="submit" disabled={!value.trim()} busy={busy}>{c.continue}</MotionButton><div className="local-tools"><MotionTooltip label={c.scan}><MotionButton icon aria-label={c.scan} onClick={()=>setCamera(true)}><ScanLine/></MotionButton></MotionTooltip><MotionTooltip label={c.import}><MotionButton icon aria-label={c.import} onClick={()=>file.current?.click()}><FileUp/></MotionButton></MotionTooltip></div></form>:
   state.value?<div className="local-code">{qr==='unavailable'?<div className="local-qr-placeholder" role="img" aria-label={c.package}><Download size={48}/></div>:qr?<img src={qr} alt={c.package} width={240} height={240}/>:<MotionSpinner/>}{state.phase==='show'&&onlineCode&&<strong className="guest-code" data-testid="connection-code">{onlineCode.slice(0,4)+' '+onlineCode.slice(4)}</strong>}<span className="guest-muted">{qr==='unavailable'?c.packageFallback:state.phase==='reply'?c.scanReply:c.scanOther}</span>{state.phase==='show'&&<span className="guest-muted">{c.expires}</span>}<div className="local-tools"><MotionTooltip label={c.copy}><MotionButton icon aria-label={c.copy} onClick={()=>perform(()=>navigator.clipboard.writeText(state.value!))}><Copy/></MotionButton></MotionTooltip><MotionTooltip label={c.download}><MotionButton icon aria-label={c.download} onClick={save}><Download/></MotionButton></MotionTooltip><MotionTooltip label={c.scan}><MotionButton icon aria-label={c.scan} onClick={()=>setCamera(true)}><ScanLine/></MotionButton></MotionTooltip></div>{state.phase==='reply'&&<MotionButton className="secondary-button guest-wide" onClick={()=>onEnter?onEnter():setEnter(true)}>{c.enter}</MotionButton>}<span hidden data-testid="local-package" data-value={state.value}/></div>:
   onNewCode?<div className="local-entry"><MotionButton className="primary-button guest-wide" onClick={onNewCode}>{c.newCode}</MotionButton></div>:<div className="local-entry"><ScanLine size={44}/><MotionButton className="primary-button guest-wide" busy={busy} onClick={()=>perform(()=>runtime.invite())}>{c.create}</MotionButton><MotionButton className="secondary-button guest-wide" onClick={()=>setEnter(true)}>{c.enter}</MotionButton></div>}
  {message&&<p className="guest-inline-error" role="alert">{message}</p>}
 </div>
}
