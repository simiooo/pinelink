// Test-only bridge inside an unpublished, per-run Docker network. The actual
// fixture and its mail probe remain loopback-bound; mail is never forwarded.
import {createServer,request} from 'node:http'
import {connect} from 'node:net'
import {spawn} from 'node:child_process'
const fixture=process.env.PINELINK_DIRECT_FIXTURE_BINARY?spawn(process.env.PINELINK_DIRECT_FIXTURE_BINARY,[],{stdio:'inherit'}):undefined
const probes=new Map([['/__test/state','/state'],['/__test/delay','/delay'],['/__test/expire-transfer','/expire-transfer']])
const server=createServer((req,res)=>{
 const url=new URL(req.url,'http://fixture'),probe=probes.get(url.pathname)
 if(!probe&&!url.pathname.startsWith('/v1/')&&url.pathname!='/healthz'){res.writeHead(404);res.end();return}
 const target=request('http://127.0.0.1:'+(probe?'8085':'8084')+(probe??url.pathname)+url.search,{method:req.method,headers:req.headers},reply=>{res.writeHead(reply.statusCode,reply.headers);reply.pipe(res)})
 target.on('error',()=>{res.writeHead(503);res.end()});req.pipe(target)
})
server.on('upgrade',(req,socket,head)=>{
 if(!req.url.startsWith('/v1/p2p/')){socket.destroy();return}
 const upstream=connect(8084,'127.0.0.1',()=>{const headers=[];for(let i=0;i<req.rawHeaders.length;i+=2)headers.push(req.rawHeaders[i]+': '+req.rawHeaders[i+1]);upstream.write('GET '+req.url+' HTTP/1.1\r\n'+headers.join('\r\n')+'\r\n\r\n');if(head.length)upstream.write(head);socket.pipe(upstream);upstream.pipe(socket)})
 socket.on('error',()=>upstream.destroy());upstream.on('error',()=>socket.destroy());socket.on('close',()=>upstream.destroy())
})
await new Promise(resolve=>server.listen(8094,'0.0.0.0',resolve))
process.on('SIGTERM',()=>{server.closeAllConnections();server.close();fixture?.kill('SIGTERM')})
