import {chromium,firefox} from 'playwright-core'
import {writeFile} from 'node:fs/promises'
import {startGuestWebServer} from './guest-server.mjs'
const kind=process.env.PINELINK_DIRECT_BROWSER??'chromium',name=process.env.PINELINK_DIRECT_PEER
const origin=process.env.PINELINK_DIRECT_ORIGIN??'http://localhost:5184'
const web=origin.startsWith('http://localhost:')?await startGuestWebServer():undefined
const server=await (kind==='firefox'?firefox:chromium).launchServer({host:'0.0.0.0',port:5190,headless:true,...(kind==='chromium'?{args:['--no-sandbox','--disable-dev-shm-usage']}:{})})
const endpoint=server.wsEndpoint().replace('0.0.0.0',name).replace('127.0.0.1',name).replace('localhost',name)
await writeFile('/output/'+process.env.PINELINK_DIRECT_SLOT+'.json',JSON.stringify({endpoint,kind,origin}))
const shutdown=async()=>{await server.close();if(web){web.closeAllConnections();web.close()}}
process.once('SIGTERM',shutdown);process.once('SIGINT',shutdown)
