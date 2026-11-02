from pathlib import Path
from html import escape
import json, sys, math, re
from PIL import ImageFont

ENGLISH='--english' in sys.argv
OUT = Path(__file__).parent / ('13-blue-graph-en' if ENGLISH else '13-blue-graph')
OUT.mkdir(exist_ok=True)
EN={
 '设备':'Devices','4 台':'4 devices','本机':'This device','小窝云仓':'Cloud vault',
 '发送文件':'Send files','传输记录':'Transfer history','接收文件？':'Receive files?',
 '来自 MacBook Pro':'From MacBook Pro','及另外 2 个文件':'and 2 more files',
 '传输已暂停':'Transfer paused','连接已中断':'Connection lost',
 '继续后从断点发送。':'Resume from where you left off.',
 '文件已保留，可继续发送。':'Your files are kept. Try again.',
 '拒绝':'Decline','接受':'Accept','取消':'Cancel','继续发送':'Resume',
 '还没有发现设备':'No devices nearby','连接同一网络后重试。':'Connect to the same network.',
 '添加设备':'Add device','设计源文件.fig':'Design source.fig','现场照片':'Photos',
 '产品演示.mp4':'Product demo.mp4','字体包.zip':'Fonts.zip','项目资料':'Project files',
 '合同.pdf':'Contract.pdf','演示文稿.key':'Presentation.key','离线备份':'Offline backup',
 '迭代方案与会议纪要.pdf':'Iteration notes and decisions.pdf',
 '选中设备':'Selected device','传输中':'Transferring','接收确认':'Receive request',
 '暂停':'Paused','断线恢复':'Connection lost','空状态':'Empty state',
 '节点图 · 桌面端':'Node graph / Desktop','节点图 · 移动端':'Node graph / Mobile',
 'Primary #2563EB  /  8px 节奏 · 4px 微调':'Primary #2563EB / 8px rhythm / 4px detail',
 '节点图 · 排版与间距基础':'Node graph / Typography and spacing',
 '颜色':'Color','白色为主，蓝色聚焦动作与当前状态。':'White first. Blue for actions and active states.',
 '字体':'Typography','间距':'Spacing',
 'Segoe UI / Microsoft YaHei · 系统字体回退':'Segoe UI / System font fallback',
 '24 / 32 · 页面标题':'24 / 32 / Page title','20 / 28 · 分组与状态':'20 / 28 / Section and status',
 '14 / 20 · 正文与节点标签':'14 / 20 / Body and node labels','12 / 16 · 辅助信息':'12 / 16 / Supporting text',
 '图标与基线微调':'Icon and baseline details','图标与文字、同组操作':'Icon to label / Related actions',
 '节点与标签':'Node to label','小容器内边距':'Small container padding',
 '面板内边距、分组':'Panel padding / Groups','桌面页面边距':'Desktop page margin',
 '文件网格行间空隙':'Space between file rows','大区块留白':'Section whitespace',
 '布局与密度':'Layout and density',
 '桌面：32px 页面边距 / 图区域最大 800px':'Desktop: 32px margin / Graph max-width 800px',
 '记录浮层 528px，内边距 24px，三列 144px，列距 24px。':'History: 528px / 24px padding / 144px columns / 24px gaps',
 '移动端：20px 页面边距':'Mobile: 20px page margin',
 '两列设备组：160 + 16 + 160px，整体居中':'Device columns: 160 + 16 + 160px, centered',
 '操作触控区 48px，安全区独立预留':'48px touch targets / Respect safe areas',
 '记录使用全屏，三列 106px，列距 16px':'History: full screen / 106px columns / 16px gaps',
 '文件单元高 80px，行距 48px，总节奏 128px':'File cell: 80px / Row gap: 48px / Pitch: 128px',
 '节点、操作与状态':'Nodes, actions and states','节点组件':'Node components',
 '设备 64 / 本机与选中 96；移动端 56 / 80。':'Desktop nodes: 64 / 96px. Mobile: 56 / 80px.',
 '在线':'Online','等待':'Waiting','离线':'Offline','云端保管':'Cloud','选中':'Selected',
 '操作与焦点':'Actions and focus','默认':'Default','悬停':'Hover','主操作':'Primary','键盘焦点':'Keyboard focus',
 '图标 22px / 触控区 48px / 焦点环 2px，外间距 4px。':'22px icons / 48px targets / 2px focus ring / 4px offset',
 '图标操作提供 tooltip、可访问名称与键盘焦点。':'Icon actions: tooltip, accessible name and keyboard focus.',
 '历史入口':'History entry','设备详情':'Device details','选中 → 更多 → 传输记录':'Select a node → More → Transfer history',
 '活动与恢复':'Activity and recovery',
 '1.8s 匀速循环，仅表示传输活动。':'1.8s linear loop. Activity only.',
 '灰色轨道 3px，蓝色活动段 4px；单段占路径约 12%。':'3px gray track / 4px blue segment / About 12% of path',
 '减少动态效果：停止循环，保留静态活动段。':'Reduced motion: keep a static activity segment.',
 '暂停 / 断线：静止轨道，明确原因与继续操作。':'Paused or offline: static track, reason and recovery.',
 '静态设计稿中的三个时刻；动画不能表示真实完成百分比。':'Three design keyframes. This animation never implies a completion percentage.',
}

def tr(s):
    if not ENGLISH:return s
    if s in EN:return EN[s]
    if any('\u4e00'<=c<='\u9fff' for c in s):
        match=re.match(r'^(\d+\s+)(.*)$',s)
        if match and match[2] in EN:return match[1]+EN[match[2]]
        raise ValueError('Missing English copy: '+s)
    return s
C = dict(blue='#2563EB', hover='#1D4ED8', soft='#EFF6FF', ink='#172033',
         muted='#647084', faint='#94A0B2', line='#E4E8EE', border='#D8DEE8',
         surface='#F7F9FC', white='#FFFFFF', icon='#7A879B')

def group(name, content, extra=''):
    return f'<g id="{escape(name)}" {extra}>{content}</g>'

def rect(x,y,w,h,fill=C['white'],r=0,stroke=None,sw=1,extra=''):
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"'+(f' stroke="{stroke}" stroke-width="{sw}"' if stroke else '')+f' {extra}/>'

def circle(x,y,r,fill=C['white'],stroke=None,sw=1,extra=''):
    return f'<circle cx="{x}" cy="{y}" r="{r}" fill="{fill}"'+(f' stroke="{stroke}" stroke-width="{sw}"' if stroke else '')+f' {extra}/>'

def path(d,stroke=C['line'],sw=1.5,fill='none',extra=''):
    return f'<path d="{d}" fill="{fill}" stroke="{stroke}" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round" {extra}/>'

def text(s,x,y,size=14,color=C['ink'],weight=400,anchor='start',extra=''):
    s=tr(s)
    # y is the top of a fixed line box; the baseline uses the same 0.8em optical offset.
    lh={12:16,14:20,16:24,20:28,24:32,32:40}.get(size, size*1.4)
    by=y+(lh-size)/2+size*.81
    cjk=any('\u4e00' <= c <= '\u9fff' for c in s)
    family='Microsoft YaHei' if cjk else 'Segoe UI'
    if cjk and weight==600:weight=700
    return f'<text x="{x}" y="{by}" font-family="{family}" font-size="{size}" font-weight="{weight}" fill="{color}" text-anchor="{anchor}" {extra}>{escape(s)}</text>'

ICONS = {
    'laptop':'M4 4h16v12H4z M2 20h20l-2-4H4z',
    'phone':'M8 2h8a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H8a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z M11 19h2',
    'tablet':'M5 2h14a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z M11 19h2',
    'nas':'M5 3h14v18H5z M5 10h14 M8 16h.01 M12 16h4',
    'cloud':'M6 18h12a4 4 0 0 0 1-7.87A7 7 0 0 0 5.15 9 4.5 4.5 0 0 0 .85 9z',
    'send':'M12 20V4 M6 10l6-6 6 6 M5 20h14',
    'folder':'M3 6h6l2 3h10v11H3z',
    'history':'M3 5v5h5 M3.5 10a9 9 0 1 1 .5 7 M12 7v5l3 2',
    'more':'M5 12h.01 M12 12h.01 M19 12h.01',
    'close':'M6 6l12 12 M18 6L6 18',
    'back':'M19 12H5 M11 6l-6 6 6 6',
    'plus':'M12 5v14 M5 12h14',
    'add':'M3 4h12v12H3z M6 20h7 M9 16v4 M20 10v8 M16 14h8',
    'check':'M5 12l4 4L19 6',
    'chevron':'M9 5l7 7-7 7',
    'download':'M12 3v12 M6 9l6 6 6-6 M4 17v4h16v-4',
    'pause':'M8 5v14 M16 5v14',
    'play':'M8 4l12 8-12 8z',
    'alert':'M12 8v5 M12 17h.01 M12 3L2 21h20z',
    'wifi':'M2 8a16 16 0 0 1 20 0 M5 12a11 11 0 0 1 14 0 M8.5 16a5 5 0 0 1 7 0 M12 20h.01',
    'search':'M10 3a7 7 0 1 0 0 14 7 7 0 0 0 0-14z M15 15l6 6',
    'settings':'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z M10 2h4l1 4 3-1 3 3-2 4 2 4-3 3-3-1-1 4h-4l-1-4-3 1-3-3 2-4-2-4 3-3 3 1z',
}

def icon(kind,x,y,size=24,color=C['ink'],sw=1.7):
    return group(kind, path(ICONS[kind],color,sw),f'transform="translate({x} {y}) scale({size/24})"')

def logo(x,y,size=28):
    content=path('M5 15c0-5 3-9 7-9s7 3 7 7-3 7-7 7c-2 0-4-1-4-3s2-4 4-4h3',C['blue'],2.4)
    return group('Pinelink-mark',content,f'transform="translate({x} {y}) scale({size/24})"')

def iconbutton(kind,x,y,primary=False,soft=False,focus=False,label=None):
    bg=C['blue'] if primary else C['soft'] if soft else C['white']
    col=C['white'] if primary else C['blue'] if soft else C['muted']
    s=''
    if focus:
        s+=circle(x+24,y+24,28,'none',C['blue'],2)
    s+=circle(x+24,y+24,24,bg,None if primary or soft else C['line'])
    s+=icon(kind,x+13,y+13,22,col,2.4 if kind=='more' else 1.7)
    return group(label or f'button-{kind}-target48',s)

def button(label,x,y,primary=False,iconkind=None):
    w=32+len(label)*14+(28 if iconkind else 0)
    if ENGLISH:
        font=ImageFont.truetype('C:/Windows/Fonts/seguisb.ttf',14)
        w=math.ceil((32+font.getlength(tr(label))+(28 if iconkind else 0))/4)*4
    col=C['white'] if primary else C['ink']
    s=rect(x,y,w,48,C['blue'] if primary else C['white'],12,None if primary else C['border'])
    if iconkind:s+=icon(iconkind,x+16,y+14,20,col)
    s+=text(label,x+(44 if iconkind else 16),y+14,14,col,600)
    return group(f'button-{label}',s),w

def node(kind,label,x,y,size=64,selected=False,own=False,status='online',opacity=1):
    r=size/2
    s=circle(x,y,r,C['soft'] if selected else C['surface'] if own else C['white'],C['blue'] if selected else C['border'],2 if selected else 1.2)
    s+=icon(kind,x-size/4,y-size/4,size/2,C['blue'] if selected else C['ink'],1.6)
    if status:
        bx,by=x+r*.72,y+r*.72
        s+=circle(bx,by,7,C['white'])
        if status=='cloud':s+=circle(bx,by,6,C['white'])+icon('cloud',bx-7,by-7,14,C['muted'])
        elif status=='waiting':s+=circle(bx,by,4,'white',C['muted'],1.4)
        else:s+=circle(bx,by,4,C['faint'] if status=='offline' else C['ink'])
    fs=14 if size>=64 else 12
    ly=y+r+12
    s+=rect(x-80,ly-2,160,24,C['white'])
    s+=text(label,x,ly,fs,C['ink'] if selected or own else C['muted'],600 if own or selected else 400,'middle')
    return group(f'node-{label}-{size}',s,f'opacity="{opacity}"')

def systembars(home=True):
    s=text('9:41',24,12,14,C['ink'],600)
    s+=path('M313 25v-4 M317 25v-7 M321 25V15 M329 20q5-6 10 0 M332 23q2-3 4 0',C['ink'],2)
    s+=rect(346,16,22,10,'none',3,C['ink'],1.2)+rect(349,19,15,4,C['ink'],1)
    s+=rect(369,19,2,4,C['ink'],1)
    if home:s+=rect(128,824,134,4,C['ink'],2)
    return group('system-safe-areas',s)

def header(m=False,mode='normal'):
    w=390 if m else 1200
    s=''
    if m:
        s+=systembars()
    y=56 if m else 24
    if mode=='records':s+=iconbutton('back',8,y)
    else:s+=logo(20 if m else 32,y+10)+text('Pinelink',60 if m else 72,y+10,20,C['ink'],600)
    if mode in ('normal','records'):
        s+=iconbutton('more',322 if m else 1120,y)
        if not m and mode=='normal':s+=iconbutton('add',1064,y)
    if not m:s+=rect(32,95,1136,1,C['line'])
    return group('header',s)

def graph(m=False,opacity=1,omit=None):
    if m:
        peers=[('laptop','MacBook Pro',107,248,'online'),('phone','iPhone',283,248,'waiting'),('nas','小窝云仓',107,392,'cloud'),('tablet','iPad',283,392,'offline')]
        own=(195,680,80)
    else:
        peers=[('laptop','MacBook Pro',312,312,'online'),('phone','iPhone',496,248,'waiting'),('nas','小窝云仓',704,248,'cloud'),('tablet','iPad',888,312,'offline')]
        own=(600,648,96)
    ox,oy,os=own
    s=''
    for kind,label,x,y,status in peers:
        if label==omit:continue
        rr=28 if m else 32
        d=f'M{ox} {oy-os/2} C{ox} {oy-176} {x} {y+160} {x} {y+rr}'
        if m and y==248:
            side=-1 if x<ox else 1
            d=f'M{ox} {oy-os/2} C{ox+side*176} {oy-144} {x+side*88} {y+160} {x} {y+rr}'
        s+=path(d,C['line'],1.5)
    for kind,label,x,y,status in peers:
        if label==omit:continue
        s+=node(kind,label,x,y,56 if m else 64,status=status,opacity=.42 if status=='offline' else 1)
    s+=node('phone' if m else 'laptop','本机',ox,oy,os,own=True)
    return group('device-graph',s,f'opacity="{opacity}"')

def default(m=False):
    s=header(m)+text('设备',20 if m else 32,136 if m else 128,24,C['ink'],600)
    s+=text('4 台',370 if m else 1168,144 if m else 136,14,C['muted'],400,'end')
    s+=graph(m)
    return s

def selection(m=False):
    cx,cy=(195,352) if m else (600,328)
    s=header(m,'focused')+graph(m,.13,omit='MacBook Pro')
    s+=circle(cx,cy,112,'none',C['line'],1,extra='stroke-dasharray="2 6"')
    s+=node('laptop','MacBook Pro',cx,cy,80 if m else 96,selected=True)
    s+=iconbutton('send',cx-136,cy-24,primary=True,label='发送文件 / Enter')
    s+=iconbutton('folder',cx-24,cy-136,label='打开文件夹')
    s+=iconbutton('more',cx+88,cy-24,label='更多 / 包含传输记录')
    if not m:
        s+=rect(cx-156,cy+40,88,32,C['ink'],8)+text('发送文件',cx-112,cy+46,12,C['white'],400,'middle')
    return group('selection',s)

def transfer(m=False,static=False):
    ox,oy=(195,680) if m else (504,648)
    tx,ty=(195,264) if m else (696,248)
    os=80 if m else 96
    d=f'M{ox} {oy-os/2} C{ox-104} {oy-184} {tx+104} {ty+176} {tx} {ty+(28 if m else 32)}'
    s=header(m,'focused')
    s+=path(d,C['line'],3)
    if not static:
        pts=[(ox,oy-os/2),(ox-104,oy-184),(tx+104,ty+176),(tx,ty+(28 if m else 32))]
        def pos(t):
            return tuple((1-t)**3*pts[0][i]+3*(1-t)**2*t*pts[1][i]+3*(1-t)*t*t*pts[2][i]+t**3*pts[3][i] for i in [0,1])
        def derivative(t):
            return tuple(3*(1-t)**2*(pts[1][i]-pts[0][i])+6*(1-t)*t*(pts[2][i]-pts[1][i])+3*t*t*(pts[3][i]-pts[2][i]) for i in [0,1])
        samples=[pos(i/1000) for i in range(1001)]
        distances=[0]
        for i in range(1,1001):distances.append(distances[-1]+sum((samples[i][j]-samples[i-1][j])**2 for j in [0,1])**.5)
        ts=[next(i/1000 for i,v in enumerate(distances) if v>=distances[-1]*f) for f in [.44,.56]]
        a,b=ts
        q0,q3=pos(a),pos(b)
        q1=tuple(q0[i]+derivative(a)[i]*(b-a)/3 for i in [0,1])
        q2=tuple(q3[i]-derivative(b)[i]*(b-a)/3 for i in [0,1])
        seg=f'M{q0[0]:.3f} {q0[1]:.3f} C{q1[0]:.3f} {q1[1]:.3f} {q2[0]:.3f} {q2[1]:.3f} {q3[0]:.3f} {q3[1]:.3f}'
        s+=group('activity-segment-12percent-single',path(seg,C['blue'],4))
    s+=node('laptop','MacBook Pro',tx,ty,56 if m else 64,status=None)
    s+=node('phone' if m else 'laptop','本机',ox,oy,os,own=True,status=None)
    return group('transfer-static-preview' if not static else 'transfer-interrupted',s)

def fileicon(kind,x,y,size=48,cloud=False):
    s=''
    if kind=='folder':
        s+=path('M3 7h7l3 4h32v29H3z',C['icon'],1.5,C['surface'])
        s+=path('M3 15h42v25H3z',C['icon'],1.5,C['surface'])
    else:
        s+=path('M9 3h19l11 11v31H9z',C['icon'],1.5,C['white'])
        s+=path('M28 3v11h11',C['icon'],1.5,C['surface'])
        if kind=='video':s+=path('M20 23l9 6-9 6z',C['muted'],1.5,C['surface'])
        elif kind=='archive':s+=path('M23 6v17 M21 25h4v7h-4z',C['muted'],1.5)
        else:s+=path('M17 24h14 M17 30h14 M17 36h9',C['muted'],1.5)
    if cloud:s+=circle(39,39,9,C['white'])+icon('cloud',31,31,16,C['muted'])
    return group('file-icon-'+kind,s,f'transform="translate({x} {y}) scale({size/48})"')

FILES=[('doc','设计源文件.fig'),('folder','现场照片'),('video','产品演示.mp4'),('archive','字体包.zip'),('folder','项目资料'),('doc','合同.pdf'),('doc','演示文稿.key'),('folder','离线备份'),('doc','迭代方案与会议纪要.pdf')]

def filegrid(x,y,w,m=False):
    gap=16 if m else 24
    cw=(w-gap*2)/3
    s=''
    for i,(kind,name) in enumerate(FILES[:9 if m else 6]):
        col,row=i%3,i//3
        cx=x+col*(cw+gap)+cw/2
        top=y+row*128
        one=fileicon(kind,cx-24,top,48,cloud=i==4)
        font=ImageFont.truetype('C:/Windows/Fonts/segoeui.ttf' if ENGLISH else 'C:/Windows/Fonts/msyh.ttc',12 if m else 14)
        display=tr(name)
        if font.getlength(display)>cw-8:
            while font.getlength(display+'…')>cw-8:display=display[:-1]
            display+='…'
        one+=text(display,cx,top+60,12 if m else 14,C['ink'],400,'middle')
        s+=group('file-'+name,one)
    return group('file-grid-3col-row128',s)

def history(m=False):
    if m:
        s=header(True,'records')+text('传输记录',20,136,24,C['ink'],600)+text('MacBook Pro',20,176,14,C['muted'])
        s+=rect(20,208,350,1,C['line'])+filegrid(20,244,350,True)
    else:
        s=header(False,'focused')+graph(False,.13,omit='MacBook Pro')
        s+=node('laptop','MacBook Pro',312,312,96,selected=True)
        s+=iconbutton('history',416,288,soft=True)
        # Low opacity outline shadows retain a flat surface in editable SVG import.
        s+=rect(508,196,536,452,'#172033',20,extra='opacity=".025"')
        s+=rect(512,192,528,448,C['white'],16,C['line'])
        s+=text('传输记录',536,216,24,C['ink'],600)+text('MacBook Pro',536,256,14,C['muted'])
        s+=iconbutton('close',968,208)
        s+=rect(536,296,480,1,C['line'])+filegrid(536,320,480)
    return group('history',s)

def statepanel(kind,m=False):
    receive=kind=='receive'
    if m:
        x,y,w,h=(0,464,390,380) if receive else (0,536,390,308)
        px=20
    else:
        x,y,w,h=(400,224,400,360) if receive else (416,256,368,288)
        px=x+24
    content=''
    if m:content+=rect(179,y+12,32,4,C['border'],2)
    top=y+(40 if m else 24)
    if receive:
        content+=icon('download',px,top,28,C['blue'])
        content+=text('接收文件？',px,top+44,24,C['ink'],600)
        content+=text('来自 MacBook Pro',px,top+84,14,C['muted'])
        content+=rect(px,top+120,w-(40 if m else 48),72,C['surface'],12)
        content+=fileicon('doc',px+12,top+132,40)
        content+=text('设计源文件.fig',px+64,top+130,14,C['ink'],600)
        content+=text('及另外 2 个文件',px+64,top+154,12,C['muted'])
        by=top+224
        b1,bw=button('拒绝',0,0)
        b2,b2w=button('接受',0,0,True)
    else:
        paused=kind=='paused'
        content+=icon('pause' if paused else 'alert',px,top,28,C['muted'])
        content+=text('传输已暂停' if paused else '连接已中断',px,top+44,20,C['ink'],600)
        content+=text('继续后从断点发送。' if paused else '文件已保留，可继续发送。',px,top+84,14,C['muted'])
        by=top+144
        b1,bw=button('取消',0,0)
        b2,b2w=button('继续发送',0,0,True)
    right=x+w-(20 if m else 24)
    b1,_=button('拒绝' if receive else '取消',right-b2w-12-bw,by)
    b2,_=button('接受' if receive else '继续发送',right-b2w,by,True)
    content+=b1+b2
    shadow=rect(x-4,y+4,w+8,h+4,C['ink'],24 if m else 20,extra='opacity=".025"') if not m else ''
    surface=rect(x,y,w,h,C['white'],24 if m else 16,None if m else C['line'])
    return group(kind+'-panel',shadow+surface+content)

def state(kind,m=False):
    s=group('dimmed-context',graph(m), 'opacity=".13"') if kind=='receive' else group('dimmed-context',transfer(m,True),'opacity=".18"')
    if kind=='receive':s=header(m,'focused')+s
    if m:s+=rect(0,48,390,796,C['ink'],0,extra='opacity=".06"')
    s+=statepanel(kind,m)
    if m:s+=rect(0,0,390,48,C['white'])+systembars()
    return s

def empty(m=False):
    s=header(m)+text('设备',20 if m else 32,136 if m else 128,24,C['ink'],600)
    cx,cy=(195,304) if m else (600,320)
    s+=icon('wifi',cx-20,cy-20,40,C['faint'])
    s+=text('还没有发现设备',cx,cy+44,20,C['ink'],600,'middle')
    s+=text('连接同一网络后重试。',cx,cy+80,14,C['muted'],400,'middle')
    b,bw=button('添加设备',0,0,True,'plus')
    s+=group('add-device',b,f'transform="translate({cx-bw/2} {cy+120})"')
    s+=node('phone' if m else 'laptop','本机',195 if m else 600,680 if m else 648,80 if m else 96,own=True)
    return s

def svg(content,w,h,id='Pinelink'):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" id="{id}">{rect(0,0,w,h)}{content}</svg>'

SCREENS=[('01-discovery','设备',default),('02-selected','选中设备',selection),('03-transfer','传输中',transfer),('04-history','传输记录',history),('05-receive','接收确认',lambda m:state('receive',m)),('06-paused','暂停',lambda m:state('paused',m)),('07-disconnected','断线恢复',lambda m:state('error',m)),('08-empty','空状态',empty)]

manifest=[]
for device,m,w,h in [('desktop',False,1200,800),('mobile',True,390,844)]:
    for slug,name,fn in SCREENS:
        name=f'{device}-{slug}'
        body=fn(m)
        (OUT/(name+'.svg')).write_text(svg(body,w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',width=w,height=h,label=slug))

def board(device,cols):
    items=[v for v in manifest if v['name'].startswith(device)]
    iw,ih=items[0]['width'],items[0]['height']
    gap,pad,top=48,48,148
    w=pad*2+cols*iw+(cols-1)*gap
    rows=(len(items)+cols-1)//cols
    h=top+rows*(ih+88)+pad
    s=rect(0,0,w,h,C['surface'])
    s+=logo(48,40,32)+text('Pinelink',92,40,24,C['ink'],600)
    s+=text('节点图 · '+('桌面端' if device=='desktop' else '移动端'),48,88,16,C['muted'])
    s+=text('Primary #2563EB  /  8px 节奏 · 4px 微调',w-48,52,14,C['muted'],400,'end')
    for i,it in enumerate(items):
        col,row=i%cols,i//cols
        x,y=pad+col*(iw+gap),top+row*(ih+88)
        s+=text(SCREENS[i][0][:2]+'  '+SCREENS[i][1],x,y-36,16,C['ink'],600)
        content=(OUT/it['file']).read_text(encoding='utf-8')
        inner=content[content.index('>')+1:content.rindex('</svg>')]
        s+=group(it['name'],inner,f'transform="translate({x} {y})"')
        s+=rect(x,y,iw,ih,'none',0,C['line'])
    return svg(s,w,h,f'{device}-board')

(OUT/'desktop-board.svg').write_text(board('desktop',2),encoding='utf-8')
(OUT/'mobile-board.svg').write_text(board('mobile',4),encoding='utf-8')
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
print(f'Generated {len(manifest)} editable screens in {OUT}')
