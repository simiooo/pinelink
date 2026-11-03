// Small editable OpenPencil companion to v27. English avoids CJK font fallback.
import { mkdir, writeFile } from 'node:fs/promises'
const directory = new URL('./', import.meta.url)
await mkdir(directory, { recursive: true })
const ink='#101010', muted='#6e6e6e', blue='#0866ff'
const text=(s,x,y,size=14,weight=400,color=ink)=>`<text x="${x}" y="${y}" font-family="Nunito" font-size="${size}" font-weight="${weight}" fill="${color}">${s}</text>`
const button=(label,x,y,w,primary=false)=>`${primary?`<rect x="${x}" y="${y}" width="${w}" height="48" rx="24" fill="${blue}"/>`:''}<text x="${x+w/2}" y="${y+30}" text-anchor="middle" font-family="Nunito" font-size="15" font-weight="700" fill="${primary?'#fff':muted}">${label}</text>`
const scenes=[]
for(const [key,kind,w,h] of [['desktop-approval','approval',1200,800],['mobile-approval','approval',390,844],['mobile-app-wait','wait',390,844],['mobile-browser-return','return',390,844],['mobile-browser-cancelled','cancelled',390,844]]){
 const desktop=w>400,x=desktop?420:20,cw=desktop?360:350,y=desktop?156:132
 let body=text('Pinelink',desktop?32:20,48,20,800)
 body+=`<g fill="${ink}"><circle cx="${w-42}" cy="38" r="2"/><circle cx="${w-34}" cy="38" r="2"/><circle cx="${w-26}" cy="38" r="2"/></g>`
 if(kind==='approval')body+=text('Sign in to your device',x,y+32,desktop?32:28,800)+text('Pinelink Linux',x,y+80,14,400,muted)+text('1234 5678',x,y+144,32,800)+text('Match this code with the code in your app.',x,y+188,14,400,muted)+button('Approve sign-in',x,y+220,cw,true)+button('Cancel',x,y+284,cw)
 else if(kind==='wait')body+=text('Waiting for approval',x,y+32,28,800)+text('1234 5678',x,y+108,32,800)+`<path d="M182 300a12 12 0 1 1 16 16m0-8v8h-8" stroke="${muted}" stroke-width="2" fill="none"/>`+button('Cancel',x,y+232,cw)
 else if(kind==='return')body+=text('Return to Pinelink',x,y+32,28,800)+`<path d="M${x+2} ${y+82}l8 8 18-20" stroke="${blue}" stroke-width="2.5" fill="none"/>`
 else body+=text('Sign-in cancelled',x,y+32,28,800)+text('Return to Pinelink',x,y+80,14,400,muted)
 if(kind!=='wait')body+=text('Terms',w/2-52,h-52,12,400,muted)+text('Privacy',w/2+12,h-52,12,400,muted)
 const svg=`<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}"><rect width="${w}" height="${h}" fill="#fff"/>${body}</svg>`
 await writeFile(new URL(key+'.svg',directory),svg)
 scenes.push({key,name:key.replaceAll('-',' '),width:w,height:h,x:scenes.length?1264+(scenes.length-1)*454:0,y:0})
}
await writeFile(new URL('manifest.json',directory),JSON.stringify({scope:'v27 browser/native handoff companion',status:'implementation-review',frames:scenes},null,2)+'\n')
