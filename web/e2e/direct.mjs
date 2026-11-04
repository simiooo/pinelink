import {chromium,firefox} from 'playwright-core'
import {mkdir,readFile,writeFile,rename} from 'node:fs/promises'
import {createHash,randomBytes,randomUUID} from 'node:crypto'
import assert from 'node:assert/strict'
import {visitorState} from './guest-probe.mjs'

const out=process.env.PINELINK_DIRECT_OUTPUT??'/output',origin=process.env.PINELINK_DIRECT_ORIGIN??'http://localhost:5184'
await mkdir(out,{recursive:true})
const configs=await Promise.all(['a','b'].map(async slot=>JSON.parse(await readFile(out+'/'+slot+'.json','utf8'))))
const browsers=await Promise.all(configs.map(c=>(c.kind==='firefox'?firefox:chromium).connect(c.endpoint)))
const checks=[],errors=[],contexts=[],records=[],proofs=[],requested=(process.env.PINELINK_DIRECT_CASES??'direct,empty,pending-close,blocked,drop,pause,cancel').split(',')
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms))
const until=async(work,timeout=60000)=>{const end=Date.now()+timeout;while(Date.now()<end){const result=await work();if(result)return result;await delay(100)}throw Error('direct browser condition timeout')}
const mark=value=>{checks.push(value);console.log(value)}
const fixture=process.env.PINELINK_BROWSER_API_BASE
const state=async()=>fixture?fetch(fixture+'/__test/state').then(r=>{assert.equal(r.status,200);return r.json()}):undefined
const network=async blocked=>{
 const id=randomUUID(),file=out+'/network-request.json'
 await writeFile(file+'.tmp',JSON.stringify({id,blocked}));await rename(file+'.tmp',file)
 await until(async()=>{try{const result=JSON.parse(await readFile(out+'/network-result.json','utf8'));if(result.id!==id)return false;assert.equal(result.applied,true,result.error);return true}catch(error){if(error.code==='ENOENT')return false;throw error}},15000)
}
const fresh=async(slot,width)=>{
 const context=await browsers[slot].newContext({locale:'en-US',viewport:{width,height:900},acceptDownloads:true})
 contexts.push(context);const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));const sockets=[]
 page.on('websocket',socket=>{if(socket.url().includes('/relay?'))sockets.push(socket.url().split('?')[0])})
 await page.goto(origin);await page.locator('.guest-page').waitFor()
 assert.equal(await page.evaluate(()=>isSecureContext&&typeof RTCPeerConnection==='function'),true)
 return{page,context,sockets}
}
const pair=async()=>{
 const a=await fresh(0,1280),b=await fresh(1,390)
 await a.page.locator('.workspace-add-device').click()
 const code=await until(async()=>{const value=await a.page.getByTestId('connection-code').textContent();return value?.match(/^[0-9A-Z]{4} [0-9A-Z]{4}$/)?value:false})
 await b.page.locator('.workspace-add-device').click();await b.page.getByRole('tab',{name:'Enter code',exact:true}).click()
 await b.page.getByLabel('Connection code',{exact:true}).fill(code);await b.page.getByRole('button',{name:'Connect',exact:true}).click()
 await a.page.getByRole('heading',{name:'Connect this device?',exact:true}).waitFor();await a.page.getByRole('button',{name:'Connect',exact:true}).click()
 await until(async()=>!await a.page.locator('[role=dialog]').count()&&!await b.page.locator('[role=dialog]').count())
 await until(async()=>(await visitorState(a.page))?.peers.length===1&&(await visitorState(b.page))?.peers.length===1)
 return{a,b}
}
const send=async(p,bytes,name)=>{
 await p.a.page.locator('input[type=file]').setInputFiles({name,mimeType:'application/octet-stream',buffer:bytes})
 await p.a.page.getByRole('button',{name:'Send',exact:true}).click()
 const job=await until(async()=>(await visitorState(p.a.page))?.jobs.find(j=>j.name===name&&j.transfer))
 records.push({name,transfer:job.transfer.id,sender:job.transfer.sender_owner_id,recipient:job.transfer.recipient_owner_id,pair:job.peer.pair_id})
 await p.b.page.getByRole('button',{name:'Receive',exact:true}).waitFor()
 return job
}
const jobOf=async(p,name)=>(await visitorState(p))?.jobs.find(j=>j.name===name)
const connected=async(p,name)=>{
 await until(async()=>(await jobOf(p.a.page,name))?.transport?.state==='direct'&&(await jobOf(p.b.page,name))?.transport?.state==='direct',25000)
 for(const side of [p.a,p.b]){assert.equal(await side.page.locator('.guest-network').getAttribute('data-route'),'direct');assert.equal(await side.page.locator('.guest-cloud').count(),0);assert.equal(await side.page.locator('.guest-curve').count(),1)}
 assert.equal((await jobOf(p.b.page,name)).received,0)
 assert.equal((await jobOf(p.b.page,name)).transfer.encrypted_manifest,undefined)
}
const save=async(page,bytes,name)=>{
 await page.getByRole('button',{name:'Save file',exact:true}).waitFor({timeout:120000})
 const waiting=page.waitForEvent('download');await page.getByRole('button',{name:'Save file',exact:true}).click()
 const download=await waiting;assert.equal(download.suggestedFilename(),name)
 const hash=createHash('sha256');for await(const part of await download.createReadStream())hash.update(part)
 const digest=hash.digest('hex');assert.equal(digest,createHash('sha256').update(bytes).digest('hex'));return digest
}
const closePair=async p=>{for(const side of [p.a,p.b])if(!side.page.isClosed())await side.context.close()}
let current
const completedDirect=async(name,size)=>{
 const p=current=await pair(),bytes=randomBytes(size),before=await state(),job=await send(p,bytes,name)
 await connected(p,name)
 if(before){const waiting=await state();assert.equal(waiting.blob_puts,before.blob_puts);assert.equal(waiting.relay_spools,before.relay_spools)}
 mark(name+': actual DataChannel and one curve, no file bytes before consent')
 await p.b.page.getByRole('button',{name:'Receive',exact:true}).click()
 const digest=await save(p.b.page,bytes,name);await until(async()=>(await jobOf(p.a.page,name))?.phase==='complete')
 const a=await jobOf(p.a.page,name),b=await jobOf(p.b.page,name)
 for(const value of [a,b]){assert.equal(value.snapshot.route.kind,'DIRECT');assert.equal(value.snapshot.completion.stored,false);assert.equal(value.snapshot.completion.delivered,true);assert.equal(value.snapshot.spool.state,'NOT_REQUIRED')}
 assert.equal(p.a.sockets.length+p.b.sockets.length,0)
 assert.ok(a.transport.proof.sent>0);assert.ok(b.transport.proof.received>0)
 assert.notEqual(a.transport.proof.local,'relay');assert.notEqual(b.transport.proof.remote,'relay')
 if(before){const after=await state();assert.equal(after.blob_puts,before.blob_puts);assert.equal(after.blob_gets,before.blob_gets);assert.equal(after.relay_spools,before.relay_spools)}
 proofs.push({name,transfer:job.transfer.id,sha256:digest,bytes:size,sender:a.transport.proof,receiver:b.transport.proof,spool:b.snapshot.spool.state})
 await p.b.page.screenshot({path:out+'/'+name+'-complete.png'})
 mark(name+': hash and receipt verified; actual ICE/data-channel counters and no cloud file I/O')
 await closePair(p)
}
const interceptCheckpoint=async(page,hold=false,onCheckpoint=async()=>{})=>{
 let reached,release;const entered=new Promise(resolve=>{reached=resolve}),gate=new Promise(resolve=>{release=resolve});let captured=false
 await page.route('**/api/v1/p2p/*/progress',async route=>{
  const req=route.request(),body=req.method()==='POST'?req.postDataJSON():undefined
  if(captured||body?.route!=='DIRECT'||!body.received_bytes){await route.continue();return}
  captured=true;const response=await route.fetch();assert.equal(response.status(),200)
  await onCheckpoint(body.received_bytes);reached(body.received_bytes);if(hold)await gate
  await route.fulfill({response}).catch(error=>{if(!/closed|canceled|abort|finished/i.test(error.message))throw error})
 })
 return{entered,release:()=>release()}
}
try{
 if(requested.includes('direct'))await completedDirect('direct-16MiB.bin',16<<20)
 if(requested.includes('empty'))await completedDirect('direct-empty.bin',0)
 if(requested.includes('pending-close')){
  const p=current=await pair(),bytes=randomBytes(2<<20),name='pending-close.bin'
  await send(p,bytes,name);await connected(p,name);await p.b.page.close()
  await until(async()=>(await jobOf(p.a.page,name))?.snapshot?.completion.stored)
  await p.a.context.close();p.b.page=await p.b.context.newPage();p.b.page.on('pageerror',e=>errors.push(e.message));await p.b.page.goto(origin)
  await p.b.page.getByRole('button',{name:'Receive',exact:true}).click();await save(p.b.page,bytes,name)
  mark('Receiver closes before file consent: sender stages cloud copy; receiver reopens and verifies after sender exit')
  proofs.push({name,bytes:bytes.length,sourceClosed:true,receiverReopened:true});await closePair(p)
 }
 if(requested.includes('blocked')){
  const p=current=await pair(),bytes=randomBytes(2<<20),name='blocked-fallback.bin';await network(true)
  let delayedCloud=false
  await p.b.page.route('**/api/v1/p2p/*/progress',async route=>{const req=route.request(),body=req.method()==='POST'?req.postDataJSON():undefined;if(!delayedCloud&&body?.route==='RELAY'&&!body.received_bytes){delayedCloud=true;await delay(500)}await route.continue()})
  await send(p,bytes,name)
  await until(async()=>(await jobOf(p.a.page,name))?.snapshot?.completion.stored)
  assert.equal((await jobOf(p.b.page,name)).received,0);assert.equal((await jobOf(p.b.page,name)).transfer.state,'PENDING')
  assert.equal(await p.a.page.locator('.guest-network').getAttribute('data-route'),'relay')
  await p.a.context.close();await p.b.page.getByRole('button',{name:'Receive',exact:true}).click()
  await save(p.b.page,bytes,name);const b=await jobOf(p.b.page,name);assert.equal(b.snapshot.route.kind,'RELAY')
  assert.equal(b.failures,undefined);assert.equal(b.error,undefined)
  proofs.push({name,bytes:bytes.length,route:b.snapshot.route,sourceClosed:true})
  mark('Blocked peer network: timed negotiation falls back and cloud stages before consent; receiver verifies with sender fully closed')
  await closePair(p);await network(false)
 }
 if(requested.includes('drop')){
  const p=current=await pair(),bytes=randomBytes(12<<20),name='dropped-direct.bin',before=await state()
  const checkpoint=await interceptCheckpoint(p.b.page,false,async cut=>{await until(async()=>(await jobOf(p.a.page,name))?.snapshot?.delivery.acknowledged_bytes>=cut);await p.a.page.screenshot({path:out+'/direct-running-desktop.png'});await p.b.page.screenshot({path:out+'/direct-running-mobile.png'});await network(true)})
  await send(p,bytes,name);await connected(p,name);await p.b.page.getByRole('button',{name:'Receive',exact:true}).click()
  const cut=await checkpoint.entered
  await save(p.b.page,bytes,name);await until(async()=>(await jobOf(p.a.page,name))?.phase==='complete')
  const b=await jobOf(p.b.page,name);assert.equal(b.snapshot.route.kind,'RELAY');assert.ok(b.snapshot.route.epoch>=2);assert.ok(b.snapshot.delivery.acknowledged_bytes>=cut)
  if(before){const after=await state();assert.ok(after.blob_put_bytes>before.blob_put_bytes)}
  assert.ok(p.a.sockets.length>0&&p.b.sockets.length>0)
  proofs.push({name,bytes:bytes.length,cut,route:b.snapshot.route})
  mark('Kernel network loss after committed direct bytes: automatic cloud fallback preserves ledger, verifies original file and receipt')
  await closePair(p);await network(false)
 }
 if(requested.includes('pause')||requested.includes('cancel')){
  for(const action of ['pause','cancel'].filter(a=>requested.includes(a))){
   const p=current=await pair(),bytes=randomBytes(16<<20),name=action+'-direct.bin',before=await state()
   const checkpoint=await interceptCheckpoint(p.b.page,true)
   await send(p,bytes,name);await connected(p,name);await p.b.page.getByRole('button',{name:'Receive',exact:true}).click();const cut=await checkpoint.entered
   const side=action==='pause'?p.b:p.a
   await side.page.getByRole('button',{name:'Transfer details',exact:true}).first().click()
   await side.page.getByRole('button',{name:action==='pause'?'Pause':'Cancel transfer',exact:true}).click();checkpoint.release()
   if(action==='pause'){
    await until(async()=>(await jobOf(p.b.page,name))?.phase==='paused')
    const paused=(await jobOf(p.b.page,name)).snapshot.delivery.acknowledged_bytes;assert.ok(paused>=cut)
    await until(async()=>(await jobOf(p.a.page,name))?.snapshot?.completion.stored)
    await delay(700);assert.equal((await jobOf(p.b.page,name)).snapshot.delivery.acknowledged_bytes,paused)
    await p.a.context.close();const identity=await visitorState(p.b.page);await p.b.page.reload()
    await until(async()=>(await jobOf(p.b.page,name))?.phase==='paused')
    assert.equal((await visitorState(p.b.page)).owner,identity.owner);assert.equal((await visitorState(p.b.page)).device,identity.device)
    assert.equal(await p.b.page.locator('[data-ledger=delivery]').getAttribute('data-running'),'false')
    await p.b.page.getByRole('button',{name:'Transfer details',exact:true}).first().click();await p.b.page.getByRole('button',{name:'Resume',exact:true}).click()
    await save(p.b.page,bytes,name);assert.equal((await jobOf(p.b.page,name)).snapshot.route.kind,'RELAY')
    mark('Direct pause fences late callbacks; frozen progress survives reload and cloud resume after the sender exits')
   }else{
    await until(async()=>(await jobOf(p.a.page,name))?.phase==='stopped'&&(await jobOf(p.b.page,name))?.phase==='stopped')
    if(before){const after=await state();assert.equal(after.relay_spools,before.relay_spools);assert.equal(after.blob_puts,before.blob_puts);assert.equal(after.delivery_receipts,before.delivery_receipts)}
    mark('Direct cancellation terminates both endpoints without a fallback upload or delivery receipt')
   }
   proofs.push({name,bytes:bytes.length,cut,action});await closePair(p)
  }
 }
 assert.deepEqual(errors,[])
 await writeFile(out+'/verification.json',JSON.stringify({passed:true,checked_at:new Date().toISOString(),origin,browsers:await Promise.all(browsers.map(b=>b.version())),checks,records,proofs,errors},null,2))
 console.log(JSON.stringify({passed:true,checks:checks.length}))
}catch(error){
 await writeFile(out+'/failure.json',JSON.stringify({error:error.stack,checks,errors,records,states:current?await Promise.all([current.a,current.b].map(s=>s.page.isClosed()?null:visitorState(s.page).catch(()=>null))):[]},null,2))
 if(current)for(const [i,side] of [current.a,current.b].entries())if(!side.page.isClosed())await side.page.screenshot({path:out+'/failure-'+i+'.png'}).catch(()=>{})
 throw error
}finally{
 for(const context of contexts)await context.close().catch(()=>{})
 for(const browser of browsers)await browser.close().catch(()=>{})
}
