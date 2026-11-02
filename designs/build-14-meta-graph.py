from pathlib import Path
from PIL import ImageFont
from html import escape
import sys, runpy, json, math

if '--english' not in sys.argv:sys.argv.append('--english')
ns=runpy.run_path(str(Path(__file__).with_name('build-13-blue-graph.py')))
p=ns['default'].__globals__
globals().update({k:p[k] for k in ['C','rect','circle','path','group','icon','tr','svg','fileicon','systembars']})
OUT=Path(__file__).parent/'14-meta-graph'
OUT.mkdir(exist_ok=True)
C.update(blue='#0866FF',hover='#005CE6',soft='#EEF5FF',ink='#101010',muted='#6E6E6E',faint='#A8A8A8',line='#EBEBEB',border='#DBDBDB',surface='#F5F5F5',icon='#808080')

def text(s,x,y,size=14,color=C['ink'],weight=400,anchor='start',extra=''):
    s=tr(s)
    size={12:13,14:15}.get(size,size)
    lh={13:18,15:20,16:24,20:28,24:32,32:40}.get(size,size*1.4)
    by=y+(lh-size)/2+size*.81
    return f'<text x="{x}" y="{by}" font-family="Segoe UI" font-size="{size}" font-weight="{weight}" fill="{color}" text-anchor="{anchor}" {extra}>{escape(s)}</text>'

def logo(x,y,size=28):
    return group('Pinelink mark',path('M5 15c0-5 3-9 7-9s7 3 7 7-3 7-7 7c-2 0-4-1-4-3s2-4 4-4h3',C['ink'],2.4),f'transform="translate({x} {y}) scale({size/24})"')

def iconbutton(kind,x,y,primary=False,soft=False,focus=False,label=None):
    s=rect(x,y,48,48,C['white'],0,extra='opacity="0"')
    if focus:s+=circle(x+24,y+24,28,'none',C['blue'],2)
    if primary or soft:s+=circle(x+24,y+24,24,C['blue'] if primary else C['surface'])
    s+=icon(kind,x+12,y+12,24,C['white'] if primary else C['ink'],2.8 if kind=='more' else 1.8)
    return group(label or 'Action-'+kind,s)

def button(label,x,y,primary=False,iconkind=None,width=None):
    display=tr(label)
    font=ImageFont.truetype('C:/Windows/Fonts/seguisb.ttf',15)
    w=width or math.ceil((36+font.getlength(display)+(28 if iconkind else 0))/4)*4
    s=rect(x,y,w,48,C['blue'] if primary else C['surface'],24)
    if iconkind:
        s+=icon(iconkind,x+18,y+14,20,C['white'] if primary else C['ink'])
        s+=text(display,x+46,y+14,14,C['white'] if primary else C['ink'],600)
    else:s+=text(display,x+w/2,y+14,14,C['white'] if primary else C['ink'],600,'middle')
    return group('Button-'+display,s),w

def node(kind,label,x,y,size=64,selected=False,own=False,status='online',opacity=1):
    r=size/2
    s=circle(x,y,r,C['white'] if selected else C['surface'],C['blue'] if selected else None,2)
    s+=icon(kind,x-size/4,y-size/4,size/2,C['ink'],1.7)
    if status:
        bx,by=x+r*.72,y+r*.72
        s+=circle(bx,by,7,C['white'])
        if status=='cloud':s+=icon('cloud',bx-7,by-7,14,C['muted'])
        elif status=='waiting':s+=circle(bx,by,4,C['white'],C['muted'],1.5)
        else:s+=circle(bx,by,4,C['faint'] if status=='offline' else C['ink'])
    ly=y+r+12
    font=ImageFont.truetype('C:/Windows/Fonts/seguisb.ttf' if selected or own else 'C:/Windows/Fonts/segoeui.ttf',15)
    labelw=math.ceil((font.getlength(tr(label))+16)/4)*4
    s+=rect(x-labelw/2,ly-2,labelw,24,C['white'])
    s+=text(label,x,ly,14,C['ink'] if selected or own else C['muted'],600 if selected or own else 400,'middle')
    return group('Node-'+tr(label),s,f'opacity="{opacity}"')

def header(m=False,mode='normal'):
    y=56 if m else 24
    s=systembars() if m else ''
    if mode=='records':
        s+=iconbutton('back',8 if m else 24,y)
        if m:s+=text('Transfer history',195,y+14,16,C['ink'],600,'middle')
    elif m:
        s+=text('pinelink',195,y+6,24,C['ink'],600,'middle')
    else:
        s+=logo(32,y+8,32)+text('pinelink',76,y+8,24,C['ink'],600)
    if mode in ('normal','empty'):
        if mode=='normal':s+=iconbutton('plus',12 if m else 1064,y)
        s+=iconbutton('more',322 if m else 1120,y)
        if not m:s+=text('Devices',600,y+12,16,C['ink'],600,'middle')
    elif mode=='records':s+=iconbutton('more',322 if m else 1120,y)
    return group('App header',s)

def default(m=False):
    s=header(m)
    if m:s+=text('Devices',20,136,20,C['ink'],600)+text('4 devices',370,140,14,C['muted'],400,'end')
    else:s+=text('4 devices',600,80,12,C['muted'],400,'middle')
    return s+p['graph'](m)

def selection(m=False):
    cx,cy=(195,352) if m else (600,328)
    s=header(m,'focused')+p['graph'](m,.13,omit='MacBook Pro')
    s+=node('laptop','MacBook Pro',cx,cy,80 if m else 96,selected=True)
    s+=iconbutton('send',cx-136,cy-24,primary=True,label='Send files / Enter')
    s+=iconbutton('folder',cx-24,cy-136,soft=True,label='Open folder')
    s+=iconbutton('more',cx+88,cy-24,soft=True,label='More / Transfer history')
    return group('Selected node actions',s)

FILES=[('doc','Design source.fig'),('folder','Photos'),('video','Product demo.mp4'),('archive','Fonts.zip'),('folder','Project files'),('doc','Contract.pdf'),('doc','Presentation.key'),('folder','Offline backup'),('doc','Iteration notes and decisions.pdf')]

def filegrid(x,y,w,m=False):
    gap=16 if m else 24
    cw=(w-gap*2)/3
    s=''
    for i,(kind,name) in enumerate(FILES[:9 if m else 6]):
        col,row=i%3,i//3
        cx=x+col*(cw+gap)+cw/2
        top=y+row*128
        one=rect(cx-cw/2,top,cw,80,C['white'],0,extra='opacity="0"')
        one+=fileicon(kind,cx-24,top,48,cloud=i==4)
        font=ImageFont.truetype('C:/Windows/Fonts/segoeui.ttf',13 if m else 15)
        display=name
        if font.getlength(display)>cw-8:
            while font.getlength(display+'…')>cw-8:display=display[:-1]
            display=display.rstrip(' ._-')+'…'
        one+=text(display,cx,top+60,12 if m else 14,C['ink'],400,'middle')
        s+=group('File-'+name,one)
    return group('Files / 3 columns / 128px row pitch',s)

def history(m=False):
    if m:
        s=header(True,'records')+text('MacBook Pro',20,136,20,C['ink'],600)
        s+=filegrid(20,196,350,True)
    else:
        s=header(False,'focused')+p['graph'](False,.13,omit='MacBook Pro')
        s+=node('laptop','MacBook Pro',312,312,96,selected=True)
        s+=iconbutton('history',416,288,soft=True)
        s+=rect(508,196,536,356,C['ink'],28,extra='opacity=".025"')
        s+=rect(512,192,528,352,C['white'],24,C['line'])
        s+=text('Transfer history',536,216,20,C['ink'],600)+text('MacBook Pro',536,248,14,C['muted'])
        s+=iconbutton('close',968,208)
        s+=rect(536,288,480,1,C['line'])+filegrid(536,312,480)
    return group('Transfer history',s)

def sheet_surface(x,y,w,h,m=False):
    if not m:return rect(x-4,y+4,w+8,h+4,C['ink'],28,extra='opacity=".025"')+rect(x,y,w,h,C['white'],24)
    return path(f'M0 {y+28} Q0 {y} 28 {y} H{w-28} Q{w} {y} {w} {y+28} V844 H0z',C['white'],0,C['white'])

def statepanel(kind,m=False):
    receive=kind=='receive'
    if m:x,y,w,h=(0,464,390,380) if receive else (0,536,390,308)
    else:x,y,w,h=(400,224,400,352) if receive else (400,256,400,280)
    px=x+(20 if m else 24)
    top=y+(40 if m else 24)
    s=sheet_surface(x,y,w,h,m)
    if m:s+=rect(179,y+12,32,4,C['border'],2)
    if receive:
        s+=icon('download',px,top+2,24,C['ink'])+text('Receive files?',px+40,top,20,C['ink'],600)
        s+=text('From MacBook Pro',px+40,top+36,14,C['muted'])
        s+=rect(px,top+92,w-(40 if m else 48),80,C['surface'],20)
        s+=fileicon('doc',px+12,top+108,48)
        s+=text('Design source.fig',px+72,top+108,14,C['ink'],600)+text('and 2 more files',px+72,top+136,12,C['muted'])
        by=top+208
        a,b='Decline','Accept'
    else:
        s+=icon('pause' if kind=='paused' else 'alert',px,top,24,C['ink'])
        s+=text('Transfer paused' if kind=='paused' else 'Connection lost',px,top+44,20,C['ink'],600)
        s+=text('Resume from where you left off.' if kind=='paused' else 'Your files are kept. Try again.',px,top+84,14,C['muted'])
        by=top+144
        a,b='Cancel','Resume'
    inner=w-(40 if m else 48)
    bw=(inner-12)/2
    s+=button(a,px,by,width=bw)[0]+button(b,px+bw+12,by,True,width=bw)[0]
    return group(kind+' / Approval sheet' if receive else kind+' / Recovery sheet',s)

def empty(m=False):
    s=header(m,'empty')
    if m:s+=text('Devices',20,136,20,C['ink'],600)
    cx,cy=(195,304) if m else (600,320)
    s+=icon('wifi',cx-20,cy-20,40,C['faint'])
    s+=text('No devices nearby',cx,cy+44,20,C['ink'],600,'middle')
    s+=text('Connect to the same network.',cx,cy+80,14,C['muted'],400,'middle')
    b,bw=button('Add device',0,0,True)
    s+=group('Add device',b,f'transform="translate({cx-bw/2} {cy+120})"')
    s+=node('phone' if m else 'laptop','This device',195 if m else 600,680 if m else 648,80 if m else 96,own=True)
    return s

p.update(C=C,text=text,logo=logo,iconbutton=iconbutton,button=button,node=node,header=header,default=default,selection=selection,filegrid=filegrid,history=history,statepanel=statepanel,empty=empty,OUT=OUT)
SCREENS=[('01-discovery','Devices',default),('02-selected','Selected device',selection),('03-transfer','Transferring',p['transfer']),('04-history','Transfer history',history),('05-receive','Receive request',lambda m:p['state']('receive',m)),('06-paused','Paused',lambda m:p['state']('paused',m)),('07-disconnected','Connection lost',lambda m:p['state']('error',m)),('08-empty','Empty state',empty)]
p['SCREENS']=SCREENS
manifest=[]
for device,m,w,h in [('desktop',False,1200,800),('mobile',True,390,844)]:
    for slug,label,fn in SCREENS:
        name=f'{device}-{slug}'
        (OUT/(name+'.svg')).write_text(svg(fn(m),w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',width=w,height=h,label=label))
p['manifest']=manifest
p['EN']['Primary #2563EB  /  8px 节奏 · 4px 微调']='Primary #0866FF / 8px rhythm / 4px detail'
(OUT/'desktop-board.svg').write_text(p['board']('desktop',2),encoding='utf-8')
(OUT/'mobile-board.svg').write_text(p['board']('mobile',4),encoding='utf-8')
(OUT/'manifest.json').write_text(json.dumps(manifest,indent=2),encoding='utf-8')
(OUT/'tokens.json').write_text(json.dumps({'color':{k:{'$type':'color','$value':v} for k,v in C.items()},'space':{str(v):{'$type':'number','$value':v} for v in [4,8,12,16,20,24,32,48,64]},'radius':{str(v):{'$type':'number','$value':v} for v in [20,24,28]}},indent=2),encoding='utf-8')
print(f'Generated {len(manifest)} Meta-inspired design screens: {OUT}')

def rule(y):return rect(48,y,1344,1,C['line'])
def section(label,x,y):return text(label,x,y,20,C['ink'],600)

s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)
s+=text('Product foundations',48,100,20,C['ink'],600)+text('14 / Meta-inspired / 2026.11.01',1392,52,14,C['muted'],400,'end')+rule(152)
s+=section('Color',48,184)+text('Neutral surfaces. Blue for actions and selection.',1392,188,14,C['muted'],400,'end')
for i,(label,key) in enumerate([('Primary','blue'),('Primary soft','soft'),('Text','ink'),('Secondary','muted'),('Surface','surface'),('Border','border'),('File icon','icon'),('Canvas','white')]):
    x=48+i*168
    s+=rect(x,232,152,64,C[key],16,C['line'] if key in ['white','surface','soft'] else None)
    if key=='blue':s+=text('Aa',x+16,246,24,C['white'],600)
    s+=text(label,x,308,14,C['ink'],600)+text(C[key],x,336,12,C['muted'])
s+=rule(384)+section('Type',48,416)+section('Spacing',768,416)
s+=text('Segoe UI / System font fallback',48,460,14,C['muted'])
for label,size,detail,y in [('pinelink',24,'24 / 32   Brand',504),('Transfer history',20,'20 / 28   Titles',556),('MacBook Pro',14,'15 / 20   Body and node labels',604),('Design source.fig',12,'13 / 18   Metadata and mobile filenames',648)]:
    s+=text(label,48,y,size,C['ink'],600 if size>=20 else 400)+text(detail,384,y+4,12,C['muted'])
for i,(v,usage) in enumerate([(4,'Fine alignment'),(8,'Related actions'),(12,'Node to label'),(16,'Compact groups'),(20,'Mobile margins'),(24,'Panel padding'),(32,'Desktop margins'),(48,'File row gaps'),(64,'Section whitespace')]):
    y=460+i*28
    s+=text(str(v),768,y,12,C['muted'])+rect(812,y+5,v*3,8,C['ink'] if v<=16 else C['line'],4)+text(usage,1040,y,12,C['muted'])
s+=rule(744)+section('Layout',48,776)
for x,title_,lines in [(48,'Desktop',['1200 × 800 / 32px page margin','Graph max-width: 800px','History: 528 × 352px / 24px padding','File columns: 144px / 24px gaps']), (768,'Mobile',['390 × 844 / 20px page margin','Device groups: 160 + 16 + 160px, centered','History: full screen / 3 × 106px / 16px gaps','File cell: 80px / Row pitch: 128px'])]:
    s+=text(title_,x,824,16,C['ink'],600)
    for i,line in enumerate(lines):s+=text(line,x,868+i*32,14,C['muted'])
(OUT/'00-foundations.svg').write_text(svg(s,1440,1040,'Product foundations'),encoding='utf-8')

s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Components and interaction',48,100,20,C['ink'],600)+rule(152)
s+=section('Node states',48,184)+text('Desktop 64 / 96px. Mobile 56 / 80px.',1392,188,14,C['muted'],400,'end')
for i,(label,kind,status,selected,own) in enumerate([('Online','laptop','online',False,False),('Waiting','phone','waiting',False,False),('Offline','tablet','offline',False,False),('Cloud','nas','cloud',False,False),('This device','laptop','online',False,True),('Selected','laptop','online',True,False)]):
    x=152+i*224
    content=node(kind,'This device' if own else 'MacBook Pro',x,288,96 if own or selected else 64,selected,own,status,.42 if status=='offline' else 1)
    s+=group('Component-Node-'+label,content)+text(label,x,392,12,C['muted'],400,'middle')
s+=rule(440)+section('Actions',48,472)+section('History entry',768,472)
for i,(label,primary,soft,focus) in enumerate([('Default',False,False,False),('Hover',False,True,False),('Primary',True,False,False),('Focus',True,False,True)]):
    x=48+i*144
    s+=group('Component-Action-'+label,iconbutton('send',x,528,primary,soft,focus))+text(label,x+24,596,12,C['muted'],400,'middle')
s+=group('Component-Button-Secondary',button('Decline',48,652)[0])+group('Component-Button-Primary',button('Accept',176,652,True)[0])
s+=text('48px targets / 24px icons / 2px focus ring / 4px offset',48,732,12,C['muted'])
s+=iconbutton('more',768,528,soft=True)+rect(848,512,256,136,C['white'],24,C['line'])
s+=icon('history',868,532,20,C['ink'])+text('Transfer history',904,532,14,C['ink'])+icon('chevron',1060,532,20,C['ink'])
s+=rect(868,572,216,1,C['line'])+icon('laptop',868,596,20,C['ink'])+text('Device details',904,596,14,C['ink'])
s+=text('Select a node → More → Transfer history',768,680,14,C['muted'])
s+=text('Icon actions have tooltips, accessible names and visible focus.',768,732,12,C['muted'])
s+=rule(780)+section('Activity and recovery',48,812)
for i,(title_,desc) in enumerate([('Transferring','3px gray track + one 4px blue segment.'),('1.8s loop','Linear motion. About 12% of the path.'),('Reduced motion','Static segment. No looping activity.'),('Paused / Disconnected','Static track + a short recovery sheet.')]):
    x=48+i*344
    s+=text(title_,x,868,16,C['ink'],600)+text(desc,x,908,12,C['muted'])
s+=text('The loop shows activity. It never implies a real completion percentage.',48,980,14,C['muted'])
(OUT/'03-components.svg').write_text(svg(s,1440,1040,'Components and interaction'),encoding='utf-8')
print('Generated product foundations and editable component specimens.')
