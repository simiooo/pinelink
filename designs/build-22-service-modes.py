"""Editable OpenPencil proposal: official by default, opt-in self hosting."""
from pathlib import Path
from html import escape
import json

OUT = Path(__file__).parent / '22-service-modes'
OUT.mkdir(exist_ok=True)
C = dict(blue='#0866FF', ink='#111111', muted='#737373', faint='#A3A3A3',
         line='#E5E5E5', surface='#F5F5F5', white='#FFFFFF')
P = {
 'laptop':'M4 4h16v12H4z M2 20h20l-2-4H4z',
 'phone':'M7 2h10v20H7z M11 19h2',
 'cloud':'M6 18h12a4 4 0 0 0 1-7.8A7 7 0 0 0 5 9a4.5 4.5 0 0 0 1 9z',
 'more':'M5 12h.01 M12 12h.01 M19 12h.01',
 'plus':'M12 5v14 M5 12h14', 'close':'M6 6l12 12 M18 6L6 18',
 'back':'M19 12H5 M11 6l-6 6 6 6', 'chevron':'M9 5l7 7-7 7',
 'down':'M6 9l6 6 6-6', 'server':'M4 3h16v8H4z M4 14h16v7H4z M7 7h.01 M7 18h.01',
 'globe':'M3 12h18 M12 3a15 15 0 0 0 0 18 M12 3a15 15 0 0 1 0 18 M3 12a9 9 0 1 0 18 0 9 9 0 1 0-18 0',
 'info':'M12 11v6 M12 7h.01 M12 3a9 9 0 1 0 0 18 9 9 0 1 0 0-18',
 'check':'M5 12l4 4L19 6', 'copy':'M8 8h12v13H8z M4 16V3h12',
 'shield':'M12 3l8 3v6c0 5-8 9-8 9s-8-4-8-9V6z',
 'pause':'M8 5v14 M16 5v14', 'alert':'M12 4l10 17H2z M12 10v5 M12 18h.01'
}
def g(name, s, extra=''): return f'<g id="{escape(name)}" {extra}>{s}</g>'
def rect(x,y,w,h,fill='#FFFFFF',r=0,stroke=None):
    if fill == 'none':
        return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="#FFFFFF" opacity="0"/>'
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"'+(f' stroke="{stroke}"' if stroke else '')+'/>'
def path(d,color='#111111',weight=1.7):
    return f'<path d="{d}" fill="none" stroke="{color}" stroke-width="{weight}" stroke-linecap="round" stroke-linejoin="round"/>'
def text(s,x,y,size=15,color='#111111',weight=400,anchor='start'):
    if not s: return ''
    return f'<text x="{x}" y="{y+size*.82+3}" font-family="Segoe UI" font-size="{size}" font-weight="{weight}" fill="{color}" text-anchor="{anchor}">{escape(s)}</text>'
def icon(kind,x,y,size=24,color='#111111'):
    return g('Icon / '+kind,path(P[kind],color,1.8),f'transform="translate({x} {y}) scale({size/24})"')
def hit(kind,x,y): return g('48px action / '+kind,rect(x,y,48,48,'none')+icon(kind,x+12,y+12))
def pill(label,x,y,w,primary=True):
    return g('Action / '+label,rect(x,y,w,48,C['blue'] if primary else C['surface'],24)+text(label,x+w/2,y+14,15,C['white'] if primary else C['ink'],600,'middle'))
def node(kind,label,x,y,d):
    return g('Device / '+label,f'<circle cx="{x}" cy="{y}" r="{d/2}" fill="{C["surface"]}"/>'+icon(kind,x-d/4,y-d/4,d/2)+text(label,x,y+d/2+8,13,C['muted'],400,'middle'))
def system_chrome():
    return text('9:41',24,12,13,C['ink'],600)+rect(337,16,24,10,C['ink'],3)+rect(141,831,108,4,C['ink'],2)
def base(m=False,custom=False):
    w,h=(390,844) if m else (1200,800)
    s=rect(0,0,w,h)
    if m:
        s+=system_chrome()
        s+=text('pinelink',195,62,22,C['ink'],600,'middle')+hit('more',322,48)
        if custom:s+=hit('server',8,48)
        s+=node('laptop','MacBook Pro',104,232,56)+node('phone','iPhone',286,232,56)+node('laptop','This device',195,658,80)
        s+=path('M195 618C195 410 104 410 104 260',C['line'],2)+path('M195 618C195 410 286 410 286 260',C['line'],2)
    else:
        s+=text('pinelink',32,32,24,C['ink'],600)+hit('plus',1064,24)+hit('more',1120,24)
        if custom:s+=hit('server',1008,24)
        s+=node('laptop','MacBook Pro',368,224,64)+node('phone','iPhone',600,192,64)+node('cloud','Cloud',832,224,64)+node('laptop','This device',600,664,96)
        s+=path('M600 616C600 408 368 440 368 256',C['line'],2)+path('M600 616C600 424 600 400 600 224',C['line'],2)+path('M600 616C600 408 832 440 832 256',C['line'],2)
    return s
def bg(m=False,custom=False): return g('Inactive node graph',base(m,custom),'opacity=".13"')+(system_chrome() if m else '')
def panel(m=False,h=520,title='Connect your server'):
    x,y,w=(0,48,390) if m else (320,(800-h)/2,560)
    px=x+(20 if m else 24); py=y+(72 if m else 28); inner=w-(40 if m else 48)
    s=rect(x,y,w,762 if m else h,C['white'],0 if m else 28,None if m else C['line'])
    if m:s+=hit('back',8,48)+text(title,20,120,24,C['ink'],600)
    else:s+=text(title,px,py,22,C['ink'],600)+hit('close',x+w-60,y+16)
    return s,x,y,w,px,(168 if m else py+64),inner
def field(label,value,x,y,w,focus=False):
    s=text(label,x,y,13,C['muted'])+rect(x,y+28,w,48,C['white'],12,C['blue'] if focus else C['line'])+text(value,x+16,y+42)
    return g('Field / '+label,s)
def row(label,value,x,y,w,arrow=True):
    s=text(label,x,y+12,15)+text(value,x+w-(28 if arrow else 0),y+12,14,C['muted'],400,'end')
    if arrow:s+=icon('chevron',x+w-20,y+12,20,C['muted'])
    return g('Row / '+label,rect(x,y,w,48,'none')+s)
def official(m=False):
    s=base(m)
    x,y,w=(0,592,390) if m else (904,88,264)
    s+=rect(x,y,w,218 if m else 200,C['white'],24,C['line'])
    if m:s+=rect(179,y+12,32,4,C['line'],2)
    py=y+(32 if m else 16)
    for i,(kind,label) in enumerate([('globe','Language'),('info','About Pinelink'),('server','Use your own server')]):
        yy=py+i*56
        s+=g('Menu / '+label,rect(x+12,yy,w-24,48,'none')+icon(kind,x+24,yy+12)+text(label,x+60,yy+14))
    return s,[]
def connect(m=False,error=False,advanced=False,active=False):
    title='Your server' if active else 'Connect your server'
    h=648 if active else 592 if advanced else 380 if error else 360
    s,x,y,w,px,fy,inner=panel(m,h,title)
    s=bg(m,active)+s
    s+=field('Server address','https://files.example.org',px,fy,inner,not active)
    if error:s+=icon('alert',px,fy+88,16)+text('Couldn’t reach this server.',px+24,fy+88,13,C['ink'])
    gy=fy+128 if error else fy+108
    s+=g('Advanced disclosure',row('Advanced','',px,gy,inner,False)+icon('down' if advanced else 'chevron',px+inner-24,gy+12,20,C['muted']))
    if advanced:
        for i,(label,value) in enumerate([('Server identity','Verify fingerprint'),('Signaling','Use service address'),('ICE servers','Automatic'),('Transfer route','Automatic')]):
            yy=gy+64+i*56
            s+=row(label,value,px,yy,inner)+rect(px,yy+48,inner,1,C['line'])
    by=728 if m else y+h-80
    if active:s+=g('Action / Use Pinelink',rect(px,by-64,inner,48,'none')+text('Use Pinelink',px,by-52,15,C['blue'],600))
    s+=pill('Save changes' if active else 'Try again' if error else 'Connect',px,by,inner)
    return s,[dict(label='Advanced',x=px,y=gy,width=inner,height=48),dict(label='Connect',x=px,y=by,width=inner,height=48)]
def trust(m=False):
    s,x,y,w,px,fy,inner=panel(m,408,'Trust this server?')
    s=bg(m)+s+icon('shield',px,fy,24)+text('files.example.org',px+36,fy,16,C['ink'],600)
    s+=text('Compare this fingerprint with your server.',px,fy+48,13,C['muted'])
    fingerprint=['89c4  705d  93a8  8d02','2be1  c0d8  4f3e  a29b','6c71  54e0  e1d3  c880','5a19  62fb  8d07  f4e3']
    s+=g('Fingerprint / SHA-256', ''.join(text(part,px,fy+96+i*28,16) for i,part in enumerate(fingerprint))+hit('copy',px+inner-48,fy+88))
    by=728 if m else y+408-80
    s+=pill('Trust and connect',px,by,inner)
    return s,[]
def switching(m=False):
    s,x,y,w,px,fy,inner=panel(m,284,'Pause before switching?')
    s=bg(m)+s+text('2 transfers are still active.',px,fy,15,C['muted'])
    s+=g('Current transfer',icon('pause',px,fy+56)+text('Product demo.mp4',px+36,fy+56,15,C['ink'],600))
    by=728 if m else y+284-80
    s+=pill('Stay',px,by,96,False)+pill('Pause and switch',px+108,by,inner-108)
    return s,[]

cases=[('official-menu','Official / no server configuration',official,{}),
       ('custom-connect','Custom / connect',connect,{}),
       ('custom-advanced','Custom / advanced',connect,dict(advanced=True)),
       ('server-trust','Custom / confirm identity',trust,{}),
       ('connection-error','Custom / connection error',connect,dict(error=True)),
       ('custom-active','Custom / edit connected server',connect,dict(advanced=True,active=True)),
       ('switch-active','Switch / active transfers',switching,{})]
manifest=[]
for i,(slug,title,fn,kw) in enumerate(cases):
    for m in [False,True]:
        body,controls=fn(m,**kw)
        w,h=(390,844) if m else (1200,800)
        name=('Mobile' if m else 'Desktop')+' / '+title
        filename=('mobile' if m else 'desktop')+'-'+slug+'.svg'
        (OUT/filename).write_text(f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{body}</svg>',encoding='utf-8')
        manifest.append(dict(name=name,file=filename,width=w,height=h,x=(i%3)*1680+(1264 if m else 0),y=(i//3)*960,controls=controls))
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'frames':len(manifest),'palette':C,'output':str(OUT)}))
