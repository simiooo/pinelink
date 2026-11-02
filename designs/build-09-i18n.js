const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const button=async(p,label,x,y,w,primary=false)=>{const f=frame(p,`${label}按钮`,x,y,w,46,primary?C.accent:C.bg,12);if(!primary){f.strokes=fill(C.line);f.strokeWeight=1}await text(f,`${label}文字`,label,20,13,15,primary?C.paper:C.muted,w-40,'Medium');return f}
const base=async(p,m,title,subtitle)=>{
 const pad=m?22:36,inner=m?346:928
 await text(p,'品牌','Pinelink',pad,32,m?23:25,C.text,130,'Bold')
 rect(p,'通知按钮',m?224:824,28,m?66:66,34,C.bg,12,true);await text(p,'通知','Notifications',m?234:834,36,m?12:13,C.muted,m?50:56)
 rect(p,'更多按钮',m?308:906,28,58,34,C.bg,12,true);await text(p,'更多','More',m?323:921,36,13,C.muted,28)
 rect(p,'分隔线',pad,78,inner,1,C.line)
 await text(p,'页面标题',title,pad,104,m?23:28,C.text,inner,'Bold');await text(p,'页面说明',subtitle,pad,m?138:148,m?12:15,C.muted,inner)
}
const build=async(p,m=false)=>{
 const pad=m?22:36,inner=m?346:928
 await base(p,m,'Send files','Choose a file, then choose a recipient. Nothing is sent before approval.')
 rect(p,'文件选择区',pad,m?184:206,inner,m?306:300,C.bg,16,true)
 rect(p,'文件图标底',pad+(m?22:32),(m?184:206)+(m?28:36),m?50:56,m?50:56,C.soft,16)
 await text(p,'文件图标','↓',pad+(m?34:43),(m?184:206)+(m?34:42),m?26:30,C.accent,40)
 await text(p,'文件提示','Drop a file here',pad+(m?22:112),(m?184:206)+(m?100:52),m?16:18,C.text,inner-100,'Bold')
 await text(p,'文件说明','or choose a file',pad+(m?22:112),(m?184:206)+(m?128:84),m?12:14,C.muted,inner-100)
 await button(p,'Choose file',pad+(m?22:32),(m?184:206)+(m?166:128),m?168:180,true)
 await text(p,'发送备注','Any file type. Review the file and recipient before sending.',pad+(m?22:32),(m?184:206)+(m?236:208),m?12:13,C.muted,inner-50)
 rect(p,'最近任务底',pad,m?508:538,inner,m?74:74,C.bg,12)
 await text(p,'最近任务','Recent tasks',pad+(m?14:24),(m?508:538)+(m?14:20),m?14:15,C.text,inner-48,'Bold')
 await text(p,'最近说明','Real progress and status appear here.',pad+(m?14:24),(m?508:538)+(m?42:44),m?12:13,C.muted,inner-48)
 const menuX=m?118:690,menuY=m?22:20,menuW=m?248:274
 rect(p,'更多菜单',menuX,menuY,menuW,224,C.paper,16,true)
 await text(p,'菜单标题','More',menuX+16,menuY+16,14,C.text,menuW-32,'Bold')
 await text(p,'当前任务','Current task',menuX+16,menuY+48,13,C.text,menuW-32)
 await text(p,'历史异常','History & issues',menuX+16,menuY+76,13,C.text,menuW-32)
 rect(p,'菜单分隔',menuX+16,menuY+106,menuW-32,1,C.line)
 await text(p,'语言标签','Language / 语言',menuX+16,menuY+120,12,C.muted,menuW-32)
 rect(p,'语言选择',menuX+16,menuY+148,menuW-32,42,C.bg,8,true)
 await text(p,'语言选项','System / English / 中文',menuX+28,menuY+160,13,C.text,menuW-56)
 await text(p,'语言备注','Saved on this device',menuX+16,menuY+198,12,C.muted,menuW-32)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='09 国际化';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面标注','09 · i18n / English hierarchy + language menu',40,28,18,C.text,700,'Bold')
await text(figma.currentPage,'移动标注','Mobile 390 × 844',1092,28,18,C.text,300,'Bold')
const desktop=frame(figma.currentPage,'桌面 · English',40,70,1000,790,C.paper,16);desktop.strokes=fill(C.line);desktop.strokeWeight=1;await build(desktop)
const mobile=frame(figma.currentPage,'移动端 · English',1092,70,390,844,C.paper,24);mobile.strokes=fill(C.line);mobile.strokeWeight=1;await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
