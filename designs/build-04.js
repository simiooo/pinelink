const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9',danger:'#85463F'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const button=async(p,label,x,y,w,primary=false)=>{const f=frame(p,`${label}按钮`,x,y,w,46,primary?C.accent:C.bg,12);if(!primary){f.strokes=fill(C.line);f.strokeWeight=1}await text(f,`${label}文字`,label,20,13,15,primary?C.paper:C.muted,w-40,'Medium')}
const build=async(p,m=false)=>{
 const pad=m?22:36,inner=m?346:928
 await text(p,'品牌','Pinelink',pad,32,m?23:25,C.text,130,'Bold')
 rect(p,'更多按钮背景',m?308:906,28,58,34,C.bg,12,true);await text(p,'更多','更多',m?323:921,36,13,C.muted,28)
 rect(p,'分隔线',pad,78,inner,1,C.line)
 await text(p,'页面标题','历史与异常',pad,104,m?23:28,C.text,inner,'Bold')
 await text(p,'页面说明',m?'删除只影响默认列表，不会抹除记录。':'已完成、暂停与异常都会保留；删除只影响默认列表。',pad,m?138:148,m?12:15,C.muted,inner)
 const fy=m?184:202
 rect(p,'筛选区域',pad,fy,inner,m?76:62,C.bg,12)
 await text(p,'状态标签','状态',pad+14,fy+14,12,C.muted,42)
 rect(p,'状态选择',pad+56,fy+10,m?100:132,36,C.paper,8,true);await text(p,'全部状态','全部状态',pad+68,fy+20,12,C.text,m?80:112)
 await text(p,'类型标签','类型',pad+(m?172:212),fy+14,12,C.muted,42)
 rect(p,'类型选择',pad+(m?214:254),fy+10,m?100:132,36,C.paper,8,true);await text(p,'全部类型','全部类型',pad+(m?226:266),fy+20,12,C.text,m?80:112)
 await text(p,'筛选说明',m?'选择一条记录查看详情':'选择一条记录查看详情，次要操作收进更多。',pad+14,fy+48,m?12:12,C.muted,inner-28)
 const y=fy+(m?96:82)
 await text(p,'记录标题','设计回顾.pdf',pad,y,m?16:18,C.text,inner-200,'Bold')
 rect(p,'记录状态底',pad+inner-86,y-2,86,28,C.soft,8);await text(p,'异常状态','未完成',pad+inner-74,y+5,12,C.danger,64)
 rect(p,'记录卡片',pad,y+38,inner,m?232:214,C.bg,16,true)
 const labels=['方向','对方设备','已传输','时间'],values=['接收','MacBook Pro','1.5 MB / 2.4 MB','今天 10:32']
 for(let i=0;i<4;i++){
  const yy=y+62+i*32
  await text(p,`${labels[i]}标签`,labels[i],pad+22,yy,12,C.muted,m?76:110)
  await text(p,labels[i],values[i],pad+(m?98:144),yy,m?12:13,C.text,inner-(m?122:172))
 }
 await text(p,'异常原因','对方取消了这次传输。文件未标记为已收到。',pad+22,y+(m?198:194),m?12:13,C.danger,inner-44)
 await button(p,'从列表删除',pad,y+(m?292:278),m?167:180,false)
 await text(p,'删除说明','再次点击确认；仅停止或完成后可删除。',pad,y+(m?348:334),12,C.muted,inner)
 await text(p,'操作约束','主屏操作：删除 / 更多；筛选使用选择器',pad,y+(m?390:376),12,C.muted,inner)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='04 历史与异常';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面标注','04 · 历史与异常 / 原因可读，记录可审计',40,28,18,C.text,700,'Bold')
await text(figma.currentPage,'移动标注','移动端 390 × 844',1092,28,18,C.text,300,'Bold')
const desktop=frame(figma.currentPage,'桌面 · 历史与异常',40,70,1000,790,C.paper,16);desktop.strokes=fill(C.line);desktop.strokeWeight=1;await build(desktop)
const mobile=frame(figma.currentPage,'移动端 · 历史与异常',1092,70,390,844,C.paper,24);mobile.strokes=fill(C.line);mobile.strokeWeight=1;await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
