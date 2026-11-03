// Native OpenPencil source. This is a design proposal, not runtime code.
import {readFile,writeFile} from 'node:fs/promises';
const q=JSON.stringify,v=n=>`{designVar("color/${n}")}`;
function text(s,{size=15,line=20,weight=400,color='ink',width='fill',name=s,extra='',truncate=false,fixed=false}={}){if(s.includes('\n'))return col(name,s.split('\n').map(t=>text(t,{size,line,weight,color,width,extra})),0);return `<Text name=${q((truncate?'Truncate / ':fixed?'Fixed / ':width==='hug'?'Intrinsic / ':'')+name)} font="Nunito" size={${size}} lineHeight={${line}} weight={${weight}} color=${v(color)} w=${typeof width==='number'?`{${width}}`:q(width)} h=${truncate||fixed?`{${line}}`:'"hug"'} ${truncate||fixed?'textAutoResize="none"':''} ${extra}>{${q(s)}}</Text>`;}
function col(name,children,gap=24,extra=''){return `<Frame name=${q(name)} flex="col" w="fill" h="hug" gap={${gap}} ${extra}>${children.join('')}</Frame>`;}
function row(name,children,gap=16,extra=''){return `<Frame name=${q(name)} flex="row" w="fill" h="hug" gap={${gap}} items="center" ${extra}>${children.join('')}</Frame>`;}
const paths={close:'m6 6 12 12M6 18 18 6',camera:'M8 6 10 3h4l2 3h5v14H3V6h5m9 7a5 5 0 1 1-10 0 5 5 0 0 1 10 0',phone:'M7 2h10v20H7V2m4 17h2',pc:'M4 3h16v12H4V3M4 15l-3 6h22l-3-6',type:'M4 4h16M12 4v16M8 20h8',file:'M5 2h9l5 5v15H5V2m9 0v6h5M8 12h8m-8 4h8',folder:'M2 6h8l2 3h10v12H2V6',copy:'M8 8h13v13H8V8M3 16V3h13',check:'m5 12 4 4L19 6',warn:'M12 8v5m0 3h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',clock:'M12 6v6l4 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',repeat:'M3 9h17l-4-4m5 10H4l4 4',link:'m9 15 6-6M8 4 4 8a5 5 0 0 0 7 7m2-6a5 5 0 0 1 7 7l-4 4',person:'M16 6a4 4 0 1 1-8 0 4 4 0 0 1 8 0M3 22v-3a9 9 0 0 1 18 0v3',chevron:'m9 5 7 7-7 7'};
const icon=(name,size=20,color='#101010')=>`<svg viewBox="0 0 24 24" size={${size}}><path d=${q(paths[name])} fill="none" stroke=${q(color)} strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" /></svg>`;
paths.more='M4 12h.01m7.99 0h.01m7.99 0h.01';
function iconButton(name,glyph){if(name==='More')glyph='more';return `<Frame name=${q(name+' / tooltip + accessible name / 48px target')} w={48} h={48} flex="row" items="center" justify="center">${icon(glyph)}</Frame>`;}
function button(label,kind='primary'){const primary=kind==='primary',disabled=kind==='disabled';return `<Frame name=${q('Button / '+label)} w="fill" h={48} flex="row" gap={8} items="center" justify="center" rounded={24} bg=${v(primary?'blue':kind==='quiet'?'white':'surface')}>${label==='Copied'?icon('check',18):''}${text(label,{weight:600,color:primary?'white':disabled?'muted':'ink',width:'hug'})}</Frame>`;}
const error=s=>row('Inline recovery / announced',[icon('warn',18),text(s,{size:13,line:18})],8,'items="start"');
const peerIcon=(glyph='phone',size=64)=>`<Frame name="Peer device icon / key-bound identity" w={${size}} h={${size}} flex="row" items="center" justify="center" bg=${v('surface')} rounded={${size/2}}>${icon(glyph,size/2)}</Frame>`;
const remember=()=>row('Remember device / does not enable automatic receiving',[`<Frame name="Checkbox / off / 48px touch target" w={48} h={48} flex="row" items="center" justify="center"><Frame w={20} h={20} stroke=${v('border')} strokeWidth={1} rounded={4} /></Frame>`,text('Remember device',{size:13,line:18})],4);
function tabs(active='Text',labels=['Files','Text']){const widths={All:24,Reusable:64,'One-time':72,Expired:56,Files:40,Text:32,'Show QR':64,'Paste link':72};return row('Tabs / measured labels and 48px targets',labels.map(label=>`<Frame name=${q('Tab / '+label)} flex="col" w={${Math.max(48,widths[label]+24)}} h={48} gap={0} items="center" justify="end"><Frame flex="row" w="fill" h={46} px={12} items="center" justify="center">${text(label,{weight:active===label?700:400,color:active===label?'ink':'muted',width:widths[label],fixed:true})}</Frame><Frame w={${label.length*7}} h={2} bg=${v(active===label?'blue':'white')} /></Frame>`),8);}
const svg=await readFile(new URL('./invite-preview.svg',import.meta.url),'utf8');const qrPath=svg.match(/<path[^>]*\sd="([^"]+)"/)[1],qrBox=svg.match(/viewBox="([^"]+)"/)[1];
const qr=()=>`<Frame name="Demo QR / inert invitation, not a live grant" flex="row" w="fill" h={224} items="center" justify="center"><svg viewBox=${q(qrBox)} size={224}><path d=${q(qrPath)} fill="#101010" /></svg></Frame>`;
function compare(){return col('Short authentication string / explicitly compared',[text('Compare on both devices',{size:13,line:18,color:'muted'}),text('318 042',{size:28,line:36,weight:600,extra:'letterSpacing={4}'})],8);}
function item(kind='file',name=kind==='text'?'Text':'Project brief.pdf'){return row('Bound transfer offer / no content preview before consent',[`<Frame w={48} h={48} flex="row" items="center" justify="center">${icon(kind==='text'?'type':'file',32,'#808080')}</Frame>`,text(name,{weight:600})],12);}
function editor({empty=false,small=false,read=false,selected=false}={}){const content=empty?[text('Write or paste text',{color:'muted'})]:[text('Meet at the north entrance.',{line:24}),text('I’ll be there at 18:30.',{line:24})];return `<Frame name=${q((read?'Selectable plain text / ':'Text editor / ')+(empty?'empty':'preserve exact line breaks'))} w="fill" h={${small?140:read?180:200}} flex="col" p={${read?0:16}} gap={8} bg=${selected?`"#EEF5FF"`:v('white')} ${read?'':`stroke=${v('border')} strokeWidth={1} rounded={14}`}>${content.join('')}</Frame>`;}
function connectContent(key){
 if(key==='invite')return [row('Invite modes',[tabs('Show QR',['Show QR','Paste link']),iconButton('Scan QR','camera')],8),qr(),row('Invitation lifetime',[icon('clock',16,'#6E6E6E'),text('Expires in 01:42',{size:13,line:18,color:'muted',width:'hug'})],8,'justify="center"'),button('Copy invite','secondary')];
 if(key==='paste')return [tabs('Paste link',['Show QR','Paste link']),col('Invitation input',[text('Invitation',{size:13,line:18,color:'muted'}),`<Frame name="Paste complete invitation / no autofetch" w="fill" h={52} px={16} flex="row" items="center" rounded={14} stroke=${v('border')} strokeWidth={1}>${text('Paste a connection link',{color:'muted'})}</Frame>`],8),button('Continue','disabled')];
 if(key==='scan')return [`<Frame name="Camera viewport / scanning example" w="fill" h={256} bg=${v('surface')} rounded={20} flex="row" items="center" justify="center"><svg viewBox="0 0 200 200" size={200}><path d="M15 55V15h40m90 0h40v40m0 90v40h-40m-90 0H15v-40" fill="none" stroke="#101010" strokeWidth={2} /></svg></Frame>`,text('Scan the other device’s QR',{size:13,line:18,color:'muted'}),button('Paste invite instead','secondary')];
 if(key==='outgoing')return [peerIcon(),text('Waiting for Alex…',{color:'muted'}),button('Cancel','quiet')];
 if(key==='incoming')return [col('Peer and intent',[peerIcon(),text('Allow this device to connect?')],12),col('Connection consent',[remember(),col('Primary and secondary actions',[button('Allow connection'),button('Decline','quiet')],8)],16)];
 if(key==='compare')return [col('Peer and comparison',[peerIcon(),compare()],16),col('Comparison consent',[remember(),col('Primary and secondary actions',[button('Codes match'),button('Don’t match','quiet')],8)],16)];
 if(key==='first-offer')return [col('Identity and bound payload',[peerIcon(),item('file')],16),col('Item consent',[remember(),col('Primary and secondary actions',[button('Receive'),button('Decline','quiet')],8)],16)];
 return [];
}
function recoveryContent(key){const spec={
 'invite-expired':[icon('clock',48,'#808080'),text('Create a new invite to connect.',{color:'muted'}),button('New invite')],
 'camera-denied':[icon('camera',48,'#808080'),text('Camera access wasn’t allowed.',{color:'muted'}),button('Paste invite'),button('Try camera again','quiet')],
 mismatch:[compare(),text('Start a new connection.',{color:'muted'}),button('Try again'),button('Cancel','quiet')],
 rejected:[peerIcon(),button('Try again in 00:59','disabled'),button('Close','quiet')],
 'key-changed':[icon('warn',48),text('Confirm this device again before sending.',{color:'muted'}),button('Connect again'),button('Cancel','quiet')],
 offline:[peerIcon(),button('Try again'),button('Close','quiet')],
 };return spec[key];}
function textContent(key,small=false){
 if(key==='files')return [tabs('Files'),item('file'),button('Add files','secondary'),button('Send')];
 if(['compose','compose-empty','too-long','queued'].includes(key))return [tabs(),editor({empty:key==='compose-empty',small}),...(key==='too-long'?[error('Text is too long. Send it as a file.')]:key==='queued'?[row('Draft state / queued is not delivered',[icon('clock',16,'#6E6E6E'),text('Waiting for a connection',{size:13,line:18,color:'muted'})],8)]:[]),button(key==='too-long'?'Send as file':key==='queued'?'Queue':'Send',key==='compose-empty'?'disabled':'primary')];
 if(key==='receive')return [peerIcon(),item('text'),button('Receive'),button('Decline','quiet')];
 if(['read','copied','copy-failed'].includes(key))return [editor({read:true,selected:key==='copy-failed'}),...(key==='copy-failed'?[error('Select the text to copy.')]:[]),button(key==='copied'?'Copied':key==='copy-failed'?'Select text':'Copy text',key==='copied'?'secondary':'primary')];
 if(key==='expired')return [icon('clock',48,'#A3A3A3'),button('Close','quiet')];
 if(key==='unsupported')return [icon('type',48,'#808080'),text('This device can receive text as a file.',{color:'muted'}),button('Send as file'),button('Cancel','quiet')];
 return [];
}
function graph(mobile,{dim=false}={}){
 const w=mobile?390:1200,h=mobile?844:800,cx=w/2,top=mobile?230:208,bottom=mobile?680:628,a=mobile?28:32,b=mobile?40:48;
 // The curve starts below the peer label's 24px box plus an 8px gap.
 const high=top+a+12+24+8,side=bottom-b-high,mid=side/2;
 const full=`M${mid} ${side}C${mid-120} ${bottom-220-high} ${mid+160} ${top+140-high} ${mid} 0`;
 return `<Frame name="Device graph / current peer" w={${w}} h={${h}} opacity={${dim?.12:1}}>
  <Frame name="Connection curve / bounded square SVG" x={${cx-mid}} y={${high}} w={${side}} h={${side}}><svg viewBox=${q(`0 0 ${side} ${side}`)} size={${side}}><path d=${q(full)} fill="none" stroke="#E5E5E5" strokeWidth={3} /></svg></Frame>
  <Frame x={${cx-a}} y={${top-a}} w={${a*2}} h={${a*2}} rounded={${a}} bg=${v('surface')}><Frame x={${a/2}} y={${a/2}} w={${a}} h={${a}}>${icon('phone',a)}</Frame><Frame name="Other person / tooltip" x={0} y={${a*2-18}} w={18} h={18} bg=${v('white')} rounded={9} flex="row" items="center" justify="center">${icon('person',12,'#6E6E6E')}</Frame><Frame name="Online / accessible state" x={${a*2-10}} y={${a*2-10}} w={8} h={8} bg=${v('ink')} rounded={4} /></Frame>
  <Frame name="Peer label" x={${cx-100}} y={${top+a+12}} w={200} h={24} flex="row" justify="center">${text('Alex’s iPhone',{size:mobile?13:15,line:20,width:'hug'})}</Frame>
  <Frame x={${cx-b}} y={${bottom-b}} w={${b*2}} h={${b*2}} flex="row" items="center" justify="center" rounded={${b}} bg=${v('surface')}>${icon(mobile?'phone':'pc',b)}</Frame>
  <Frame x={${cx-100}} y={${bottom+b+12}} w={200} h={24} flex="row" justify="center">${text('This device',{size:14,line:20,width:'hug'})}</Frame>
 </Frame>`;
}
function appHeader(mobile,w){return `<Frame name="App header / behind active surface" opacity={.14} x={20} y={${mobile?56:24}} w={${w-40}} h={48} flex="row" items="center">${text('pinelink',{size:24,line:32,weight:700})}${iconButton('More','chevron')}</Frame>`;}
const titleFor={invite:'Connect devices',paste:'Connect devices',scan:'Scan QR',outgoing:'Alex’s iPhone',incoming:'Alex’s iPhone',compare:'Alex’s iPhone','first-offer':'Alex’s iPhone','invite-expired':'Invite expired','camera-denied':'Use a connection link',mismatch:'Codes don’t match',rejected:'Connection declined','key-changed':'Device changed',offline:'Can’t reach this device',expired:'Text expired',unsupported:'Send text as a file'};
function keyboard(w){return `<Frame name="System keyboard / viewport only" x={0} y={554} w={${w}} h={290} bg=${v('surface')}><Frame x={8} y={12} w={${w-16}} h={46} bg=${v('white')} rounded={6} flex="row" items="center" justify="center">${text('q   w   e   r   t   y   u   i   o   p',{size:18,line:24,width:'hug'})}</Frame><Frame x={20} y={68} w={${w-40}} h={46} bg=${v('white')} rounded={6} flex="row" items="center" justify="center">${text('a   s   d   f   g   h   j   k   l',{size:18,line:24,width:'hug'})}</Frame><Frame x={36} y={124} w={${w-72}} h={46} bg=${v('white')} rounded={6} flex="row" items="center" justify="center">${text('z   x   c   v   b   n   m',{size:18,line:24,width:'hug'})}</Frame><Frame x={72} y={184} w={${w-144}} h={46} bg=${v('white')} rounded={6} flex="row" items="center" justify="center">${text('space',{size:15,width:'hug'})}</Frame></Frame>`;}
function screen(key,{mobile=false,width=390,area='connect',keyboardOpen=false}={}){
 const w=mobile?width:1200,h=mobile?(width===320?768:844):800,pw=mobile?w:480,x=mobile?0:(w-pw)/2,y=mobile?(keyboardOpen?44:width===320?108:144):100;
 const parts=area==='recovery'?recoveryContent(key):area==='text'?textContent(key,keyboardOpen):connectContent(key);const title=titleFor[key]??'Alex’s iPhone';
 const header=row('Surface title and dismiss',[text(title,{size:20,line:28,weight:700}),iconButton('Close','close')],16);
 const panel=`<Frame name="Active surface / 16-20h 24v insets" ${mobile?`x={${x}} y={${y}}`:''} w={${pw}} h=${mobile?`{${keyboardOpen?510:width===320?660:700}}`:'"hug"'} px={${mobile?16:20}} py={${keyboardOpen?16:24}} flex="col" gap={${keyboardOpen?16:24}} bg=${v('white')} ${mobile?'':`stroke=${v('border')} strokeWidth={1}`} rounded={24}>${header}${col('Semantic content groups',parts,keyboardOpen?16:24)}</Frame>`;
 const surface=mobile?panel:`<Frame name="Modal placement" x={0} y={0} w={${w}} h={${h}} flex="row" items="center" justify="center">${panel}</Frame>`;
 return `<Frame name=${q(`${mobile?'Mobile':'Desktop'} / ${key}${width===320?' / 320px':''}${keyboardOpen?' / keyboard':''}`)} w={${w}} h={${h}} bg=${v('white')} overflow="hidden">${width===390||!mobile?graph(mobile,{dim:true}):''}${appHeader(mobile,w)}${mobile?`<Frame x={20} y={12} w={70} h={20} flex="row">${text('9:41',{size:14,width:'hug'})}</Frame>`:''}${surface}${keyboardOpen?keyboard(w):''}${mobile?`<Frame x={${(w-126)/2}} y={${h-20}} w={126} h={4} rounded={2} bg=${v('ink')} />`:''}</Frame>`;
}
const frames=[];
function addPair(key,page,area=page){const n=frames.filter(f=>f.page===page&&!f.key.includes('contract')).length/2;for(const mobile of [false,true])frames.push({key:(mobile?'mobile':'desktop')+'-'+key,page,x:(n%3)*1740+(mobile?1264:0),y:Math.floor(n/3)*980,jsx:screen(key,{mobile,area})});}
for(const key of ['invite','paste','scan','outgoing','incoming','compare','first-offer'])addPair(key,'connect');
for(const [i,key]of ['invite-expired','camera-denied','mismatch','rejected','key-changed','offline'].entries())frames.push({key:'mobile-'+key,page:'recovery',x:(i%4)*480,y:Math.floor(i/4)*980,jsx:screen(key,{mobile:true,area:'recovery'})});
for(const [i,key]of ['mismatch','key-changed'].entries())frames.push({key:'desktop-'+key,page:'recovery',x:2400,y:i*980,jsx:screen(key,{area:'recovery'})});
for(const key of ['files','compose-empty','compose','receive','read','copied','copy-failed','queued'])addPair(key,'text');
for(const [i,key]of ['too-long','expired','unsupported'].entries())frames.push({key:'mobile-'+key,page:'text',x:i*480,y:2940,jsx:screen(key,{mobile:true,area:'text'})});
frames.push({key:'mobile-compose-keyboard',page:'text',x:1440,y:2940,jsx:screen('compose',{mobile:true,area:'text',keyboardOpen:true})},{key:'mobile-compose-320',page:'text',x:1920,y:2940,jsx:screen('compose',{mobile:true,width:320,area:'text'})});
function history(mobile){
 const w=mobile?390:1200,h=mobile?844:800,pw=mobile?390:528,cw=mobile?106:144,gap=mobile?20:32;
 const cells=[['file','Design source.fig','clock'],['type','Meet at the north entrance','check'],['folder','Photos','repeat'],['type','Travel checklist','repeat'],['file','Project brief.pdf','check'],['type','Wi-Fi details','clock']];
 const rows=[];
 for(let n=0;n<2;n++)rows.push(row('Icon row',cells.slice(n*3,n*3+3).map(([glyph,label,badge],i)=>{const display=label==='Meet at the north entrance'?(mobile?'Meet at the…':'Meet at the north…'):label;return `<Frame name=${q('History identity / '+label)} w={${cw}} h={88} flex="col" gap={8} ${n===1&&i===2?'opacity={.38}':''}><Frame w={48} h={48}>${icon(glyph,40,'#808080')}<Frame name=${q('Availability badge / tooltip / '+badge)} x={28} y={28} w={20} h={20} flex="row" items="center" justify="center" bg=${v('white')} rounded={10}>${icon(badge,14,'#6E6E6E')}</Frame></Frame>${text(display,{name:label,size:13,line:20,width:cw,truncate:true})}</Frame>`;}),gap));
 const panel=`<Frame name="History panel / local left axes" x={${mobile?0:640}} y={${mobile?44:144}} w={${pw}} h={${mobile?800:464}} px={16} py={${mobile?8:24}} flex="col" gap={${mobile?24:32}} rounded={${mobile?0:24}} bg=${v('white')} ${mobile?'':`stroke=${v('border')} strokeWidth={1}`}>${row('History title',[text('Alex’s iPhone',{size:20,line:28,weight:700}),iconButton('Close','close')],16)}${tabs('All',['All','Reusable','One-time','Expired'])}${col('File and text identities',rows,32)}</Frame>`;
 const system=mobile?`<Frame x={20} y={12} w={70} h={20} flex="row">${text('9:41',{size:14,width:'hug'})}</Frame><Frame x={132} y={824} w={126} h={4} bg=${v('ink')} rounded={2} />`:graph(false,{dim:true})+appHeader(false,w);
 return `<Frame name=${q((mobile?'Mobile':'Desktop')+' / Mixed history')} w={${w}} h={${h}} bg=${v('white')}>${system}${panel}</Frame>`;
}
for(const mobile of [false,true])frames.push({key:(mobile?'mobile':'desktop')+'-history',page:'text',x:mobile?3664:2400,y:2940,jsx:history(mobile)});
// Import each editable SVG header into its connected graph, then reparent the
// imported outer GROUP into that same graph and remove the empty import FRAME.
// The importer gives FRAME a white fill, and this version ignores fill alpha.
// Final GROUP positions: desktop (38.666668,24), mobile (12,11.65).
// Cross-page native cloning corrupted CanvasKit; SVG keeps the accepted axes.
for(const mobile of [false,true])frames.push({key:(mobile?'mobile':'desktop')+'-connected-graph',page:'connect',x:mobile?4744:3480,y:1960,headerAsset:(mobile?'mobile':'desktop')+'-accepted-header.svg',headerX:mobile?12:37.46666793823242,headerY:mobile?11.649999618530273:24,jsx:`<Frame name=${q((mobile?'Mobile':'Desktop')+' / Connected peer graph')} w={${mobile?390:1200}} h={${mobile?844:800}} bg=${v('white')}>${graph(mobile)}</Frame>`});
function contract(key,title,blocks,x,y,h=880){frames.push({key,page:'contract',x,y,jsx:`<Frame name=${q('Contract / '+title)} w={800} h={${h}} p={32} flex="col" gap={24} bg=${v('white')}>${text(title,{size:28,line:36,weight:700})}${blocks.map(([title,body])=>col(title,[text(title,{size:18,line:24,weight:700}),text(body,{line:24})],8)).join('')}</Frame>`});}
contract('consent-contract','Connect, then receive',[
 ['QR invitation','Scan or paste a bounded single-use invite. Its keys and target device are pinned before connecting. No public account directory or email disclosure.'],
 ['Two decisions','A remembered connection permits requests, not automatic receiving. When a payload is already offered, one Receive action may confirm the key-bound connection and that specific payload.'],
 ['Both sides matter','QR scanning is the initiator’s explicit action; the other device approves. Nearby/manual comparison requires both users to confirm a transcript-bound code. Never hash public metadata and call it a secure handshake.'],
 ['Trust is scoped','Service origin + account + device signing/encryption keys. Key change, cancellation, expiry and revocation are enforced atomically and during active I/O.'],
 ['Offline stays local','Native LAN uses its local trust/session path without cloud availability. Web exposes only an actually available bridge/transport. A new service never inherits existing account credentials.'],
 ],0,0);
contract('payload-contract','Files and text share delivery',[
 ['Explicit payload kind','FILE or TEXT in the authenticated encrypted manifest; text/plain UTF-8, exact bytes and line breaks, initial 256 KiB bound. Keep original text out of cloud filenames, logs and notifications.'],
 ['The same receipt','Direct and relay use receiver-confirmed bytes and verified local commit. Cloud Stored is distinct from Delivered. Copying text is a separate user action, never a delivery signal.'],
 ['Copy without surprises','No automatic clipboard read/write, URL navigation or HTML rendering. Clipboard failure keeps selectable text and a manual-copy action.'],
 ['History and expiry','Type icon + local title + small availability badge. Expired cloud references do not invalidate an existing local copy; one-time storage cannot prevent a recipient from copying accepted bytes.'],
 ['Cross-user isolation','Connection and item grants do not expose another account’s roster, other files, encryption keys or complete history. Refuse/expiry revokes this delivery’s reference atomically.'],
 ],880,0);
contract('motion-contract','Motion follows confirmed state',[
 ['Connect','Pending has a quiet activity cue. Only confirmed mutual authorization reveals the peer: opacity 0–1, Y 8–0, 180 ms. Error replacement 160 ms; no success timer fabricates a connection.'],
 ['Surfaces','Desktop modal: 160 ms opacity/scale. Mobile sheet: 220 ms with 24px travel; exit 160 ms. Keyboard follows the OS and keeps editor/actions reachable.'],
 ['Transfer','Blue follows actual receiver-confirmed arc length; the 1.8 s flow stays inside it. Pause freezes flow and preserves the boundary. Text may finish immediately after its real receipt.'],
 ['Copy and access','Copied appears only after clipboard success; announce it, then restore the action. 48px targets, visible focus, Enter submits only the focused form.'],
 ['Reduced motion and races','Immediate state changes with static busy state. No auth/invite/send/receive mutation from a React mount effect; use server idempotency, revision checks and account epochs.'],
 ],1760,0);
if(process.argv[2])await writeFile(process.argv[2],JSON.stringify(frames));else process.stdout.write(JSON.stringify(frames));
