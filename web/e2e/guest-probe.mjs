export const visitorState=page=>page.evaluate(async()=>{
 const name=(await indexedDB.databases()).find(d=>d.name?.startsWith('pinelink-visitor:'))?.name;if(!name)return null
 const db=await new Promise((resolve,reject)=>{const r=indexedDB.open(name);r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error)})
 const get=(table,key)=>new Promise((resolve,reject)=>{const r=db.transaction(table).objectStore(table).get(key);r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error)})
 const all=table=>new Promise((resolve,reject)=>{const r=db.transaction(table).objectStore(table).getAll();r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error)})
 const lease=await get('leases','self'),profile=await get('profiles','self'),jobs=await all('jobs'),peers=await all('guestPeers');db.close()
 return {owner:lease?.owner_id,device:profile?.device,jobs:jobs.map(j=>({id:j.id,name:j.name,phase:j.phase,peer:j.peer,transfer:j.transfer,snapshot:j.snapshot,received:j.received,transport:j.transport,directAttempted:j.directAttempted,error:j.error,failures:j.failures,hasPlain:!!j.plain})),peers}
})
export const visitorRequest=(page,method,path,input)=>page.evaluate(async({method,path,input})=>{
 const name=(await indexedDB.databases()).find(d=>d.name?.startsWith('pinelink-visitor:')).name;const db=await new Promise(resolve=>{const r=indexedDB.open(name);r.onsuccess=()=>resolve(r.result)})
 const get=table=>new Promise(resolve=>{const r=db.transaction(table).objectStore(table).get('self');r.onsuccess=()=>resolve(r.result)})
 const lease=await get('leases'),profile=await get('profiles');db.close();const body=input===undefined?'':JSON.stringify(input),timestamp=Math.floor((Date.now()+lease.offset)/1000).toString(),hash=[...new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(body)))].map(b=>b.toString(16).padStart(2,'0')).join('')
 const signature=btoa(String.fromCharCode(...new Uint8Array(await crypto.subtle.sign('Ed25519',profile.signingPrivate,new TextEncoder().encode(method+'\n'+path+'\n'+timestamp+'\n'+hash)))))
 const response=await fetch('/api'+path,{method,body:method==='GET'?undefined:body,headers:{'Content-Type':'application/json','X-Pinelink-Owner':lease.owner_id,'X-Pinelink-Device':profile.device,'X-Pinelink-Timestamp':timestamp,'X-Pinelink-Signature':signature,'X-Pinelink-CSRF':lease.csrf}})
 return {status:response.status,data:await response.json()}
},{method,path,input})
