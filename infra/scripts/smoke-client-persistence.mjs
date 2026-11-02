const stagingHost = process.env.PINELINK_STAGING_HOST ?? '127.0.0.1'
import { chromium } from '../../web/node_modules/playwright-core/index.mjs'
import { execFileSync } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { randomUUID, createHash, webcrypto } from 'node:crypto'

const version=process.argv[2]
if (!/^[a-f0-9]{40}$/.test(version||'')) throw new Error('A full source version is required')
const root=resolve(import.meta.dirname,'../..'), output=join(root,'.artifacts','client-smoke',version,randomUUID())
// Keep Chrome's CacheStorage paths below the Windows path limit.
const profileRoot=join(root,'.artifacts','qa-'+randomUUID().slice(0,8))
await mkdir(output,{recursive:true})
const owner='staging-persistence', sender='qa-'+randomUUID(), api=`http://${stagingHost}:18680`
const binary=join(output,'pinelink.exe')
execFileSync('go',['build','-trimpath','-o',binary,'./cmd/pinelink'],{cwd:join(root,'cli'),timeout:120000})
const environment={...process.env,PINELINK_CONFIG_HOME:join(output,'sender-profile')}
function cli(...args) {
  const stdout=execFileSync(binary,['--server',api,'--owner',owner,'--device',sender,'--scope',sender,'--encryption-domain',sender,'--json',...args],{env:environment,encoding:'utf8',timeout:90000})
  return JSON.parse(stdout)
}
cli('device','register')
const fixture=join(output,'fixture.fig')
await writeFile(fixture,Buffer.from('encrypted browser persistence fixture\n'.repeat(256)))
const sleep=ms=>new Promise(r=>setTimeout(r,ms))
async function until(read,accept,timeout=90000) {
  const end=Date.now()+timeout
  while(Date.now()<end) { const value=await read(); if(accept(value))return value; await sleep(350) }
  throw new Error('client persistence convergence timed out')
}
const results={version,started_at:new Date().toISOString(),checks:{}}

async function launch(kind,offline=false) {
  const base=kind==='web'?`https://${stagingHost}:18682`:`https://${stagingHost}:18683`
  const context=await chromium.launchPersistentContext(join(profileRoot,kind),{executablePath:process.env.CHROMIUM_EXECUTABLE ?? 'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true,ignoreHTTPSErrors:true,locale:'en-US',viewport:{width:kind==='web'?1280:390,height:844},args:['--ignore-certificate-errors','--no-proxy-server']})
  await context.setOffline(offline)
  const page=context.pages()[0]??await context.newPage()
  const state={context,page,base,kind,device:'',signed:undefined,consoleErrors:[]}
  page.on('request',request=>{
    const headers=request.headers()
    if(request.url().includes('/api/v1/') && headers['x-pinelink-device']) {
      state.device=headers['x-pinelink-device']
      state.signed={method:request.method(),url:request.url(),body:request.postData()??'',headers}
    }
  })
  page.on('pageerror',error=>state.consoleErrors.push(error.name+':'+error.message.slice(0,180)))
  try {
  await page.goto(base,{waitUntil:'load',timeout:90000})
  if(kind==='web'&&offline)await page.waitForFunction(()=>document.getElementById('root')?.childElementCount>0)
  else await until(()=>state.device,v=>Boolean(v))
  if(!offline) {
    const actual=await page.evaluate(async()=>await (await fetch('/version.json')).json())
    if(actual.version!==version)throw new Error('client version mismatch')
    await page.evaluate(async()=>{await Promise.race([navigator.serviceWorker.ready,new Promise((_,reject)=>setTimeout(()=>reject(new Error('SW readiness timed out')),30000))])})
    await until(()=>page.evaluate(async()=>(await caches.keys()).length),v=>v>0)
  }
  if(kind==='flutter') {
    await page.locator('flutter-view').waitFor({timeout:90000})
    const placeholder=page.locator('flt-semantics-placeholder')
    if(await placeholder.count())await placeholder.evaluate(e=>e.click())
  }
  return state
  } catch(error) {
    await page.screenshot({path:join(output,kind+'-startup-failure.png'),fullPage:true}).catch(()=>{})
    await context.close()
    throw error
  }
}
async function flutterPage(state,name) {
  if(name==='File notifications') {
    const shortcut=state.page.getByRole('button',{name:'Notifications',exact:true})
    if(await shortcut.count()) { await shortcut.click(); return }
  }
  await state.page.getByRole('button',{name:/more/i}).first().click({timeout:15000})
  await state.page.getByLabel(name,{exact:true}).or(state.page.getByText(name,{exact:true})).filter({visible:true}).first().click({timeout:15000})
}
async function history(state) {
  if(state.kind==='web')await state.page.goto(state.base+'/history',{waitUntil:'domcontentloaded'})
  else await flutterPage(state,'History & issues')
  const file=/encrypted file|received file|fixture\.fig/i
  await state.page.getByText(file).or(state.page.getByLabel(file)).filter({visible:true}).first().waitFor({timeout:30000})
}
async function publicIdentity(state) {
  if(state.kind==='web')return state.page.evaluate(async()=>{
    const names=await indexedDB.databases(),name=names.find(x=>x.name?.startsWith('pinelink-'))?.name
    return await new Promise((resolve,reject)=>{
      const open=indexedDB.open(name)
      open.onerror=()=>reject(new Error('profile open failed'))
      open.onsuccess=()=>{
        const db=open.result,request=db.transaction('profiles').objectStore('profiles').get('self')
        request.onsuccess=()=>{const p=request.result;resolve({root:p.root,device:p.device,signing_key:p.signingKey,nonextractable:!p.signingPrivate.extractable&&!p.encryptionPrivate.extractable});db.close()}
        request.onerror=()=>reject(new Error('profile read failed'))
      }
    })
  })
  if(!/^flutter-[a-f0-9-]+$/.test(state.device))throw new Error('unexpected public device id')
  const sql=`SELECT signing_public_key FROM devices WHERE owner_id='${owner}' AND device_id='${state.device}';`
  const publicKey=execFileSync('docker',['--context',process.env.PINELINK_DOCKER_CONTEXT ?? 'default','exec','pinelink-staging-postgres-1','psql','-U','pinelink','-d','pinelink','-At','-c',sql],{encoding:'utf8'}).trim()
  return {device:state.device,signing_key:publicKey,root:createHash('sha256').update(Buffer.from(publicKey,'base64')).digest('hex')}
}
async function verifyRequest(request,identity) {
  const url=new URL(request.url),path=url.pathname.replace(/^\/api/,'')+url.search
  const h=request.headers
  const proof=Buffer.from(`${request.method}\n${path}\n${h['x-pinelink-timestamp']}\n${createHash('sha256').update(request.body).digest('hex')}`)
  const key=await webcrypto.subtle.importKey('raw',Buffer.from(identity.signing_key,'base64'),'Ed25519',false,['verify'])
  return webcrypto.subtle.verify('Ed25519',key,Buffer.from(h['x-pinelink-signature'],'base64'),proof)
}
for(const kind of ['web','flutter']) {
  let state
  try {
    console.log(kind+': launching')
    state=await launch(kind)
    console.log(kind+': client ready')
    const identity=await publicIdentity(state)
    console.log(kind+': persistent public identity read')
    if(!await verifyRequest(state.signed,identity))throw new Error('live device signature invalid')
    const sent=cli('p2p','send',fixture,'--to-owner',owner,'--to-device',identity.device,'--path','relay')
    console.log(kind+': encrypted fixture stored')
    if(!sent.stored)throw new Error('fixture was not encrypted and stored')
    if(kind==='web')await state.page.goto(state.base+'/notifications',{waitUntil:'domcontentloaded'})
    else await flutterPage(state,'File notifications')
    await state.page.getByRole('button',{name:'Accept',exact:true}).click({timeout:45000})
    await history(state)
    await state.page.screenshot({path:join(output,kind+'-online.png'),fullPage:true})
    await state.context.setOffline(true)
    await state.context.close()
    cli('cancel',sent.id)
    state=await launch(kind,true)
    // Web may satisfy all reads from IDB without making an offline request.
    if(kind==='web') {
      const after=await publicIdentity(state)
      if(after.root!==identity.root||after.device!==identity.device||!after.nonextractable)throw new Error('reopened identity changed')
    } else if(state.device!==identity.device||!await verifyRequest(state.signed,identity))throw new Error('reopened vault identity changed')
    await history(state)
    await state.page.screenshot({path:join(output,kind+'-offline-reopened.png'),fullPage:true})
    await state.context.setOffline(false)
    const cancelled=/cancelled|canceled/i
    await state.page.getByText(cancelled).or(state.page.getByLabel(cancelled)).filter({visible:true}).first().waitFor({timeout:90000})
    await state.page.screenshot({path:join(output,kind+'-caught-up.png'),fullPage:true})
    if(state.consoleErrors.length)throw new Error('client page errors: '+state.consoleErrors.join(';'))
    results.checks[kind]={public_root:identity.root,device:identity.device,real_signed_api:true,encrypted_cloud_fixture:true,persistent_browser_restart:true,offline_history:true,automatic_catchup:true,service_worker:true}
    await writeFile(join(output,'progress.json'),JSON.stringify(results,null,2))
  } catch(error) {
    if(state)await state.page.screenshot({path:join(output,kind+'-failure.png'),fullPage:true}).catch(()=>{})
    throw error
  } finally { if(state)await state.context.close() }
}
results.completed_at=new Date().toISOString()
await writeFile(join(output,'smoke.json'),JSON.stringify(results,null,2))
console.log(JSON.stringify({evidence:join(output,'smoke.json'),...results},null,2))
