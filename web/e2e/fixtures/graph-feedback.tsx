import React from 'react'
import { createRoot } from 'react-dom/client'
import { GuestGraph } from '../../src/guest/Graph'
import { I18nProvider } from '../../src/i18n'
import { MotionProvider } from '../../src/motion/react'
import '../../src/styles/app.css'
import '../../src/guest/guest.css'
import '../../src/motion/motion.css'
const peer:any={pair_id:'p',owner_id:'o',device_id:'d',device_name:'Phone',encryption_key:'k',signing_key:'k',expires_at:'2099-01-01T00:00:00Z',online:true}
const job:any={id:'j',created:0,direction:'send',peer,received:100,name:'file',plainSize:100,phase:'complete',transport:{state:'direct'},transfer:{id:'t',state:'COMPLETED',version:2,file_name:'file',size_bytes:100,content_root:'r',encryption_suite:'s',sender_owner_id:'o',sender_device_id:'d',recipient_owner_id:'b',recipient_device_id:'e',expires_at:peer.expires_at,received_bytes:100},snapshot:{transfer_id:'t',state:'COMPLETED',state_version:2,control_revision:1,progress_revision:1,route:{kind:'DIRECT',epoch:1},delivery:{state:'DELIVERED',acknowledged_bytes:100,total_bytes:100,revision:1},spool:{state:'NOT_REQUIRED',acknowledged_bytes:0,total_bytes:100},completion:{delivered:true,stored:false},capabilities:{pause:false,resume:false,cancel:false,restore:false}}}
if (location.search.includes('relay')) { job.transport.state='cloud'; job.snapshot.route.kind='RELAY'; job.snapshot.spool={state:'SPOOLING',acknowledged_bytes:58,total_bytes:100}; job.snapshot.completion.stored=false }
createRoot(document.getElementById('root')!).render(<I18nProvider initialPreference="en-US"><MotionProvider><GuestGraph peers={[peer]} job={location.search ? job : undefined} onConnect={()=>{}} onPeer={()=>{}} onDetails={()=>{}} dragging={false} onFileDrop={()=>{}} localDevice="self" /></MotionProvider></I18nProvider>)


