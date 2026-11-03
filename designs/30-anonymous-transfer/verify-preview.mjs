import {readFile,writeFile,mkdir} from 'node:fs/promises'
import {createHash} from 'node:crypto'
const {chromium}=await import(process.env.PINELINK_PLAYWRIGHT_MODULE)
const file=new URL('./anonymous-v30.html',import.meta.url),out=new URL('./evidence/',import.meta.url);await mkdir(out,{recursive:true})
const manifest=JSON.parse(await readFile(new URL('./manifest.json',import.meta.url),'utf8'))
const browser=await chromium.launch({executablePath:'/usr/bin/chromium',headless:true,args:['--no-sandbox','--disable-dev-shm-usage']})
const page=await browser.newPage({viewport:{width:1360,height:1100}}),errors=[],external=[],checks=[]
page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(/^https?:/.test(r.url()))external.push(r.url())})
const check=(name,ok,detail)=>{checks.push({name,passed:!!ok,detail});if(!ok)throw Error(name+' '+JSON.stringify(detail))}
const diagnostics=()=>page.evaluate(()=>window.PinelinkGuestPreview.diagnostics())
try{
 await page.goto(file.href);await page.waitForFunction(()=>window.PinelinkGuestPreview);await page.evaluate(()=>document.fonts.ready)
 check('22 native-corresponding scenes',manifest.frames.length===22)
 check('bundled Nunito loaded',await page.evaluate(()=>[...document.fonts].some(f=>f.family==='Nunito'&&f.status==='loaded')))
 for(const f of manifest.frames){
  await page.evaluate(k=>window.PinelinkGuestPreview.show(k),f.key)
  check('scene '+f.key,(await diagnostics()).scene===f.key)
  for(const h of f.hotspots)check('48px target '+f.key+'/'+h.label,h.w>=48&&h.h>=48&&h.x>=0&&h.y>=0&&h.x+h.w<=f.width&&h.y+h.h<=f.height,h)
 }
 await page.evaluate(()=>window.PinelinkGuestPreview.show('mobile-start'))
 await page.getByRole('button',{name:'Connect a device',exact:true}).click();check('guest entry leads to pairing',(await diagnostics()).scene==='mobile-connect')
 await page.getByRole('button',{name:'Enter code',exact:true}).click();check('manual pairing available',(await diagnostics()).scene==='mobile-enter')
 await page.getByRole('button',{name:'Connect',exact:true}).click();check('joining waits for peer confirmation',(await diagnostics()).scene==='mobile-pair-waiting')
 await page.evaluate(()=>window.PinelinkGuestPreview.show('mobile-pair-request'))
 await page.getByRole('button',{name:'Connect',exact:true}).click();check('pair confirmation does not start file transfer',(await diagnostics()).scene==='mobile-start')
 await page.getByRole('button',{name:'模拟收到请求',exact:true}).click();check('explicit file confirmation',(await diagnostics()).scene==='mobile-incoming')
 await page.getByRole('button',{name:'Receive',exact:true}).click();check('receive starts the transfer state',(await diagnostics()).scene==='mobile-active')
 const d=await page.locator('[id="Real-receiver-ledger"] path[stroke="#0866ff"]').getAttribute('d')
 await page.getByRole('button',{name:'Transfer details',exact:true}).click()
 check('pause has no sheen',(await diagnostics()).scene==='mobile-paused'&&await page.locator('.flow').count()===0)
 check('pause preserves blue boundary',d===await page.locator('[id="Real-receiver-ledger"] path[stroke="#0866ff"]').getAttribute('d'))
 await page.getByRole('button',{name:'Resume',exact:true}).click();check('resume restarts only sheen',(await diagnostics()).loop===1)
 await page.emulateMedia({reducedMotion:'reduce'});check('reduced motion stops activity',(await diagnostics()).loop===0)
 await page.emulateMedia({reducedMotion:'no-preference'});await page.getByRole('button',{name:'模拟校验完成',exact:true}).click()
 check('completion never forces registration',(await diagnostics()).scene==='mobile-complete'&&await page.getByRole('button',{name:'Send another',exact:true}).count()===1)
 await page.getByRole('button',{name:'Keep in cloud',exact:true}).click();check('account value is opt-in',(await diagnostics()).scene==='mobile-keep')
 await page.getByRole('button',{name:'Continue without an account',exact:true}).click();check('can decline account prompt',(await diagnostics()).scene==='mobile-complete')
 await page.getByRole('button',{name:'模拟连接码过期',exact:true}).click();await page.getByRole('button',{name:'New code',exact:true}).click();check('expired invite recovers without login',(await diagnostics()).scene==='mobile-connect')
 await page.keyboard.press('Escape');check('escape closes the sheet',(await diagnostics()).scene==='mobile-start')
 for(const [width,key]of [[1360,'desktop-start'],[390,'mobile-start'],[320,'mobile-start-320'],[390,'mobile-connect'],[390,'mobile-active'],[390,'mobile-complete']]){
  await page.setViewportSize({width,height:1100});await page.evaluate(k=>window.PinelinkGuestPreview.show(k),key);await page.waitForTimeout(320)
  check('viewport '+width+'/'+key,await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  await page.screenshot({path:new URL(key+'-'+width+'.png',out).pathname,fullPage:true})
 }
 check('design preview identified',await page.getByText('交互设计预览，不会发送真实文件',{exact:true}).count()===1)
 check('no backend or external requests',external.length===0,external);check('no browser errors',errors.length===0,errors)
 await writeFile(new URL('prototype-verification.json',out),JSON.stringify({passed:true,checked_at:new Date().toISOString(),artifact_sha256:createHash('sha256').update(await readFile(file)).digest('hex'),environment:'Fedora / isolated no-network Chromium',checks,errors,external},null,2)+'\n')
 console.log(JSON.stringify({passed:true,checks:checks.length}))
}catch(e){await writeFile(new URL('prototype-verification.json',out),JSON.stringify({passed:false,error:e.stack,checks,errors,external},null,2)+'\n');throw e}
finally{await browser.close()}
