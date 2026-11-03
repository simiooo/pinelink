import {chromium} from 'playwright-core'
import {mkdir,writeFile} from 'node:fs/promises'
import assert from 'node:assert/strict'
import {startGuestWebServer} from './guest-server.mjs'

const server=await startGuestWebServer(),out='e2e-output/surfaces';await mkdir(out,{recursive:true})
const browser=await chromium.launch({...(process.env.PINELINK_BUNDLED_BROWSER?{}:{executablePath:'/usr/bin/chromium'}),headless:true,args:['--no-sandbox','--disable-dev-shm-usage']})
const checks=[],errors=[]
try{
 for(const width of [1280,390])for(const reducedMotion of ['no-preference','reduce']){
  const context=await browser.newContext({viewport:{width,height:900},locale:'en-US',reducedMotion});const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.goto('http://localhost:5184/');await page.locator('.guest-page').waitFor()
  for(const closer of ['close','escape','backdrop',...(width<640?['drag']:[])]){
   await page.getByRole('button',{name:'Connect a device',exact:true}).first().click();await page.getByRole('dialog').waitFor();await page.waitForTimeout(300)
   await page.evaluate(()=>{
    const graph=document.querySelector('.guest-network');graph.dataset.surfaceProbe='stable';const start=performance.now();window.__surfaceFrames=[]
    const frame=()=>{const overlay=document.querySelector('[data-slot=dialog-overlay],[data-slot=sheet-overlay]'),network=document.querySelector('.guest-network'),box=network.getBoundingClientRect(),style=overlay&&getComputedStyle(overlay);window.__surfaceFrames.push({time:performance.now()-start,closed:overlay?.getAttribute('data-state')==='closed',opacity:style?Number(style.opacity):0,fill:style?.animationFillMode,stable:network.dataset.surfaceProbe==='stable',x:box.x,y:box.y,width:box.width,height:box.height});if(performance.now()-start<550)requestAnimationFrame(frame)};requestAnimationFrame(frame)
   })
   if(closer==='close')await page.getByRole('button',{name:'Close',exact:true}).click()
   if(closer==='escape')await page.keyboard.press('Escape')
   if(closer==='backdrop')await page.mouse.click(8,8)
   if(closer==='drag'){const box=await page.locator('.sheet-drag-target').boundingBox();await page.mouse.move(box.x+box.width/2,box.y+20);await page.mouse.down();await page.mouse.move(box.x+box.width/2,box.y+144,{steps:7});await page.mouse.up()}
   await page.locator('[role=dialog]').waitFor({state:'detached'});await page.waitForTimeout(600)
   const frames=await page.evaluate(()=>window.__surfaceFrames),closed=frames.filter(f=>f.closed)
   for(let i=1;i<closed.length;i++)assert.ok(closed[i].opacity<=closed[i-1].opacity+.03,'backdrop becomes opaque again after fading')
   assert.ok(frames.every(f=>f.stable),'device graph was remounted on close')
   for(const coordinate of ['x','y','width','height'])assert.ok(Math.max(...frames.map(f=>f[coordinate]))-Math.min(...frames.map(f=>f[coordinate]))<1,'device graph shifted on close')
   if(reducedMotion==='no-preference')assert.ok(closed.every(f=>f.fill==='both'),'exit must retain its final transparent frame')
   checks.push({width,reducedMotion,closer,frames:frames.length,closedFrames:closed.length,passed:true})
  }
  await page.screenshot({path:out+'/closed-'+width+'-'+reducedMotion+'.png'});await context.close()
 }
 assert.deepEqual(errors,[]);await writeFile(out+'/verification.json',JSON.stringify({passed:true,checked_at:new Date().toISOString(),checks,errors},null,2));console.log(JSON.stringify({passed:true,checks:checks.length}))
}catch(error){await writeFile(out+'/verification.json',JSON.stringify({passed:false,error:error.stack,checks,errors},null,2));throw error}
finally{await browser.close();server.closeAllConnections();await new Promise(r=>server.close(r))}
