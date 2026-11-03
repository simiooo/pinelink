// Read-only browser smoke. Preview credentials arrive on stdin and never enter reports.
const {chromium}=await import(process.env.PINELINK_PLAYWRIGHT_MODULE??new URL('../../web/node_modules/playwright-core/index.mjs',import.meta.url).href);
import {createHash} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
let input='';for await(const part of process.stdin)input+=part;const config=JSON.parse(input);
const preview=new URL(config.url),base=preview.origin;
if(preview.protocol!=='https:'||preview.hostname!=='staging-pinelink.sqmem.top'||preview.pathname!=='/designs/motion-v29.html')throw Error('Only the staging motion preview is supported');
const out=new URL(process.env.PINELINK_MOTION_OUT??'file:///tmp/pinelink-motion-staging/');await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.PINELINK_CHROMIUM??'/usr/bin/chromium',headless:true,args:['--no-sandbox','--disable-dev-shm-usage'],env:{...process.env,HOME:'/tmp'}});
const report={tested_at:new Date().toISOString(),url:preview.href,browser:browser.version(),checks:[],page_errors:[],preview_domain_requests:[],passed:false};
const check=(name,passed,details)=>{report.checks.push({name,passed:Boolean(passed),details});if(!passed)throw Error(name);};
try{
 const anon=await browser.newContext();const denied=await anon.request.get(preview.href);check('preview retains authentication',denied.status()===401,{status:denied.status()});await anon.close();
 const context=await browser.newContext({httpCredentials:{username:config.username,password:config.password,origin:base},viewport:{width:1360,height:960},reducedMotion:'no-preference'});
 const page=await context.newPage();page.on('pageerror',e=>report.page_errors.push(e.message));page.on('request',r=>{const u=new URL(r.url());if(u.origin===base&&/^\/(?:api|v1)\//.test(u.pathname))report.preview_domain_requests.push(u.pathname);});
 const response=await page.goto(preview.href);check('published artifact is served',response.status()===200,{status:response.status()});
 const digest=createHash('sha256').update(await response.body()).digest('hex');check('public artifact matches reviewed bytes',digest===config.artifact_sha256,{sha256:digest});
 const headers=await response.allHeaders(),csp=headers['content-security-policy']??'';
 check('preview has opaque sandbox and no API access',csp.includes('sandbox allow-scripts;')&&!csp.includes('allow-same-origin')&&csp.includes("connect-src 'none'"));
 check('preview cannot be cached or transformed',/private/.test(headers['cache-control'])&&/no-store/.test(headers['cache-control'])&&/no-transform/.test(headers['cache-control']));
 check('preview served through Cloudflare',Boolean(headers['cf-ray']));
 await page.waitForFunction(()=>window.PinelinkMotionPreview?.diagnostics().mounted);
 check('109 targets available',await page.evaluate(()=>window.PinelinkMotionPreview.catalog.targets.length)===109);
 check('storage isolated from product origin',await page.evaluate(()=>{try{localStorage.setItem('__motion_review_probe','test');localStorage.removeItem('__motion_review_probe');return false;}catch(e){return e.name==='SecurityError';}}));
 await page.evaluate(async()=>document.fonts.ready);check('offline embedded font loaded',await page.evaluate(()=>[...document.fonts].every(f=>f.status==='loaded')));
 await page.evaluate(()=>window.PinelinkMotionPreview.select('flow-active'));check('one relevant flow loop',(await page.evaluate(()=>window.PinelinkMotionPreview.diagnostics())).runningLoops===1);
 await page.selectOption('#motion-mode','reduced');check('reduced motion stops flow',(await page.evaluate(()=>window.PinelinkMotionPreview.diagnostics())).runningLoops===0);
 await page.selectOption('#motion-mode','full');await page.evaluate(()=>window.PinelinkMotionPreview.select('mobile-sheet'));await page.click('#replay');await page.waitForSelector('.preview-overlay');await page.keyboard.press('Escape');await page.waitForSelector('.preview-overlay',{state:'detached'});check('sandbox surface closes and releases focus',await page.locator('.preview-overlay').count()===0);
 await page.evaluate(()=>window.PinelinkMotionPreview.select('google-pending'));check('Google stays intrinsic with no email field',await page.locator('.google-action svg').evaluate(e=>e.getBoundingClientRect().height)===40&&await page.locator('#stage input').count()===0);
 await page.setViewportSize({width:390,height:960});await page.selectOption('#platform','mobile');check('mobile review has no horizontal overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 const missing=await context.request.get(base+'/designs/no-such-motion-preview.html');check('unknown review path does not return app shell',missing.status()===404,{status:missing.status()});
 const app=await context.request.get(base+'/');check('existing application still served',app.status()===200);
 for(const path of ['/version.json','/healthz']){const result=await context.request.get(base+path);const data=await result.json();check('runtime version retained '+path,result.status()===200&&data.version===config.runtime_source_version,{version:data.version});}
 check('no preview API effects',report.preview_domain_requests.length===0);check('no preview page errors',report.page_errors.length===0);report.passed=true;
}catch(e){report.failure=e.message;}finally{await writeFile(new URL('staging-preview-verification.json',out),JSON.stringify(report,null,2)+'\n');await browser.close();}
if(!report.passed)process.exitCode=1;
