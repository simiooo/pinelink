const C = { bg:'#F5F6F7', paper:'#FFFFFF', text:'#30363D', muted:'#626C76', line:'#E2E6EA', accent:'#6B4A34', soft:'#E9E2D9' }
const rgb = hex => { const n=parseInt(hex.slice(1),16); return {r:((n>>16)&255)/255,g:((n>>8)&255)/255,b:(n&255)/255} }
const fill = color => [{type:'SOLID',color:rgb(color)}]
const rect = (parent,name,x,y,w,h,color,radius=0,stroke=false)=>{const n=figma.createRectangle();n.name=name;n.x=x;n.y=y;n.resize(w,h);n.fills=fill(color);n.cornerRadius=radius;if(stroke){n.strokes=[{type:'SOLID',color:rgb(C.line)}];n.strokeWeight=1}parent.appendChild(n);return n}
const text = async(parent,name,value,x,y,size,color=C.text,weight='Regular',width=600)=>{const n=figma.createText();n.name=name;n.x=x;n.y=y;n.characters=value;n.fontSize=size;n.fills=fill(color);n.fontName={family:'Inter',style:weight};n.resize(width,Math.ceil(size*1.6));parent.appendChild(n);return n}
const frame = (parent,name,x,y,w,h,color=C.paper,radius=0,stroke=true)=>{const n=figma.createFrame();n.name=name;n.x=x;n.y=y;n.resize(w,h);n.fills=fill(color);n.cornerRadius=radius;if(stroke){n.strokes=[{type:'SOLID',color:rgb(C.line)}];n.strokeWeight=1}n.clipsContent=true;parent.appendChild(n);return n}
const button = async(parent,label,x,y,w,primary=false)=>{const container=frame(parent,`${label}按钮`,x,y,w,46,primary?C.accent:C.bg,12,!primary);await text(container,`${label}按钮文字`,label,20,13,15,primary?C.paper:C.muted,'Medium',w-40)}
const build = async(root,mobile=false)=>{
  const m=mobile,pad=m?22:36,inner=m?346:928
  await text(root,'品牌','Pinelink',pad,32,m?23:25,C.text,'Bold',120)
  const chipX=m?308:906
  rect(root,'更多按钮背景',chipX,28,58,34,C.bg,12,true)
  await text(root,'更多','更多',chipX+15,36,13,C.muted,'Regular',28)
  rect(root,'分隔线',pad,78,inner,1,C.line)
  await text(root,'页面标题','通知',pad,104,m?23:28,C.text,'Bold',inner)
  await text(root,'页面说明',m?'核对发送设备和文件后，再决定是否接收。':'先核对发送设备、文件与大小，再决定是否接收。',pad,m?138:148,m?12:15,C.muted,'Regular',inner)

  const cardY=m?184:206,cardH=m?252:240
  rect(root,'文件请求卡片',pad,cardY,inner,cardH,C.bg,16,true)
  rect(root,'文件类型底',pad+(m?22:32),cardY+(m?24:32),m?50:56,m?50:56,C.soft,16)
  await text(root,'文件类型','PDF',pad+(m?29:41),cardY+(m?40:50),m?13:15,C.accent,'Medium',40)
  await text(root,'文件名称','设计回顾.pdf',pad+(m?86:112),cardY+(m?24:34),m?16:18,C.text,'Bold',inner-(m?108:144))
  await text(root,'文件大小','2.4 MB',pad+(m?86:112),cardY+(m?52:66),m?12:14,C.muted,'Regular',inner-(m?108:144))
  await text(root,'发送设备标签','发送设备',pad+(m?22:32),cardY+(m?104:118),m?12:13,C.muted,'Regular',100)
  await text(root,'发送设备','MacBook Pro',pad+(m?86:112),cardY+(m?104:118),m?12:13,C.text,'Regular',inner-(m?108:144))
  await text(root,'发送账户标签','发送账户',pad+(m?22:32),cardY+(m?130:144),m?12:13,C.muted,'Regular',100)
  await text(root,'发送账户','team-design',pad+(m?86:112),cardY+(m?130:144),m?12:13,C.text,'Regular',inner-(m?108:144))
  await text(root,'接收提醒',m?'接受后才会获取文件；对方看不到你的其他文件。':'接受后才会获取文件；发送者看不到你的其他文件。',pad+(m?22:32),cardY+(m?180:192),m?12:13,C.muted,'Regular',inner-(m?44:64))

  const actionY=cardY+cardH+(m?20:24),actionW=m?167:180
  await button(root,'拒绝',pad,actionY,actionW,false)
  await button(root,'接受',pad+inner-actionW,actionY,actionW,true)

  rect(root,'临时副本说明底',pad,m?510:518,inner,m?112:74,C.bg,12)
  await text(root,'临时副本说明',m?'若经过中转，将暂存端到端加密副本，\n24 小时后删除。':'若经过中转，将暂存端到端加密副本，24 小时后删除。',pad+(m?14:24),(m?510:518)+(m?14:20),m?12:13,C.muted,'Regular',inner-(m?28:48))
  await text(root,'持久化说明',m?'长期保存需要另行选择。':'长期保存需要另行选择，不会默认开启。',pad+(m?14:24),(m?510:518)+(m?64:44),m?12:13,C.muted,'Regular',inner-(m?28:48))
  await text(root,'操作约束','主屏操作：拒绝 / 接受 / 更多',pad,m?654:640,12,C.muted,'Regular',inner)
}
await figma.loadFontAsync({family:'Inter',style:'Regular'})
await figma.loadFontAsync({family:'Inter',style:'Medium'})
await figma.loadFontAsync({family:'Inter',style:'Bold'})
figma.currentPage.name='02 通知'
for(const child of [...figma.currentPage.children])child.remove()
rect(figma.currentPage,'页面背景',0,0,1536,960,C.bg)
await text(figma.currentPage,'桌面版标注','02 · 通知 / 接收前核对来源',40,28,18,C.text,'Bold',700)
await text(figma.currentPage,'移动端标注','移动端 390 × 844',1092,28,18,C.text,'Bold',300)
const desktop=frame(figma.currentPage,'桌面 · 通知',40,70,1000,790,C.paper,16,true)
await build(desktop,false)
const mobile=frame(figma.currentPage,'移动端 · 通知',1092,70,390,844,C.paper,24,true)
await build(mobile,true)
figma.currentPage.selection=[desktop,mobile]
