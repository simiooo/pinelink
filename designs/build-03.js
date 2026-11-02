const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9',fur:'#C9844A',tail:'#B86F3C',cream:'#F2D4AC'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=Math.round(x);n0.y=Math.round(y);n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(Math.round(w),Math.ceil(size*1.6));p.appendChild(n0);return n0}
const ellipse=(p,n,x,y,w,h,c)=>{const v=figma.createEllipse();v.name=n;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);p.appendChild(v);return v}
const vector=(p,n,x,y,w,h,data,c)=>{const v=figma.createVector();v.name=n;v.vectorPaths=[{windingRule:'NONZERO',data}];v.resize(Math.round(w),Math.round(h));v.x=Math.round(x);v.y=Math.round(y);v.fills=fill(c);p.appendChild(v);return v}
const button=async(p,label,x,y,w,primary=false)=>{const f=frame(p,`${label}按钮`,x,y,w,46,primary?C.accent:C.bg,12);if(!primary){f.strokes=fill(C.line);f.strokeWeight=1}await text(f,`${label}文字`,label,20,13,15,primary?C.paper:C.muted,w-40,'Medium')}
const polygon=(p,n,x,y,w,h,c)=>{const v=figma.createPolygon();v.name=n;v.pointCount=3;v.x=Math.round(x);v.y=Math.round(y);v.resize(Math.round(w),Math.round(h));v.fills=fill(c);p.appendChild(v);return v}
const courier=(p,x,y,s)=>{
 ellipse(p,'顺时针尾卷外轮廓',x,y+20*s,140*s,188*s,C.tail)
 ellipse(p,'尾卷留白',x+24*s,y+44*s,88*s,132*s,C.bg)
 rect(p,'尾卷开口',x+84*s,y+132*s,82*s,90*s,C.bg)
 ellipse(p,'尾尖',x+108*s,y+130*s,26*s,26*s,C.tail)
 ellipse(p,'身体',x+95*s,y+106*s,110*s,124*s,C.fur);ellipse(p,'腹部',x+116*s,y+134*s,68*s,86*s,C.cream)
 ellipse(p,'头部',x+96*s,y+34*s,108*s,108*s,C.fur)
 polygon(p,'左耳',x+98*s,y+14*s,38*s,46*s,C.tail);polygon(p,'右耳',x+166*s,y+14*s,38*s,46*s,C.tail)
 const hair=rect(p,'右倾呆毛',x+146*s,y+10*s,7*s,30*s,C.accent,4);hair.rotation=20
 ellipse(p,'眼睛左',x+125*s,y+79*s,10*s,14*s,C.text);ellipse(p,'眼睛右',x+166*s,y+79*s,10*s,14*s,C.text)
 for(const [dx,dy] of [[107,98],[127,108],[174,98]]){const mark=rect(p,'腮帮坚果壳纹',x+dx*s,y+dy*s,16*s,16*s,C.accent,2);mark.rotation=45}
 rect(p,'包裹',x+166*s,y+143*s,70*s,48*s,'#E7C79B',8)
 polygon(p,'松果封口',x+186*s,y+158*s,18*s,18*s,C.accent)
 ellipse(p,'左脚',x+103*s,y+215*s,32*s,23*s,C.tail);ellipse(p,'右脚',x+158*s,y+215*s,32*s,23*s,C.tail)
}
const build=async(p,m=false)=>{
 const pad=m?22:36,inner=m?346:928
 await text(p,'品牌','Pinelink',pad,32,m?23:25,C.text,130,'Bold')
 rect(p,'更多按钮背景',m?308:906,28,58,34,C.bg,12,true);await text(p,'更多','更多',m?323:921,36,13,C.muted,28)
 rect(p,'分隔线',pad,78,inner,1,C.line)
 await text(p,'页面标题','当前任务',pad,104,m?23:28,C.text,inner,'Bold')
 await text(p,'页面说明',m?'传输路径自动选择，进度来自真实字节。':'传输路径自动选择，状态始终来自真实进度。',pad,m?138:148,m?12:15,C.muted,inner)
 const y=m?184:206
 rect(p,'任务卡片',pad,y,inner,m?300:270,C.bg,16,true)
 await text(p,'文件名称','设计回顾.pdf',pad+(m?22:32),y+(m?24:30),m?16:18,C.text,inner-140,'Bold')
 rect(p,'状态底',pad+inner-(m?86:106),y+(m?22:28),m?64:74,28,C.soft,8)
 await text(p,'任务状态','接收中',pad+inner-(m?76:94),y+(m?29:35),m?12:12,C.accent,52)
 courier(p,pad+(m?16:38),y+(m?76:76),m?.48:.56)
 const progressX=pad+(m?154:202),progressW=inner-(m?176:246)
 await text(p,'百分比','62%',progressX,y+(m?92:100),m?25:32,C.text,progressW,'Bold')
 rect(p,'进度轨道',progressX,y+(m?136:152),progressW,8,C.line,4)
 rect(p,'真实进度',progressX,y+(m?136:152),progressW*.62,8,C.accent,4)
 await text(p,'已接收字节','1.5 MB / 2.4 MB',progressX,y+(m?160:180),m?12:14,C.muted,progressW)
 await text(p,'对方设备','来自 MacBook Pro',progressX,y+(m?184:206),m?12:13,C.muted,progressW)
 await text(p,'角色说明',m?'栗小递正在送，未收到完整确认前不会显示完成。':'栗小递正在送；未收到完整确认前，不会显示完成。',pad+(m?22:32),y+(m?260:232),m?12:12,C.muted,inner-(m?44:64))
 const ay=y+(m?320:294),aw=m?167:180
 await button(p,'暂停',pad,ay,aw,true);await button(p,'取消',pad+inner-aw,ay,aw,false)
 await text(p,'操作约束','主屏操作：暂停 / 取消 / 更多',pad,ay+74,12,C.muted,inner)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='03 当前任务';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面标注','03 · 当前任务 / 真实进度与明确控制',40,28,18,C.text,700,'Bold')
await text(figma.currentPage,'移动标注','移动端 390 × 844',1092,28,18,C.text,300,'Bold')
const desktop=frame(figma.currentPage,'桌面 · 当前任务',40,70,1000,790,C.paper,16);desktop.strokes=fill(C.line);desktop.strokeWeight=1;await build(desktop)
const mobile=frame(figma.currentPage,'移动端 · 当前任务',1092,70,390,844,C.paper,24);mobile.strokes=fill(C.line);mobile.strokeWeight=1;await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
