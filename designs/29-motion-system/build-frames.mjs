import {readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const catalog=JSON.parse(await readFile(new URL('./catalog.json',import.meta.url),'utf8'));
const q=JSON.stringify,c={ink:'#101010',muted:'#6E6E6E',line:'#DBDBDB',surface:'#F5F5F5',blue:'#0866FF',soft:'#EEF5FF',white:'#FFFFFF'};
const t=(s,w=280,size=15,h=24,weight=400,color='ink',x,y,align='left')=>`<Text name=${q(s)} ${x===undefined?'':`x={${x}} y={${y}}`} w={${w}} h={${h}} font="Nunito" size={${size}} lineHeight={${size>=20?28:size===13?18:22}} weight={${weight}} color=${q(c[color])} textAlign=${q(align)} textAutoResize="none">{${q(s)}}</Text>`;
const svg=(path,size=24,color=c.ink)=>`<svg viewBox="0 0 24 24" size={${size}}><path d=${q(path)} stroke=${q(color)} strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" fill="none" /></svg>`;
const paths={phone:'M7 2h10v20H7V2m4 17h2',pc:'M4 3h16v12H4V3m0 12-3 6h22l-3-6',check:'m5 12 4 4L19 6',file:'M5 2h9l5 5v15H5V2m9 0v6h5M8 12h8m-8 4h8',cloud:'M7 19a5 5 0 1 1 0-10 6 6 0 0 1 12 0 5 5 0 0 1 0 10H7',copy:'M8 8h13v13H8V8M3 16V3h13',more:'M4 12h.01m7.99 0h.01m7.99 0h.01',close:'m6 6 12 12M6 18 18 6',clock:'M12 6v6l4 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',warn:'M12 7v6m0 4h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',type:'M4 4h16M12 4v16M8 20h8'};
const frame=(name,children,w,h,x=0,y=0,extra='')=>`<Frame name=${q(name)} x={${x}} y={${y}} w={${w}} h={${h}} ${extra}>${children}</Frame>`;
const icon=(key,size=24,color)=>svg(paths[key]??paths.more,size,color);
paths.pause='M8 5v14M16 5v14';paths.play='M8 5l11 7-11 7V5';
const button=(label,{x=28,y=136,w=280,scale=1,quiet=false,busy=false}={})=>frame('48px target / '+label,frame('Visual button',t(label,w*scale-(busy?96:32),15,24,600,quiet?'ink':'white',busy?48:16,12,'center')+(busy?frame('Busy / true pending',icon('clock',16,quiet?c.ink:c.white),16,16,18,16):''),w*scale,48*scale,(w-w*scale)/2,(48-48*scale)/2,`bg=${q(quiet?c.surface:c.blue)} rounded={24}`),w,48,x,y);
function partialCurve(side,fraction){
 const pts=[];for(let i=0;i<=120;i++){const u=i/120,v=1-u;pts.push([v*v*v*side/2+3*v*u*u*side+u*u*u*side/2,v*v*v*side+3*v*v*u*side*.7+3*v*u*u*side*.3]);}
 const lens=pts.slice(1).map((p,i)=>Math.hypot(p[0]-pts[i][0],p[1]-pts[i][1])),total=lens.reduce((a,b)=>a+b,0),limit=total*fraction;let n=0,d=`M${pts[0].map(x=>x.toFixed(2)).join(' ')}`;
 for(let i=0;i<lens.length;i++){if(n+lens[i]>limit){const k=(limit-n)/lens[i];d+=`L${(pts[i][0]+(pts[i+1][0]-pts[i][0])*k).toFixed(2)} ${(pts[i][1]+(pts[i+1][1]-pts[i][1])*k).toFixed(2)}`;break;}d+=`L${pts[i+1].map(x=>x.toFixed(2)).join(' ')}`;n+=lens[i];}return d;
}
function graph(state){const ack=state==='active'?.42:state==='paused'?.42:state==='confirmed'?1:0,sel=state==='selected',cloud=state==='cloud',top=sel?80:56,labelY=54+top/2+12,high=labelY+28,side=200-high,mid=side/2;
 const curve=`<svg viewBox="0 0 ${side} ${side}" size={${side}}><path d="M${mid} ${side}C0 ${side*.7} ${side} ${side*.3} ${mid} 0" stroke="#E5E5E5" strokeWidth={3} fill="none" />${ack?`<path d=${q(partialCurve(side,ack))} stroke="#0866FF" strokeWidth={3} fill="none" />`:''}</svg>`;
 const badge=state==='confirmed'?frame('Receipt confirmed',icon('check',14,c.blue),18,18,top-18,top-18,'bg="#FFFFFF" rounded={9}'):state==='waiting'||state==='offline'?frame('Availability state / '+state,icon('clock',12,c.muted),16,16,top-16,top-16,'bg="#FFFFFF" rounded={8}'):frame('Online point','',8,8,top-8,top-8,`bg=${q(c.ink)} rounded={4}`);
 const lower=frame('Dimmed context when selected',frame('One connection curve',curve,side,side,164-mid,high,`opacity={${state==='absent'?0:state==='enter'?.6:1}}`)+frame('This device',frame('Device glyph',icon('phone',40),40,40,20,20),80,80,124,200,`bg=${q(c.surface)} rounded={40}`)+t('This device',200,13,20,400,'ink',64,292,'center'),328,320,0,0,`opacity={${sel?.13:1}}`);
 const peer=frame('Peer identity',frame('Peer',frame('Device glyph',icon(cloud?'cloud':'phone',top/2),top/2,top/2,top/4,top/4)+badge,top,top,164-top/2,54-top/2,`bg=${q(c.surface)} rounded={${top/2}}`)+t(cloud?'Cloud':'Alex’s iPhone',200,13,20,400,'ink',64,labelY,'center'),328,140,0,state==='enter'?8:0,`opacity={${state==='absent'?0:state==='enter'?.6:state==='offline'?.4:1}}`);
 return frame('Compact mobile graph / 56:80 nodes / confirmed snapshot',lower+peer,328,320);
}
function field(state){return frame('Field',t('Email',280,13,20,400,'muted',0,0)+frame('Input surface',t('mika@example.com',244,15,24,400,'ink',16,14),280,52,0,28,`bg="#FFFFFF" stroke=${q(state===1?c.blue:state===2?c.ink:c.line)} strokeWidth={${state===1?2:1}} rounded={14}`)+(state===2?t('Check your email address.',280,13,20,400,'ink',0,92):''),280,140,28,80);}
function panel(kind,state){const sheet=kind==='sheet',opacity=state===0?.2:state===1?.7:1,y=sheet?(state===0?72:state===1?58:48):(state===0?60:48);return frame('Active surface',t('Alex’s iPhone',210,20,28,700,'ink',16,20)+frame('Close / 48px target',frame('Close glyph',icon('close',20),20,20,14,14),48,48,264,10)+t('Meet at the north entrance.',280,15,24,400,'ink',16,88)+button('Send',{x:16,y:152,w:280}),312,232,8,y,`bg="#FFFFFF" stroke=${q(c.line)} strokeWidth={1} rounded={24} opacity={${opacity}}`);}
function files(state){
 const all=[['Design source','file'],['Travel notes','type'],['Photos','folder'],['Project brief','file'],['Wi-Fi details','type'],['Iteration notes','file']],visible=state?all.filter((_,i)=>[0,1,5].includes(i)):all;
 const glyph=(key)=>key==='folder'?svg('M2 6h8l2 3h10v15H2V6m0 4h20',36,c.muted):icon(key,36,c.muted);
 return frame('Keyed history content',t('Alex’s iPhone',292,20,28,700,'ink',18,18)+t('All     Reusable     One-time     Expired',292,13,24,400,'muted',18,72)+frame('Active underline','',state?52:24,2,state?56:18,100,`bg=${q(c.blue)}`)+visible.map(([s,key],i)=>frame('Keyed item / '+s,frame('File glyph',glyph(key),36,36,0,0)+t(s,88,13,20,400,'ink',0,48)+frame('Availability icon',icon('cloud',12,c.muted),16,16,26,26,'bg="#FFFFFF" rounded={8}'),88,72,18+i%3*100,124+Math.floor(i/3)*88+(state===1?8:0))).join(''),328,320);
}
function scene(recipe,state){
 if(recipe==='orbit')return frame('Selected peer',frame('Device',frame('Glyph',icon('phone',40),40,40,20,20),80,80,124,104,`bg=${q(c.surface)} stroke=${q(c.blue)} strokeWidth={2} rounded={40}`)+t('Alex’s iPhone',200,13,20,400,'ink',64,196)+(state?[[140,24,'file'],[236,120,'more'],[140,240,'clock'],[44,120,'copy']].map(([x,y,key])=>frame('Orbit action / 48px target',frame('Glyph',icon(key,22),22,22,13,13),48,48,x,y,`bg=${q(c.white)} stroke=${q(c.line)} strokeWidth={1} rounded={24} opacity={${state===1?.55:1}}`)).join(''):''),328,320);
 if(['node','selection','graph','cloud','flow','progress','receipt','badge'].includes(recipe))return graph(recipe==='node'?(state===0?'absent':state===1?'enter':'idle'):recipe==='badge'?(state===0?'idle':state===1?'waiting':'offline'):recipe==='cloud'?'cloud':recipe==='flow'||recipe==='progress'?(state===2?'paused':state===1?'active':'idle'):recipe==='receipt'?(state===2?'confirmed':'idle'):recipe==='selection'&&state>0?'selected':'idle');
 if(recipe==='press')return button('Send',{y:132,scale:state===1?.97:1});
 if(recipe==='icon')return frame('Icon action / 48px target',frame('Glyph',icon(state===0?'pause':state===1?'clock':'play',24),24,24,12,12),48,48,140,132,`bg=${q(state===1?c.surface:c.white)} rounded={24}`);
 if(recipe==='file-badge')return frame('File identity / fixed name and badge host',frame('File icon',icon('file',48,c.muted)+frame('Availability badge',icon(state===0?'clock':state===1?'cloud':'check',14,c.muted),20,20,28,28,'bg="#FFFFFF" rounded={10}'),48,48,0,0)+t('Project brief.pdf',240,13,20,400,'ink',0,60),240,88,28,108);
 if(recipe==='tooltip')return frame('Focused icon',icon('more',24),24,24,152,130)+(state?frame('Tooltip',t('More',64,13,20,400,'white',12,8),88,36,120,state===1?168:164,`bg=${q(c.ink)} rounded={10} opacity={${state===1?.6:1}}`):'');
 if(recipe==='menu')return frame('More trigger',icon('more',24),24,24,250,28)+(state?frame('Menu / anchored to trigger',t('History',168,15,24,400,'ink',16,14)+t('Connect device',168,15,24,400,'ink',16,62)+t('Settings',168,15,24,400,'ink',16,110),200,160,92,state===1?68:60,`bg="#FFFFFF" stroke=${q(c.line)} strokeWidth={1} rounded={20} opacity={${state===1?.7:1}}`):'');
 if(recipe==='tabs')return t('Files',80,15,24,state===0?700:400,state===0?'ink':'muted',28,84)+t('Text',80,15,24,state===2?700:400,state===2?'ink':'muted',132,84)+frame('Shared underline','',state===1?68:32,2,state===0?28:state===1?76:132,120,`bg=${q(c.blue)}`)+t(state===0?'Project brief.pdf':'Meet at the north entrance.',280,15,24,400,'ink',28,168);
 if(recipe==='field'||recipe==='validation')return field(recipe==='validation'?(state===2?2:0):state);
 if(recipe==='check')return frame('48px checkbox target',frame('Checkbox',state?frame('Check path',icon('check',18,c.white),18,18,1,1):'',20,20,14,14,`bg=${q(state?c.blue:c.white)} stroke=${q(state?c.blue:c.line)} strokeWidth={1} rounded={4}`),48,48,22,126)+t('Remember device',236,15,24,400,'ink',78,138);
 if(recipe==='busy')return button(state===0?'Receive':state===1?'Receiving…':'Received',{y:132,busy:state===1});
 if(recipe==='copy')return t('Meet at the north entrance.',280,15,24,400,'ink',28,64)+button(state===0?'Copy text':state===1?'Copying…':'Copied',{y:176,quiet:state!==2});
 if(recipe==='feedback')return t('Meet at the north entrance.',280,15,24,400,'ink',28,70)+(state?frame('Result',icon(state===2?'check':'clock',18),18,18,28,154)+t(state===2?'Saved on this device':'Saving…',244,13,24,400,'ink',58,152):'');
 if(recipe==='drag'||recipe==='drop')return frame('Eligible drop halo','',state===1?124:state===2?108:96,state===1?124:state===2?108:96,164-(state===1?62:state===2?54:48),156-(state===1?62:state===2?54:48),`stroke=${q(state?c.blue:c.line)} strokeWidth={${state?2:1}} rounded={80} bg=${q(state?c.soft:c.white)}`)+frame('Target',frame('Phone glyph',icon('phone',32),32,32,16,16),64,64,132,124,`bg=${q(c.surface)} rounded={32}`)+(state===1?frame('System drag image / before transfer',icon('file',32,c.muted),32,32,78,104):'')+t(state===0?'Alex’s iPhone':state===1?'Release to send':recipe==='drop'?'Review opened':'Alex’s iPhone',280,13,20,400,'ink',28,240);
 if(recipe==='surface'||recipe==='sheet')return panel(recipe,state);
 if(recipe==='navigation')return frame('Page relationship',t(state===0?'Devices':'Alex’s iPhone',272,20,28,700,'ink',16,22)+t(state===0?'This device':'Project brief.pdf',272,15,24,400,'ink',16,108),304,260,state===1?24:8,24,'bg="#FFFFFF"');
 if(recipe==='list')return files(state);
 if(recipe==='queue')return frame('Intent / committed state',icon(state===2?'check':'clock',32,state===2?c.blue:c.muted),32,32,148,80)+t(state===0?'Waiting for a connection':state===1?'Checking the latest state':'Ready to continue',280,15,24,400,'ink',28,148)+button(state===2?'Continue':'Queued',{y:204,quiet:true});
 return t('Use the platform transition',280,15,24,400,'ink',28,132);
}
const frames=[];
function board(key,title,recipe,page,x,y,states=['Before','During','After'],detail=''){
 const r=catalog.recipes[recipe];
 const previews=states.map((label,i)=>`<Frame name=${q(label+' / '+recipe)} w={336} h={392} flex="col" gap={16}><Frame name=${q('Preview / '+label)} w={328} h={320}>${scene(key==='availability'?'file-badge':recipe,i)}</Frame>${t(label,328,13,20,600)}${t(i===1?r.timing:i===2?'Settled state':'Ready',328,13,20,400,'muted')}</Frame>`).join('');
 const jsx=`<Frame name=${q('Motion / '+title)} w={1120} h={640} p={32} flex="col" gap={24} bg="#FFFFFF">${t(title,1056,22,28,700)}${t(detail||r.behavior,1056,13,36,400,'muted')}<Frame name="Keyframes / shared local axes" w={1056} h={392} flex="row" gap={24}>${previews}</Frame>${t('Reduced motion · '+r.reduced,1056,13,40,400,'muted')}</Frame>`;
 frames.push({key,title,recipe,page,x,y,jsx});
}
function doc(key,title,lines,page,x,y){const jsx=`<Frame name=${q(title)} w={1120} h={720} p={32} flex="col" gap={24} bg="#FFFFFF">${t(title,1056,24,32,700)}${lines.map(([a,b])=>`<Frame w={1056} h={88} flex="col" gap={8}>${t(a,1056,18,28,700)}${t(b,1056,15,48)}</Frame>`).join('')}</Frame>`;frames.push({key,title,page,x,y,jsx});}
doc('principles','Motion with a purpose',[
 ['Respond','Press immediately. Keep labels and hit targets stable. Rich feedback follows an interaction or an actual state change.'],
 ['Connect','Origins, nodes, lines and surfaces explain relationships. Back and dismiss reverse the spatial direction.'],
 ['Resolve','A receipt, permission or durable commit determines success. An animation never sends or approves a request.'],
 ['Stay coherent','One shared language: 80 / 120 / 160 / 220 / 280ms; a small spatial spring; one visible activity loop.'],
 ['Be inclusive','Static content and native controls stay readable. Reduced motion removes travel, scale, stagger and loops.'],
],'foundations',0,0);
doc('coverage','Coverage and delivery boundary',[
 ['Source','12 React components, 21 Flutter Widgets, 70 interaction / semantic-control declarations.'],
 ['Latest design','27 nonempty pages, 266 boards, 5,142 container instances. Containers are not independent reusable components.'],
 ['Audit','109 interaction / state targets; 101 animated, 4 platform-owned, 4 stable-content dispositions.'],
 ['Retire','Legacy Courier and CLI preparation flows are inventoried, then replaced rather than decorated.'],
 ['Delivery','This is an editable motion proposal and runnable review prototype. Production feature wiring is not yet delivered.'],
],'foundations',1200,0);
doc('tokens','A small, shared motion vocabulary',[
 ['Feedback','80ms press, 120ms state, 160ms control, 220ms surface, 280ms page. Hit areas remain at least 48px.'],
 ['Space','8px local approach, 24px sheet approach, 16px parent-child page relationship; small spatial overshoot only.'],
 ['Effects','Opacity and colors never overshoot. Progress is monotonic, bounded by real confirmed bytes.'],
 ['Lifecycle','Retarget from the current pose and velocity. Cancel owned animation/listeners; stop invisible activity.'],
 ['Continuity','Tab/list/locale refreshes preserve identity, selection, focus and drafts. Polling does not replay entrances.'],
],'foundations',2400,0);
const specs=[
 ['press','Press and release','press','controls'],['icon','Icon state','icon','controls'],['tooltip','Hover and keyboard focus','tooltip','controls'],['menu','Anchored menu','menu','controls'],['tabs','Tab selection','tabs','controls'],['field','Field focus','field','controls'],['validation','Inline recovery','validation','controls'],['check','Remember device','check','controls'],['busy','Pending and confirmed','busy','controls'],['copy','Copy after success','copy','controls'],
 ['node','Peer arrival and departure','node','graph'],['selection','Select and return','selection','graph'],['badge','Online and offline badges','badge','graph'],['orbit','Orbit actions','orbit','graph'],['drag','Approach and leave a target','drag','graph'],['drop','Drop opens review','drop','graph'],['progress','Real progress and pause','progress','graph'],['route','Route continuity','graph','graph'],['receipt','Verified delivery','receipt','graph'],['cloud','Cloud is a real leg','cloud','graph'],
 ['modal','Desktop modal','surface','content'],['sheet','Mobile sheet','sheet','content'],['navigation','Enter and go back','navigation','content'],['history','Keyed history filters','list','content'],['availability','File availability','badge','content'],['account','Account result feedback','feedback','content'],['offline','Queue and catch up','queue','content']
];
const captions={node:['Before discovery','Device arrives','Available'],badge:['Online','Waiting','Offline'],availability:['Temporary','Cloud copy','On this device'],progress:['Ready','Running · confirmed bytes','Paused · no flow'],route:['Direct','Route changes','Relay · same ledger'],receipt:['Awaiting proof','Verify and commit','Delivered · receipt verified'],cloud:['Cloud source or target','Encrypted staging','Stored is not delivered'],copy:['Ready','Awaiting clipboard result','Copy confirmed']};
const positions={};for(const [key,title,recipe,page]of specs){const i=positions[page]??0;positions[page]=i+1;board(key,title,recipe,page,(i%3)*1200,Math.floor(i/3)*720,captions[key]);}
doc('reduced','Reduced motion keeps meaning',[
 ['Immediate','Keep focus rings, codes, filenames, numbers and text stable. Geometry updates immediately without travel or scale.'],
 ['Equivalent','Retain running/paused semantics, selected outlines, errors, copy results and local/cloud availability.'],
 ['Native','System keyboard, picker, camera permission and install prompt use platform motion and accessibility settings.'],
 ['Live preference','A system preference change stops running motion immediately; it does not remount or resubmit the operation.'],
 ['Input','Keyboard focus has the same affordance as hover. Touch never needs hover, and composing text never triggers submit.'],
],'accessibility',0,0);
doc('interrupt','Interruptions and truthful state',[
 ['Reverse','Rapid open/close and target changes start from the visible pose. One owner prevents old animations from winning.'],
 ['Pause','Stop flow immediately. A real late acknowledgment can correct the static boundary without restarting activity.'],
 ['No authority','Unmount, animation end, timeout or a demo replay never approves, sends, deletes or copies content.'],
 ['Background','Only visible, relevant activity runs. Hidden and offscreen loops stop; dispose cancels owned resources.'],
 ['Prove','Verify reduced motion, rapid reversal, dragleave nesting, focus return, route epochs, stable lists and no API effects.'],
],'accessibility',1200,0);
if(frames.length!==32)throw Error('Expected 32 boards');
await writeFile(process.argv[2]??fileURLToPath(new URL('./frames.json',import.meta.url)),JSON.stringify(frames));
console.log(JSON.stringify({boards:frames.length,pages:Object.fromEntries(Object.entries(positions)),text:'Nunito; fixed explicit line boxes'}));
