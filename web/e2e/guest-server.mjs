import { createServer, request } from 'node:http'
import { connect } from 'node:net'
import { readFile } from 'node:fs/promises'
import { resolve, extname } from 'node:path'
export async function startGuestWebServer({apiBase=process.env.PINELINK_BROWSER_API_BASE??'http://127.0.0.1:8084'}={}) {
 const root=resolve('dist'),origin='http://localhost:5184',base=apiBase.replace(/\/$/,''),upstreamURL=new URL(base)
 const server=createServer(async(req,res)=>{
  res.setHeader('Cache-Control','no-store');res.setHeader('Referrer-Policy','no-referrer')
  const url=new URL(req.url,origin)
  if(url.pathname.startsWith('/api/')){const target=request(base+url.pathname.slice(4)+url.search,{method:req.method,headers:req.headers},reply=>{res.writeHead(reply.statusCode,reply.headers);reply.pipe(res)});target.on('error',()=>{res.statusCode=503;res.end()});req.pipe(target);return}
  if(url.pathname==='/service-config.json'){try{const info=await fetch(base+'/v1/replication/info?nonce='+'A'.repeat(43)).then(r=>r.json());res.setHeader('Content-Type','application/json');res.end(JSON.stringify({version:1,api_url:'/api',history_public_key:info.public_key}))}catch{res.statusCode=503;res.end()};return}
  try{const path=resolve(root,'.'+decodeURIComponent(url.pathname));if(!path.startsWith(root+'/')&&path!==root)throw Error('path');const file=extname(path)?path:resolve(root,'index.html');res.setHeader('Content-Type',({'.js':'text/javascript','.html':'text/html','.css':'text/css','.json':'application/json','.png':'image/png','.ttf':'font/ttf','.webmanifest':'application/manifest+json'})[extname(file)]??'application/octet-stream');res.end(await readFile(file))}catch{res.statusCode=404;res.end()}
 })
 server.on('upgrade',(req,socket,head)=>{
  if(!req.url.startsWith('/api/v1/p2p/')){socket.destroy();return}
  const upstream=connect(Number(upstreamURL.port||80),upstreamURL.hostname,()=>{const headers=[];for(let i=0;i<req.rawHeaders.length;i+=2)headers.push(req.rawHeaders[i]+': '+req.rawHeaders[i+1]);upstream.write('GET '+req.url.slice(4)+' HTTP/1.1\r\n'+headers.join('\r\n')+'\r\n\r\n');if(head.length)upstream.write(head);socket.pipe(upstream);upstream.pipe(socket)})
  socket.on('error',()=>upstream.destroy());upstream.on('error',()=>socket.destroy());socket.on('close',()=>upstream.destroy())
 })
 await new Promise(r=>server.listen(5184,'0.0.0.0',r))
 for(let i=0;i<100;i++){try{if((await fetch(base+'/healthz')).ok)return server}catch{}await new Promise(r=>setTimeout(r,200))}
 throw Error('guest fixture unavailable')
}
