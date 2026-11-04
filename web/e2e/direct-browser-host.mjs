import {chromium,firefox} from 'playwright-core'
import {writeFile} from 'node:fs/promises'
import {spawn} from 'node:child_process'
import {startGuestWebServer} from './guest-server.mjs'
const kind=process.env.PINELINK_DIRECT_BROWSER??'chromium',name=process.env.PINELINK_DIRECT_PEER
const origin=process.env.PINELINK_DIRECT_ORIGIN??'http://localhost:5184'
const launcher=process.env.PINELINK_LOCAL_WEB_BINARY?spawn(process.env.PINELINK_LOCAL_WEB_BINARY,['local-web','--web-dir','/source/web/dist','--listen','127.0.0.1:'+new URL(origin).port,'--open=false'],{stdio:'inherit'}):undefined
if(launcher){let ready=false;for(let i=0;i<100;i++){if(launcher.exitCode!==null)throw Error('Local launcher exited');try{const r=await fetch(origin+'/api/v1/auth/capabilities');if(r.ok&&(await r.json()).local_only){ready=true;break}}catch{}await new Promise(r=>setTimeout(r,100))}if(!ready)throw Error('Local launcher did not become ready')}
const web=!launcher&&origin.startsWith('http://localhost:')?await startGuestWebServer():undefined
const server=await (kind==='firefox'?firefox:chromium).launchServer({host:'0.0.0.0',port:5190,headless:true,...(kind==='chromium'?{args:['--no-sandbox','--disable-dev-shm-usage']}:{})})
const endpoint=server.wsEndpoint().replace('0.0.0.0',name).replace('127.0.0.1',name).replace('localhost',name)
await writeFile('/output/'+process.env.PINELINK_DIRECT_SLOT+'.json',JSON.stringify({endpoint,kind,origin}))
const shutdown=async()=>{await server.close();if(web){web.closeAllConnections();web.close()};launcher?.kill('SIGTERM')}
process.once('SIGTERM',shutdown);process.once('SIGINT',shutdown)
