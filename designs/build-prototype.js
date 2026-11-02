const C={bg:'#F5F6F7',paper:'#FFFFFF',text:'#30363D',muted:'#626C76',line:'#E2E6EA',accent:'#6B4A34',soft:'#E9E2D9'}
const rgb=h=>{const n=parseInt(h.slice(1),16);return{r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255}}
const fill=c=>[{type:'SOLID',color:rgb(c)}]
const rect=(p,n,x,y,w,h,c,r=0,s=false)=>{const v=figma.createRectangle();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;if(s){v.strokes=fill(C.line);v.strokeWeight=1}p.appendChild(v);return v}
const frame=(p,n,x,y,w,h,c=C.paper,r=0)=>{const v=figma.createFrame();v.name=n;v.x=x;v.y=y;v.resize(w,h);v.fills=fill(c);v.cornerRadius=r;v.clipsContent=true;p.appendChild(v);return v}
const text=async(p,n,v,x,y,size,c=C.text,w=600,weight='Regular')=>{const n0=figma.createText();n0.name=n;n0.x=x;n0.y=y;n0.characters=v;n0.fontSize=size;n0.fills=fill(c);n0.fontName={family:'Inter',style:weight};n0.resize(w,Math.ceil(size*1.6));p.appendChild(n0);return n0}
const button=async(p,label,x,y,w,primary=false)=>{const f=frame(p,`${label}操作`,x,y,w,46,primary?C.accent:C.bg,12);if(!primary){f.strokes=fill(C.line);f.strokeWeight=1}await text(f,`${label}文字`,label,20,13,15,primary?C.paper:C.muted,w-40,'Medium');return f}
const base=async(p,title,sub)=>{
 await text(p,'品牌','Pinelink',22,32,23,C.text,130,'Bold');rect(p,'更多按钮背景',308,28,58,34,C.bg,12,true);await text(p,'更多','更多',323,36,13,C.muted,28)
 rect(p,'分隔线',22,78,346,1,C.line);await text(p,'页面标题',title,22,112,23,C.text,346,'Bold');await text(p,'页面说明',sub,22,146,12,C.muted,346)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'});await figma.loadFontAsync({family:'Inter',style:'Medium'});await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='08 交互流程（静态）';for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1830,980,C.bg)
await text(figma.currentPage,'原型说明','08 · 交互流程 / 真实点击由 Web 与 Flutter 验收',40,28,18,C.text,1300,'Bold')
const home=frame(figma.currentPage,'首页 · 选择文件',40,80,390,844,C.paper,24);await base(home,'发送文件','对方同意前不会发送内容。')
rect(home,'文件拖放区',22,190,346,300,C.bg,16,true);rect(home,'文件图标底',44,218,50,50,C.soft,16);await text(home,'文件提示','把文件拖到这里',44,286,16,C.text,280,'Bold');await text(home,'文件辅助','或点击选择文件',44,314,12,C.muted,280)
const choose=await button(home,'选择文件',44,354,180,true);const notify=await button(home,'通知',278,32,90,false)
const prepare=frame(figma.currentPage,'发送准备',490,80,390,844,C.paper,24);await base(prepare,'准备发送','复制命令到本机终端，不代表已发送。')
rect(prepare,'文件摘要',22,190,346,86,C.bg,16);await text(prepare,'文件名称','设计回顾.pdf',44,212,16,C.text,300,'Bold');await text(prepare,'文件大小','2.4 MB',44,240,12,C.muted,300)
for(let i=0;i<3;i++){const label=['接收账户','接收设备','文件路径'][i];await text(prepare,label,label,22,310+i*76,12,C.muted,346);rect(prepare,`${label}输入框`,22,332+i*76,346,42,C.paper,8,true)}
const copy=await button(prepare,'复制发送命令',22,574,200,true)
const notice=frame(figma.currentPage,'通知 · 接受或拒绝',940,80,390,844,C.paper,24);await base(notice,'通知','核对发送设备和文件后，再决定是否接收。')
rect(notice,'请求卡片',22,190,346,252,C.bg,16,true);await text(notice,'文件名称','设计回顾.pdf',44,214,16,C.text,300,'Bold');await text(notice,'文件大小','2.4 MB',44,242,12,C.muted,300);await text(notice,'来源','ui-macbook',44,296,13,C.text,300)
const reject=await button(notice,'拒绝',22,468,167,false);const accept=await button(notice,'接受',201,468,167,true)
const task=frame(figma.currentPage,'任务 · 暂停与继续',1390,80,390,844,C.paper,24);await base(task,'当前任务','进度来自真实字节，路径切换不改变叙事。')
rect(task,'任务卡片',22,190,346,300,C.bg,16,true);await text(task,'文件名称','设计回顾.pdf',44,214,16,C.text,280,'Bold');await text(task,'进度','0%',44,284,25,C.text,300,'Bold');rect(task,'进度轨道',44,334,302,8,C.line,4);await text(task,'真实进度','0 B / 2.4 MB',44,360,12,C.muted,300)
const pause=await button(task,'暂停',22,516,167,true);const cancel=await button(task,'取消',201,516,167,false)
await text(figma.currentPage,'选择到准备','选择文件 →',440,480,13,C.muted,60)
await text(figma.currentPage,'终端发送到通知','CLI 发送 →',890,480,13,C.muted,60)
await text(figma.currentPage,'接受到任务','接受 →',1340,480,13,C.muted,50)
await text(figma.currentPage,'工具边界','OpenPencil 当前不持久化点击链接；此稿只描述流程，真实交互由双端实现与验收。',40,948,12,C.muted,1300)
figma.currentPage.selection=[home,prepare,notice,task]
