const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9',fur:'#C9844A',tail:'#B86F3C',cream:'#F2D4AC'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const ellipse=(p,n,x,y,w,h,c)=>{const v=figma.createEllipse();v.name=n;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);p.appendChild(v);return v}
const polygon=(p,n,x,y,w,h,c)=>{const v=figma.createPolygon();v.name=n;v.pointCount=3;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);p.appendChild(v);return v}
const courier=(p,x,y,s)=>{
 ellipse(p,'顺时针尾卷外轮廓',x,y+20*s,140*s,188*s,C.tail);ellipse(p,'尾卷留白',x+24*s,y+44*s,88*s,132*s,C.bg);rect(p,'尾卷开口',x+84*s,y+132*s,82*s,90*s,C.bg);ellipse(p,'尾尖',x+108*s,y+130*s,26*s,26*s,C.tail)
 ellipse(p,'身体',x+95*s,y+106*s,110*s,124*s,C.fur);ellipse(p,'腹部',x+116*s,y+134*s,68*s,86*s,C.cream);ellipse(p,'头部',x+96*s,y+34*s,108*s,108*s,C.fur)
 polygon(p,'左耳',x+98*s,y+14*s,38*s,46*s,C.tail);polygon(p,'右耳',x+166*s,y+14*s,38*s,46*s,C.tail)
 const hair=rect(p,'右倾呆毛',x+146*s,y+10*s,7*s,30*s,C.accent,4);hair.rotation=20
 ellipse(p,'眼睛左',x+125*s,y+79*s,10*s,14*s,C.text);ellipse(p,'眼睛右',x+166*s,y+79*s,10*s,14*s,C.text)
 for(const [dx,dy] of [[107,98],[127,108],[174,98]]){const mark=rect(p,'腮帮坚果壳纹',x+dx*s,y+dy*s,14*s,14*s,C.accent,2);mark.rotation=45}
 rect(p,'包裹',x+166*s,y+143*s,70*s,48*s,'#E7C79B',8);polygon(p,'松果封口',x+186*s,y+158*s,18*s,18*s,C.accent)
 ellipse(p,'左脚',x+103*s,y+215*s,32*s,23*s,C.tail);ellipse(p,'右脚',x+158*s,y+215*s,32*s,23*s,C.tail)
}
const states=[
 ['01 跑','RUNNING','1.8s 循环；只表示真实传输中。','run'],
 ['02 停一下','PAUSED','0.4–0.6s；保留真实进度。','pause'],
 ['03 塞进格子','UPLOAD COMPLETED','0.5s；不代表对方已经收到。','store'],
 ['04 吹口哨','BROADCAST PENDING','0.3s；只一圈涟漪，不画拓扑。','notify'],
 ['05 探头','PULL START','0.5s；接收端决定开始获取。','pull'],
 ['06 贴贴纸','CONTENT SEALED','0.2s；不展示加密算法名。','seal'],
 ['07 上云','PROMOTE SUCCESS','0.5s；只有提升成功才播放。','persist'],
 ['08 保管签','AUTHORIZED','0.4s；真实权限结果决定动作。','permission'],
 ['09 看门点头/摇头','ALLOW / DENIED','0.2s；必须同时给出可读原因。','deny'],
]
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='06 栗小递动作契约';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1240,900,C.paper)
await text(figma.currentPage,'标题','栗小递 · 九动作契约',40,28,25,C.text,700,'Bold')
await text(figma.currentPage,'原则','动作只服从真实状态。直连与中继切换不触发差异动画，减少动态效果时展示静态终态。',40,70,14,C.muted,1120)
const nodes=[]
for(let i=0;i<states.length;i++){
 const [title,event,note,action]=states[i],x=40+(i%3)*400,y=112+Math.floor(i/3)*250
 const card=frame(figma.currentPage,title,x,y,360,222,C.bg,16);card.strokes=fill(C.line);card.strokeWeight=1;nodes.push(card)
 courier(card,16,52,.53)
 await text(card,'动作名称',title,154,22,16,C.text,186,'Bold')
 await text(card,'触发事件',event,154,54,12,C.accent,186,'Medium')
 await text(card,'时长与语义',note,154,84,12,C.muted,180)
 if(action==='notify'){const ripple=ellipse(card,'单圈涟漪',x?114:114,91,56,56,C.accent);ripple.fills=[];ripple.strokes=fill(C.accent);ripple.strokeWeight=2;ripple.opacity=.35}
 if(action==='store'||action==='persist'){rect(card,'云朵格',94,50,54,44,C.soft,12);rect(card,'格子封口',111,60,20,24,C.paper,4,true)}
 if(action==='permission'||action==='deny'){polygon(card,'看门小帽',79,75,58,30,C.accent)}
}
await text(figma.currentPage,'交互契约','动画是状态的可视化，不是传输时钟；完成必须由完整校验与交付确认驱动。',40,870,13,C.muted,1120)
figma.currentPage.selection=nodes
