const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const build=async(p,m=false)=>{
 const pad=m?22:36,inner=m?346:928
 await text(p,'品牌','Pinelink',pad,32,m?23:25,C.text,130,'Bold')
 rect(p,'更多按钮背景',m?308:906,28,58,34,C.bg,12,true);await text(p,'更多','更多',m?323:921,36,13,C.muted,28)
 rect(p,'分隔线',pad,78,inner,1,C.line)
 await text(p,'页面标题','小窝与云仓',pad,104,m?23:28,C.text,inner,'Bold')
 await text(p,'页面说明',m?'查看此设备、已登记设备与保管空间。':'查看此设备、已登记设备与保管空间；更改保管方式需明确操作。',pad,m?138:148,m?12:15,C.muted,inner)
 await text(p,'此设备标题','此设备',pad,m?184:206,m?16:17,C.text,inner,'Bold')
 rect(p,'此设备卡片',pad,m?212:236,inner,m?120:94,C.bg,16,true)
 await text(p,'设备名称','dev-device',pad+22,m?234:256,m?14:15,C.text,inner-44,'Medium')
 await text(p,'设备状态','当前使用的设备 · 已登记',pad+22,m?262:284,12,C.muted,inner-44)
 await text(p,'设备标识','账户 dev-owner',pad+22,m?288:310,12,C.muted,inner-44)
 await text(p,'云仓标题','保管空间',pad,m?360:366,m?16:17,C.text,inner,'Bold')
 rect(p,'空间卡片',pad,m?390:396,inner,m?152:142,C.bg,16,true)
 await text(p,'空间用量','2.4 GB / 10 GB',pad+22,m?414:420,m?16:18,C.text,inner-44,'Bold')
 rect(p,'用量轨道',pad+22,m?448:454,inner-44,8,C.line,4);rect(p,'实际用量',pad+22,m?448:454,(inner-44)*.24,8,C.accent,4)
 await text(p,'空间说明',m?'临时副本到期自动删除。\n长期保存需要在文件上另行选择。':'临时副本到期自动删除；长期保存需要在文件上另行选择。',pad+22,m?472:482,12,C.muted,inner-44)
 await text(p,'设备列表标题','已登记设备',pad,m?574:574,m?16:17,C.text,inner,'Bold')
 rect(p,'设备选择底',pad,m?606:606,inner,m?54:56,C.bg,12,true)
 await text(p,'设备选择','MacBook Pro · 最近活动于今天 10:24',pad+16,m?623:624,m?12:13,C.text,inner-32)
 await text(p,'保管提示','设备撤销会影响其未完成的传输，请在更多菜单中谨慎操作。',pad,m?684:692,12,C.muted,inner)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='05 小窝与云仓';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面标注','05 · 小窝与云仓 / 保管状态可见',40,28,18,C.text,700,'Bold')
await text(figma.currentPage,'移动标注','移动端 390 × 844',1092,28,18,C.text,300,'Bold')
const desktop=frame(figma.currentPage,'桌面 · 小窝与云仓',40,70,1000,790,C.paper,16);desktop.strokes=fill(C.line);desktop.strokeWeight=1;await build(desktop)
const mobile=frame(figma.currentPage,'移动端 · 小窝与云仓',1092,70,390,844,C.paper,24);mobile.strokes=fill(C.line);mobile.strokeWeight=1;await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
