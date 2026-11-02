from pathlib import Path
import json, runpy

ns=runpy.run_path(str(Path(__file__).with_name('build-13-blue-graph.py')))
globals().update({k:ns[k] for k in ['OUT','C','rect','text','icon','circle','path','group','logo','svg','node','iconbutton','button']})

def rule(y):return rect(48,y,1344,1,C['line'])
def title(num,label,y,x=48):return text(num+'  '+label,x,y,20,C['ink'],600)
def note(s,x,y):return text(s,x,y,14,C['muted'])

s=logo(48,40,36)+text('Pinelink',100,40,32,C['ink'],600)
s+=text('节点图 · 排版与间距基础',48,104,20,C['ink'],600)
s+=text('13 / 2026.11.01',1392,52,14,C['muted'],400,'end')+rule(160)
s+=title('01','颜色',192)+text('白色为主，蓝色聚焦动作与当前状态。',1392,196,14,C['muted'],400,'end')
swatches=[('Primary','blue'),('Primary soft','soft'),('Text','ink'),('Secondary','muted'),('Surface','surface'),('Border','border'),('File icon','icon'),('Canvas','white')]
for i,(label,key) in enumerate(swatches):
    x=48+i*168
    s+=rect(x,240,152,64,C[key],8,C['line'] if key in ['white','surface','soft'] else None)
    if key=='blue':s+=text('Aa',x+16,254,24,C['white'],600)
    s+=text(label,x,316,14,C['ink'],600)+text(C[key],x,344,12,C['muted'])
s+=rule(392)+title('02','字体',424)+title('03','间距',424,768)
s+=note('Segoe UI / Microsoft YaHei · 系统字体回退',48,464)
types=[('传输记录',24,600,'24 / 32 · 页面标题',512),('接收文件？',20,600,'20 / 28 · 分组与状态',568),('设计源文件.fig',14,400,'14 / 20 · 正文与节点标签',616),('MacBook Pro',12,400,'12 / 16 · 辅助信息',656)]
for label,size,weight,detail,y in types:
    s+=text(label,48,y,size,C['ink'],weight)+text(detail,400,y+4,12,C['muted'])
spacing=[(4,'图标与基线微调'),(8,'图标与文字、同组操作'),(12,'节点与标签'),(16,'小容器内边距'),(24,'面板内边距、分组'),(32,'桌面页面边距'),(48,'文件网格行间空隙'),(64,'大区块留白')]
for i,(v,label) in enumerate(spacing):
    y=472+i*28
    s+=text(str(v),768,y,12,C['muted'])+rect(812,y+4,v*3,8,C['blue'] if v<=16 else C['soft'],2)+text(label,1040,y,12,C['muted'])
s+=rule(728)+title('04','布局与密度',760)
# Desktop keyline schematic.
s+=rect(48,816,640,248,C['surface'],12,C['line'])
s+=rect(64,832,608,24,C['white'],4)
s+=rect(64,872,608,176,C['white'],4)
s+=path('M368 1016C368 956 240 956 240 904 M368 1016C368 952 320 948 320 888 M368 1016C368 952 416 948 416 888 M368 1016C368 956 496 956 496 904',C['line'])
for x,y in [(240,904),(320,888),(416,888),(496,904)]:s+=circle(x,y,12,C['white'],C['border'])
s+=circle(368,1016,18,C['soft'],C['blue'])
s+=note('桌面：32px 页面边距 / 图区域最大 800px',48,1084)
s+=note('记录浮层 528px，内边距 24px，三列 144px，列距 24px。',48,1116)
# Mobile schematic, dimensioned independently of the desktop.
s+=rect(768,816,144,280,C['surface'],16,C['line'])
s+=rect(780,828,120,24,C['white'],4)
s+=path('M840 1056C840 1008 808 952 808 904 M840 1056C840 1008 872 952 872 904 M840 1056C840 1016 808 1008 808 968 M840 1056C840 1016 872 1008 872 968',C['line'])
for x,y in [(808,904),(872,904),(808,968),(872,968)]:s+=circle(x,y,10,C['white'],C['border'])
s+=circle(840,1056,15,C['soft'],C['blue'])
for i,label in enumerate(['移动端：20px 页面边距','两列设备组：160 + 16 + 160px，整体居中','操作触控区 48px，安全区独立预留','记录使用全屏，三列 106px，列距 16px','文件单元高 80px，行距 48px，总节奏 128px']):s+=note(label,960,832+i*48)
(OUT/'00-foundations.svg').write_text(svg(s,1440,1184,'Foundations'),encoding='utf-8')

s=logo(48,40,36)+text('Pinelink',100,40,32,C['ink'],600)
s+=text('节点、操作与状态',48,104,20,C['ink'],600)+rule(160)
s+=title('01','节点组件',192)+note('设备 64 / 本机与选中 96；移动端 56 / 80。',768,196)
specs=[('在线','laptop','online',False,False),('等待','phone','waiting',False,False),('离线','tablet','offline',False,False),('云端保管','nas','cloud',False,False),('本机','laptop','online',False,True),('选中','laptop','online',True,False)]
for i,(label,kind,status,selected,own) in enumerate(specs):
    x=152+i*224
    content=node(kind,'本机' if own else 'MacBook Pro' if selected else label,x,304,96 if own or selected else 64,selected,own,status,.42 if status=='offline' else 1)
    s+=group('Component-Node-'+label,content)+text(label,x,404,12,C['muted'],400,'middle')
s+=rule(456)+title('02','操作与焦点',488)
controls=[('默认',False,False,False),('悬停',False,True,False),('主操作',True,False,False),('键盘焦点',True,False,True)]
for i,(label,primary,soft,focus) in enumerate(controls):
    x=48+i*144
    s+=group('Component-Action-'+label,iconbutton('send',x,552,primary,soft,focus))+text(label,x+24,620,12,C['muted'],400,'middle')
s+=note('图标 22px / 触控区 48px / 焦点环 2px，外间距 4px。',48,676)
s+=note('图标操作提供 tooltip、可访问名称与键盘焦点。',48,708)
s+=title('03','历史入口',488,768)
s+=iconbutton('more',768,552,soft=True)
s+=rect(848,536,256,152,C['white'],12,C['line'])
s+=icon('history',868,554,20,C['muted'])+text('传输记录',904,554,14,C['ink'])+icon('chevron',1060,554,20,C['muted'])
s+=rect(864,592,224,1,C['line'])
s+=icon('laptop',868,618,20,C['muted'])+text('设备详情',904,618,14,C['ink'])
s+=note('选中 → 更多 → 传输记录',768,708)
s+=rule(760)+title('04','活动与恢复',792)
# These are design keyframes, not live percentage progress.
for i,label in enumerate(['0.0s','0.6s','1.2s']):
    x=48+i*240
    s+=path(f'M{x} 900 C{x+48} 852 {x+144} 948 {x+192} 900',C['line'],3)
    pts=[(x,900),(x+48,852),(x+144,948),(x+192,900)]
    def at(t):return tuple((1-t)**3*pts[0][j]+3*(1-t)**2*t*pts[1][j]+3*(1-t)*t*t*pts[2][j]+t**3*pts[3][j] for j in [0,1])
    def dv(t):return tuple(3*(1-t)**2*(pts[1][j]-pts[0][j])+6*(1-t)*t*(pts[2][j]-pts[1][j])+3*t*t*(pts[3][j]-pts[2][j]) for j in [0,1])
    a,b=[(.04,.16),(.44,.56),(.84,.96)][i]
    q0,q3=at(a),at(b)
    q1=tuple(q0[j]+dv(a)[j]*(b-a)/3 for j in [0,1])
    q2=tuple(q3[j]-dv(b)[j]*(b-a)/3 for j in [0,1])
    s+=path(f'M{q0[0]} {q0[1]} C{q1[0]} {q1[1]} {q2[0]} {q2[1]} {q3[0]} {q3[1]}',C['blue'],4)
    s+=text(label,x+96,948,12,C['muted'],400,'middle')
for i,label in enumerate(['1.8s 匀速循环，仅表示传输活动。','灰色轨道 3px，蓝色活动段 4px；单段占路径约 12%。','减少动态效果：停止循环，保留静态活动段。','暂停 / 断线：静止轨道，明确原因与继续操作。']):s+=note(label,816,832+i*44)
s+=note('静态设计稿中的三个时刻；动画不能表示真实完成百分比。',48,1020)
(OUT/'03-interaction.svg').write_text(svg(s,1440,1104,'Interaction'),encoding='utf-8')

colors={k:{'$type':'color','$value':v} for k,v in C.items()}
space={str(v):{'$type':'number','$value':v} for v in [4,8,12,16,20,24,32,48,64,80,96]}
tokens={'color':colors,'space':space,'radius':{str(v):{'$type':'number','$value':v} for v in [8,12,16,24]}}
(OUT/'tokens.json').write_text(json.dumps(tokens,ensure_ascii=False,indent=2),encoding='utf-8')
print('Generated foundations, interaction board, and tokens.')
