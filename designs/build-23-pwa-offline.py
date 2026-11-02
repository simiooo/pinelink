"""SIM-859: editable proposals for local files and offline intents (not runtime UI)."""
from pathlib import Path
from html import escape
import json

OUT = Path(__file__).parent / '23-pwa-offline'
OUT.mkdir(exist_ok=True)
C = dict(ink='#111111', muted='#737373', faint='#A3A3A3', line='#E5E5E5', surface='#F5F5F5', white='#FFFFFF', blue='#0866FF')
P = {
 'laptop':'M4 4h16v12H4z M2 20h20l-2-4H4z', 'phone':'M7 2h10v20H7z M11 19h2',
 'cloud':'M6 18h12a4 4 0 0 0 1-7.8A7 7 0 0 0 5 9a4.5 4.5 0 0 0 1 9z',
 'cloud-off':'M3 3l18 18 M6 18h10 M5 9a4.5 4.5 0 0 0 1 9 M10 5a7 7 0 0 1 9 6 4 4 0 0 1 2 5',
 'more':'M5 12h.01 M12 12h.01 M19 12h.01', 'close':'M6 6l12 12 M18 6L6 18',
 'back':'M19 12H5 M11 6l-6 6 6 6', 'clock':'M12 3a9 9 0 1 0 0 18 9 9 0 1 0 0-18 M12 7v5l3 2',
 'check':'M5 12l4 4L19 6', 'alert':'M12 4l10 17H2z M12 10v5 M12 18h.01',
 'file':'M5 2h9l5 5v15H5z M14 2v6h5 M8 12h8 M8 16h6',
 'folder':'M2 7h8l2 2h10v12H2z M2 7V4h7l3 3h10v2',
 'repeat':'M4 8h14l-3-3 M20 16H6l3 3 M18 8l-3 3 M6 16l3-3',
 'drive':'M5 4h14l3 12H2z M2 16h20v5H2z M6 18h.01 M10 18h.01',
 'mark':'M5 15c0-5 3-9 7-9s7 3 7 7-3 7-7 7c-2 0-4-1-4-3s2-4 4-4h3',
}
def group(name, body, attrs=''): return f'<g id="{escape(name)}" {attrs}>{body}</g>'
def rect(x,y,w,h,fill=C['white'],r=0,stroke=None):
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"'+(f' stroke="{stroke}"' if stroke else '')+'/>'
def text(label,x,y,size=15,color=C['ink'],weight=400,anchor='start'):
    return f'<text x="{x}" y="{y+size*.82+3}" font-family="Segoe UI" font-size="{size}" font-weight="{weight}" fill="{color}" text-anchor="{anchor}">{escape(label)}</text>'
def line(d,color=C['ink'],width=1.7): return f'<path d="{d}" fill="none" stroke="{color}" stroke-width="{width}" stroke-linecap="round" stroke-linejoin="round"/>'
def icon(kind,x,y,size=24,color=C['ink']): return group('Icon / '+kind,line(P[kind],color,1.5*24/size if kind in ['file','folder'] else 1.7),f'transform="translate({x} {y}) scale({size/24})"')
def hit(kind,x,y): return group('Action / '+kind,rect(x,y,48,48)+icon(kind,x+12,y+12))
def button(label,x,y,w,primary=True):
    return group('Action / '+label,rect(x,y,w,48,C['blue'] if primary else C['surface'],24)+text(label,x+w/2,y+14,15,C['white'] if primary else C['ink'],600,'middle'))
def badge(kind,x,y,label):
    return group('Tooltip / '+label,f'<circle cx="{x}" cy="{y}" r="8" fill="{C["white"]}" stroke="{C["line"]}"/>'+icon(kind,x-5,y-5,10))
def node(kind,label,x,y,d=64,status=None,opacity=1):
    size=14 if label=='This device' and d==96 else 12 if d==56 else 13
    body=f'<circle cx="{x}" cy="{y}" r="{d/2}" fill="{C["surface"]}"/>'+icon(kind,x-d/4,y-d/4,d/2)+text(label,x,y+d/2+8,size,C['ink'] if label=='This device' else C['muted'],600 if label=='This device' else 400,'middle')
    if status in ['online','offline']:
        bx,by=x+d*.34,y+d*.34
        body+=group('Tooltip / '+status,f'<circle cx="{bx}" cy="{by}" r="7" fill="{C["white"]}"/><circle cx="{bx}" cy="{by}" r="4" fill="{C["ink"] if status=="online" else C["faint"]}"/>')
    elif status:body+=badge(status,x+d*.34,y+d*.34,{'clock':'Saved locally; waiting for connection','cloud-off':'Central service unavailable'}[status])
    return group('Device / '+label,body,f'opacity="{opacity}"')
def mix(a,b,t): return tuple(a[i]*(1-t)+b[i]*t for i in range(2))
def split(p,t):
    a,b,c=mix(p[0],p[1],t),mix(p[1],p[2],t),mix(p[2],p[3],t)
    d,e=mix(a,b,t),mix(b,c,t)
    f=mix(d,e,t)
    return (p[0],a,d,f),(f,e,c,p[3])
def curve(p): return 'M%.2f %.2f C%.2f %.2f %.2f %.2f %.2f %.2f'%tuple(v for point in p for v in point)
def at(p,t): return split(p,t)[0][-1]
def arc_t(p,ratio):
    points=[at(p,i/400) for i in range(401)]
    distances=[sum((points[i][j]-points[i-1][j])**2 for j in range(2))**.5 for i in range(1,401)]
    target=sum(distances)*ratio; total=0
    for i,d in enumerate(distances):
        if total+d>=target:return (i+(target-total)/d)/400
        total+=d
    return 1
def graph(m=False,mode='offline-home',dim=False):
    w,h=(390,844) if m else (1200,800)
    s=rect(0,0,w,h)
    system=''
    if m:
        system=text('9:41',24,12,13,C['ink'],600)+rect(337,16,24,10,C['ink'],3)+rect(141,831,108,4,C['ink'],2)
        if not dim:s+=system
        s+=text('pinelink',195,62,20,C['ink'],600,'middle')+hit('more',322,48)+group('Status 48px / Central service unavailable',rect(8,48,48,48)+icon('cloud-off',20,60,24,C['muted']))
        local=(195,658,80);target=(104,232,56)
        p=((195,618),(195,420),(104,420),(104,260))
    else:
        s+=icon('mark',32,32,32)+text('pinelink',76,32,24,C['ink'],600)+hit('more',1120,24)+group('Status 48px / Central service unavailable',rect(1064,24,48,48)+icon('cloud-off',1076,36,24,C['muted']))
        local=(600,664,96);target=(368,224,64)
        p=((600,616),(600,440),(368,440),(368,256))
    if mode in ['offline-home','peer-offline']:
        s+=node('laptop','MacBook Pro',*target,status='offline' if mode=='peer-offline' else 'online',opacity=.35 if mode=='peer-offline' else 1)+node('phone','iPhone',286 if m else 600,232 if m else 192,56 if m else 64,status='offline',opacity=.35)
        if not m:s+=node('cloud','Cloud',832,224,64,'cloud-off',.35)
    else:
        s+=line(curve(p),C['line'],3)
        if mode in ['lan-active','cancel-pending']:
            t=arc_t(p,.42); s+=line(curve(split(p,t)[0]),C['blue'],3)
            if mode=='lan-active':
                a,b=arc_t(p,.23),arc_t(p,.30); s+=line(curve(split(split(p,b)[0],a/b)[1]),'#80B1FF',3)
        s+=node('laptop','MacBook Pro',*target,opacity=.35 if mode in ['queued','cancel-pending'] else 1)
    s+=node('laptop' if not m else 'phone','This device',*local,status='clock' if mode in ['queued','cancel-pending'] else None)
    return group('Inactive node graph' if dim else 'Node graph',s,'opacity=".13"' if dim else '')+(system if dim else '')
def panel(m,title='MacBook Pro',height=536):
    x,y,w=(0,48,390) if m else (608,184 if height==400 else 112,560)
    px=x+(20 if m else 24); content=168 if m else y+88
    s=rect(x,y,w,762 if m else height,C['white'],0 if m else 28,None if m else C['line'])
    if m:s+=hit('back',8,48)+text(title,20,120,24,C['ink'],600)
    else:s+=text(title,px,y+28,22,C['ink'],600)+hit('close',x+w-60,y+16)
    return s,px,content,w-(40 if m else 48),728 if m else y+height-80
FILES=[('file','Design source.fig'),('folder','Photos'),('file','Presentation.key')]
def files_grid(px,y,w,states=('clock','clock','clock'),files=FILES):
    s=''; cell=w/3
    for i,(kind,name) in enumerate(files):
        cx=px+cell*(i+.5)
        label={'clock':'Saved locally; waiting to send','repeat':'Reusable local copy','cloud':'Cloud copy; connect to restore'}[states[i]]
        s+=icon(kind,cx-28,y,56,C['muted'])+badge(states[i],cx+24,y+48,label)+text(name if len(name)<19 else name[:16]+'…',cx,y+72,13,C['ink'],400,'middle')
    return group('Files / icons, names, status tooltips',s)
def detail(m=False,kind='prepared'):
    title='This device' if kind=='history' else 'MacBook Pro'
    s,px,y,w,by=panel(m,title,400 if kind=='history' else 256 if kind in ['prepared','prepared-lan','saving'] else 304)
    s=graph(m,'cancel-pending' if kind=='cancel-pending' else 'queued' if kind in ['history','queued-detail'] else 'offline-home' if kind=='prepared-lan' else 'peer-offline',True)+s
    controls=[]
    if kind=='history':
        tabx=px; spans=[('All',32),('Reusable',72),('One-time',80),('Expired',64)]
        for label,width in spans:
            s+=group('Tab 48px / '+label,rect(tabx,y,width+16,48)+text(label,tabx+8,y,14,C['ink'] if label=='All' else C['muted']))
            if label=='All':s+=rect(tabx+8,y+30,20,2,C['ink'],1)
            tabx+=width+16
        s+=files_grid(px,y+64,w)+files_grid(px,y+188,w,('repeat','clock','cloud'),[('file','Contract.pdf'),('file','Notes.txt'),('file','Product demo.mp4')])
    else:
        state='alert' if kind in ['expired','space'] else 'drive' if kind in ['prepared','prepared-lan'] else 'clock'
        filename='Product demo.mp4' if kind=='cancel-pending' else 'Design source.fig'
        s+=icon('file',px,y,56,C['muted'])+badge(state,px+50,y+48,{'prepared':'Saved locally; no request sent','prepared-lan':'Saved locally; direct route available','saving':'Saving locally; not yet durable'}.get(kind,'Local intent / '+kind))+text(filename,px+68,y+16,16,C['ink'],600)
        if kind in ['prepared','prepared-lan','saving']:
            s+=button('Saving…' if kind=='saving' else 'Send' if kind=='prepared-lan' else 'Send when connected',px,by,w,kind!='saving')
            if kind!='saving':controls.append(dict(label='Persist send intent; receiving still needs consent',x=px,y=by,width=w,height=48))
        elif kind=='expired':
            s+=text('Request expired',px,y+80,16,C['ink'],600)+button('Keep file',px,by,112,False)+button('Choose device',px+124,by,w-124)
        elif kind=='space':
            s+=text('Not enough space',px,y+80,16,C['ink'],600)+button('Cancel',px,by,112,False)+button('Choose files',px+124,by,w-124)
        elif kind=='cancel-pending':
            s+=text('Cancel pending',px,y+80,16,C['ink'],600)+button('Close',px,by,w,False)
        else:
            s+=text('Waiting for connection',px,y+80,15,C['muted'])+button('Cancel queued transfer',px,by,w,False)
    return s,controls
cases=[('offline-home','Offline / local network remains available',None),('local-saving','Local / durability not yet confirmed','saving'),('local-prepared','Local / durable file, target not reachable','prepared'),('local-lan-ready','Local / durable file, direct route available','prepared-lan'),('queued','Intent / waiting, no transfer activity',None),('queued-detail','Intent / queued detail','queued-detail'),('local-history','History / local queue under All','history'),('lan-active','Direct / local network active without center',None),('expired','Conflict / expired request','expired'),('space','Storage / local commit failed','space'),('cancel-pending','Intent / cancellation not acknowledged','cancel-pending')]
manifest=[]
for i,(slug,label,kind) in enumerate(cases):
    for m in [False,True]:
        body,controls=detail(m,kind) if kind else (graph(m,slug),[])
        w,h=(390,844) if m else (1200,800)
        filename=('mobile' if m else 'desktop')+'-'+slug+'.svg'
        name=('Mobile' if m else 'Desktop')+' / '+label
        (OUT/filename).write_text(f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{body}</svg>',encoding='utf-8')
        manifest.append(dict(name=name,file=filename,width=w,height=h,x=(i%3)*1680+(1264 if m else 0),y=(i//3)*960,controls=controls))
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'frames':len(manifest),'output':str(OUT)}))
