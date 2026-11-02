const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const build=async(p,m=false)=>{
 const pad=m?22:36,inner=m?346:928
 await text(p,'品牌','Pinelink',pad,32,m?23:25,C.text,130,'Bold')
 rect(p,'通知按钮背景',m?234:834,28,56,34,C.bg,12,true);await text(p,'通知','通知',m?248:848,36,13,C.muted,28)
 rect(p,'更多按钮背景',m?308:906,28,58,34,C.bg,12,true);await text(p,'更多','更多',m?323:921,36,13,C.muted,28)
 rect(p,'分隔线',pad,78,inner,1,C.line)
 await text(p,'页面标题','准备发送',pad,104,m?23:28,C.text,inner,'Bold')
 await text(p,'页面说明',m?'选择接收设备，再复制命令到本机终端。':'选择接收设备，再复制命令到本机终端。对方同意前不会发送内容。',pad,m?138:148,m?12:15,C.muted,inner)
 const y=m?184:206
 rect(p,'文件摘要',pad,y,inner,m?86:90,C.bg,16,true)
 rect(p,'文件图标底',pad+20,y+18,50,50,C.soft,16);await text(p,'文件类型','PDF',pad+29,y+34,13,C.accent,40,'Medium')
 await text(p,'文件名称','设计回顾.pdf',pad+86,y+18,m?15:16,C.text,inner-110,'Bold')
 await text(p,'文件大小','2.4 MB',pad+86,y+46,12,C.muted,inner-110)
 const formY=y+(m?106:114),fieldW=m?inner:440
 const labels=['接收账户','接收设备','文件在终端中的路径'],values=['team-design','macbook-pro','D:\\Transfers\\设计回顾.pdf']
 for(let i=0;i<3;i++){
  const x=m?pad:pad+(i===2?0:(i%2)*(fieldW+24)),fy=m?formY+i*76:formY+(i===2?84:0)
  await text(p,labels[i],labels[i],x,fy,12,C.muted,fieldW)
  rect(p,`${labels[i]}输入框`,x,fy+22,fieldW,42,C.paper,8,true);await text(p,values[i],values[i],x+12,fy+34,m?12:13,C.text,fieldW-24)
 }
 const noteY=m?formY+242:formY+150
 await text(p,'数据面说明',m?'这里不会直接发送文件。请在已登记设备的\n终端执行命令。':'这里不会直接发送文件。请在已登记设备的终端执行命令。',pad,noteY,12,C.muted,inner)
 const button=frame(p,'复制按钮',pad,noteY+(m?54:32),m?inner:200,46,C.accent,12);await text(button,'复制按钮文字','复制发送命令',20,13,15,C.paper,(m?inner:200)-40,'Medium')
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='07 发送准备';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面标注','07 · 发送准备 / 不伪装为已发送',40,28,18,C.text,700,'Bold')
await text(figma.currentPage,'移动标注','移动端 390 × 844',1092,28,18,C.text,300,'Bold')
const desktop=frame(figma.currentPage,'桌面 · 发送准备',40,70,1000,790,C.paper,16);desktop.strokes=fill(C.line);desktop.strokeWeight=1;await build(desktop)
const mobile=frame(figma.currentPage,'移动端 · 发送准备',1092,70,390,844,C.paper,24);mobile.strokes=fill(C.line);mobile.strokeWeight=1;await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
