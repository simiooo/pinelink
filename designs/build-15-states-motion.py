from pathlib import Path
import runpy, json, math

ns=runpy.run_path(str(Path(__file__).with_name('build-14-meta-graph.py')))
p=ns['p']
globals().update({k:p[k] for k in ['C','rect','circle','path','group','icon','text','svg','node','header','fileicon','filegrid','logo','systembars','sheet_surface'] if k in p})
surface=ns['sheet_surface']
OUT=Path(__file__).parent/'15-states-motion'
OUT.mkdir(exist_ok=True)
ICONS=p['icon'].__globals__['ICONS']
ICONS.update(lock='M6 11h12v10H6z M8 11V7a4 4 0 0 1 8 0v4',ban='M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M6 6l12 12',refresh='M20 5v6h-6 M20 11a8 8 0 1 0-1 6',cursor='M4 3l4 17 4-6 7-2z')

def pill(label,x,y,w=None):
    w=w or 32+len(label)*7
    return rect(x-w/2,y,w,40,C['surface'],20)+text(label,x,y+10,14,C['ink'],400,'middle')

def pair(m=False,mode='quiet',cloud=False):
    ox,oy,tx,ty,sz=(195,680,195,264,80) if m else (504,648,696,248,96)
    d=f'M{ox} {oy-sz/2} C{ox-104} {oy-184} {tx+104} {ty+176} {tx} {ty+(28 if m else 32)}'
    s=header(m,'focused')
    if mode!='gone':s+=path(d,C['line'],3,extra='stroke-dasharray="4 8"' if mode=='waiting' else '')
    if mode=='running':
        # Reuse the exact single activity segment of the approved design.
        return p['transfer'](m)
    s+=node('nas' if cloud else 'laptop','Cloud vault' if cloud else 'MacBook Pro',tx,ty,56 if m else 64,status='cloud' if cloud else None)
    s+=node('phone' if m else 'laptop','This device',ox,oy,sz,own=True,status=None)
    return s

def panel(kind,m=False):
    cases={
      'rejected':('Request declined','MacBook Pro declined this request.','Close','Choose device','ban'),
      'expired':('Request expired','Send a new request when you\u2019re ready.','Close','Send again','history'),
      'failed':('Couldn\u2019t send files','Check your connection and try again.','Close','Try again','alert'),
      'unavailable':('File unavailable','Choose the file again to continue.','Close','Choose files','ban'),
      'prepared':('Ready to send','To MacBook Pro','Cancel','Send','send'),
      'async':('Save for later?','MacBook Pro is offline.','Cancel','Store in cloud','cloud'),
      'unreadable':('Couldn\u2019t read the file','Choose the file again to continue.','Close','Choose files','alert'),
      'cancel-confirm':('Cancel transfer?','The current transfer will stop.','Keep transfer','Cancel transfer','ban'),
      'task-controls':('Transfer options','To MacBook Pro','Cancel transfer','Pause','pause'),
    }
    title,body,a,b,kindicon=cases[kind]
    ready=kind in ['prepared','async']
    if m:x,y,w,h=0,(464 if ready else 536),390,(380 if ready else 308)
    else:x,y,w,h=400,(224 if ready else 256),400,(352 if ready else 280)
    px=x+(20 if m else 24);top=y+(40 if m else 24)
    s=surface(x,y,w,h,m)
    if m:s+=rect(179,y+12,32,4,C['border'],2)
    if ready:
        s+=icon(kindicon,px,top+2,24,C['ink'])+text(title,px+40,top,20,C['ink'],600)+text(body,px+40,top+36,14,C['muted'])
        s+=rect(px,top+92,w-(40 if m else 48),80,C['surface'],20)+fileicon('doc',px+12,top+108,48)
        s+=text('Design source.fig',px+72,top+108,14,C['ink'],600)+text('and 2 more files',px+72,top+136,12,C['muted'])
        by=top+208
    else:
        s+=icon(kindicon,px,top,24,C['ink'])+text(title,px,top+44,20,C['ink'],600)+text(body,px,top+84,14,C['muted'])
        by=top+144
    bw=(w-(40 if m else 48)-12)/2
    controls=[dict(label=a,x=px,y=by,width=bw,primary=False),dict(label=b,x=px+bw+12,y=by,width=bw,primary=True)]
    return group(kind+' panel',s),controls

def transfer_state(kind,m=False):
    s=pair(m,mode='waiting' if kind in ['waiting','connecting','queued'] else 'gone' if kind in ['cancelled','delivered','received'] else 'quiet',cloud=kind=='stored')
    cx=195 if m else 600
    if kind in ['waiting','connecting','queued','verifying','stored','cancelled']:
        label={'waiting':'Waiting for MacBook Pro','connecting':'Connecting','queued':'Queued','verifying':'Confirming delivery','stored':'Stored in cloud','cancelled':'Cancelled'}[kind]
        s+=pill(label,cx,448 if m else 416)
    elif kind in ['delivered','received']:
        tx,ty=((195,680) if m else (504,648)) if kind=='received' else ((195,264) if m else (696,248))
        s+=circle(tx+24,ty+24,14,C['white'])+circle(tx+24,ty+24,11,C['ink'])+icon('check',tx+17,ty+17,14,C['white'],2)
        s+=pill('Received' if kind=='received' else 'Delivered',cx,448 if m else 416,112)
    if kind in ['rejected','expired','failed','unavailable','cancel-confirm','task-controls']:
        s=group('dimmed context',s,'opacity=".13"')
        pane,controls=panel(kind,m)
        if m:s+=rect(0,48,390,796,C['ink'],0,extra='opacity=".06"')
        s+=pane
        if m:s+=rect(0,0,390,48,C['white'])+systembars()
        return s,controls
    return s,[]

def drag_state(kind,m=False):
    s=header(m,'focused')+p['graph'](m,1 if kind=='entered' else .28)
    tx,ty=(107,248) if m else (312,312)
    if kind not in ['prepared','async','unreadable']:
        s+=pill('Drop on a device',195 if m else 600,128 if m else 112,176)
    if kind in ['near','target','exit']:
        s+=circle(tx,ty,56,C['soft'],C['blue'],1.5,extra='stroke-dasharray="3 6"')
        s+=node('laptop','MacBook Pro',tx,ty,72,status=None)
        s+=circle(tx+26,ty+26,14,C['white'])+circle(tx+26,ty+26,11,C['blue'])+icon('plus',tx+18,ty+18,16,C['white'],2)
        s+=text('Drop to send',tx,ty+84,14,C['blue'],600,'middle')
        # The cursor and file chip are annotations of the system drag image, not extra in-app UI.
        gx,gy=tx+(52 if kind=='near' else 12),ty+(40 if kind=='near' else 8)
        s+=group('System drag preview',rect(gx,gy,176,48,C['white'],16,C['line'])+fileicon('doc',gx+8,gy+9,28)+text('Design source.fig',gx+44,gy+14,12,C['ink'])+icon('cursor',gx-12,gy-12,24,C['ink']))
    elif kind in ['offline','revoked','self']:
        label='Device is offline' if kind=='offline' else 'Device unavailable' if kind=='revoked' else 'Choose another device'
        if kind=='self':tx,ty=(195,680) if m else (600,648)
        size=(80 if m else 96) if kind=='self' else (56 if m else 64)
        s+=circle(tx,ty,size/2+24,C['surface'],C['border'],1.5,extra='stroke-dasharray="3 6"')
        s+=node('laptop' if not m else 'phone' if kind=='self' else 'laptop','This device' if kind=='self' else 'MacBook Pro',tx,ty,size,own=kind=='self',status=None)
        s+=circle(tx+26,ty+26,14,C['white'])+icon('lock' if kind=='revoked' else 'ban',tx+16,ty+16,20,C['ink'])
        s+=text(label,tx,ty+(84 if kind!='self' else -100),14,C['ink'],600,'middle')
    elif kind in ['prepared','async','unreadable']:
        pane,controls=panel(kind,m);s=group('dimmed context',header(m,'focused')+p['graph'](m),'opacity=".13"')
        if m:s+=rect(0,48,390,796,C['ink'],0,extra='opacity=".06"')
        s+=pane
        if m:s+=rect(0,0,390,48,C['white'])+systembars()
        return s,controls
    return s,[]

manifest=[]
transfer_kinds=['waiting','connecting','queued','verifying','delivered','stored','rejected','expired','failed','cancelled','unavailable','received','cancel-confirm','task-controls']
drag_kinds=['entered','near','target','offline','revoked','self','prepared','async','unreadable']
for device,m,w,h,kinds,fn in [('desktop',False,1200,800,transfer_kinds,transfer_state),('drag',False,1200,800,drag_kinds,drag_state),('mobile',True,390,844,['waiting','delivered','failed','prepared','async','unavailable','received','cancel-confirm','task-controls'],None)]:
    for i,kind in enumerate(kinds):
        body,controls=(drag_state(kind,m) if kind in ['prepared','async'] else transfer_state(kind,m)) if fn is None else fn(kind,m)
        name=f'{device}-{i+1:02}-{kind}'
        (OUT/(name+'.svg')).write_text(svg(body,w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',width=w,height=h,kind=kind,device=device,controls=controls))

def board(device,cols):
    items=[a for a in manifest if a['device']==device]
    iw,ih=items[0]['width'],items[0]['height'];pad,gap,top=48,48,148
    w=pad*2+cols*iw+(cols-1)*gap;rows=math.ceil(len(items)/cols);h=top+rows*(ih+88)+pad
    s=rect(0,0,w,h,C['surface'])+logo(48,40,32)+text('pinelink',92,40,24,C['ink'],600)+text({'desktop':'Transfer states','drag':'Drag and drop','mobile':'Mobile states'}[device],48,88,16,C['muted'])
    for i,a in enumerate(items):
        x,y=pad+(i%cols)*(iw+gap),top+(i//cols)*(ih+88)
        s+=text(a['kind'].replace('-',' ').capitalize(),x,y-36,16,C['ink'],600)
        source=(OUT/a['file']).read_text(encoding='utf-8');inner=source[source.index('>')+1:source.rindex('</svg>')]
        # Static review boards include the native action geometry used after import.
        for c in a['controls']:inner+=ns['button'](c['label'],c['x'],c['y'],c['primary'],width=c['width'])[0]
        s+=group(a['name'],inner,f'transform="translate({x} {y})"')+rect(x,y,iw,ih,'none',0,C['line'])
    return svg(s,w,h,device+' overview')

for device,cols in [('desktop',2),('drag',3),('mobile',3)]:(OUT/(device+'-board.svg')).write_text(board(device,cols),encoding='utf-8')

def section(label,x,y):return text(label,x,y,20,C['ink'],600)
s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Node language and authority',48,104,20,C['ink'],600)+rect(48,152,1344,1,C['line'])
states=[('Online','online',False),('Connecting','waiting',False),('Offline','offline',False),('Revoked',None,False),('Cloud', 'cloud',False),('Selected','online',True),('Drop allowed',None,False),('Drop blocked',None,False)]
for i,(label,status,selected) in enumerate(states):
    x,y=216+(i%4)*336,288+(i//4)*256
    if label=='Drop allowed':s+=circle(x,y,56,C['soft'],C['blue'],1.5,extra='stroke-dasharray="3 6"')
    s+=node('laptop','MacBook Pro',x,y,96 if selected else 64,selected=selected,status=status,opacity=.42 if status=='offline' else 1)
    if label in ['Revoked','Drop blocked']:s+=circle(x+24,y+24,14,C['white'])+icon('lock' if label=='Revoked' else 'ban',x+14,y+14,20,C['ink'])
    if label=='Drop allowed':s+=circle(x+24,y+24,11,C['blue'])+icon('plus',x+16,y+16,16,C['white'])
    s+=text(label,x,y+108,16,C['ink'],600,'middle')
s+=rect(48,728,1344,1,C['line'])+section('Trust, reachability and interaction are independent',48,768)
for i,line in enumerate(['ACTIVE / REVOKED is device authorization. ACTIVE does not prove the device is online.','Online / Offline / Connecting is derived from reachability. Cloud is a storage capability.','Hover / Focus / Selected / Drop allowed / Drop blocked is local UI state, never a server status.','Offline targets may offer explicit asynchronous storage; online-only drops stay blocked.','A drag gesture freezes node positions. The hit target extends 24px beyond the node radius.']):s+=text(line,48,824+i*44,14,C['muted'])
(OUT/'04-node-states.svg').write_text(svg(s,1440,1104,'Node state language'),encoding='utf-8')

motion=[
 ('Node enters','200ms','opacity 0 / scale .96 / y +8','opacity 1 / scale 1 / y 0','120ms fade'),
 ('Node leaves','160ms','current position','opacity 0 / scale .96 / y -4','120ms fade'),
 ('Select / unselect','180ms','64px / 100% context','96px / 13% context','instant geometry'),
 ('Drag target enters','120ms','64px / no halo','72px / 112px halo / plus','instant halo'),
 ('Drag target leaves','100ms','active target','normal node / no halo','instant reset'),
 ('Drop accepted','140ms','system drag image','image ends / review opens','no chip motion'),
 ('Desktop panel enters','180ms','opacity 0 / y +8 / scale .98','opacity 1 / y 0 / scale 1','120ms fade'),
 ('Desktop panel leaves','120ms','open panel','opacity 0 / y +4 / scale .985','120ms fade'),
 ('Mobile sheet enters','240ms','opacity 0 / y +24','opacity 1 / y 0','120ms fade'),
 ('Mobile sheet leaves','180ms','open sheet','opacity 0 / y +24','120ms fade'),
 ('Transfer activity','1800ms','one 12% blue segment','linear loop along same path','static segment'),
 ('Delivery confirmed','320ms','activity segment','140ms settle / 180ms curve exit','120ms fade'),
]
s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Motion contract',48,104,20,C['ink'],600)+rect(48,152,1344,1,C['line'])
for x,label in [(48,'Transition'),(320,'Duration'),(448,'From'),(800,'To'),(1200,'Reduced motion')]:s+=text(label,x,184,14,C['muted'],600)
for i,(label,ms,a,b,rm) in enumerate(motion):
    y=232+i*48
    for x,copy in [(48,label),(320,ms),(448,a),(800,b),(1200,rm)]:s+=text(copy,x,y,12,C['ink'] if x==48 else C['muted'],600 if x==48 else 400)
    s+=rect(48,y+36,1344,1,C['line'])
for i,line in enumerate(['Enter / change: cubic-bezier(.16, 1, .3, 1). Exit: cubic-bezier(.4, 0, 1, 1). Activity: linear.','No bounce, orbiting controls, repeated glow or fake percentage. Hidden pages pause continuous motion.','State updates interrupt the old transition. Stable node IDs preserve position and focus; stale versions cannot show success.','Success requires a delivery receipt or verified P2P completion. Upload completion only means stored, not delivered.']):s+=text(line,48,840+i*44,14,C['muted'])
(OUT/'08-motion-contract.svg').write_text(svg(s,1440,1056,'Motion contract'),encoding='utf-8')

s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Entry, transition and exit keyframes',48,104,20,C['ink'],600)+rect(48,152,1344,1,C['line'])
for row,label in enumerate(['Node entry / 200ms','Drop target / 120ms','Panel entry / 180ms','Delivery / settle and return']):
    yy=192+row*240;s+=text(label,48,yy,16,C['ink'],600)
    for col,t in enumerate(['Start','Middle','End']):
        cx=240+col*464;s+=text(t,cx,yy+36,12,C['muted'],400,'middle')
        cy=yy+120
        if row==0:s+=group('Node entry '+t,node('laptop','MacBook Pro',cx,cy,64,opacity=[0,.55,1][col]),f'transform="translate(0 {[8,3,0][col]})"')
        elif row==1:
            if col:s+=circle(cx,cy,56,C['soft'],C['blue'],1.5,extra='stroke-dasharray="3 6"')
            s+=node('laptop','MacBook Pro',cx,cy,[64,68,72][col],status=None)
            if col:s+=circle(cx+26,cy+26,11,C['blue'])+icon('plus',cx+18,cy+18,16,C['white'])
        elif row==2:
            opacity=[0,.6,1][col];dy=[8,3,0][col]
            pane=rect(cx-176,cy-48+dy,352,136,C['white'],24,C['line'])+text('Ready to send',cx-152,cy-24+dy,20,C['ink'],600)+text('To MacBook Pro',cx-152,cy+16+dy,14,C['muted'])
            s+=group('Panel '+t,pane,f'opacity="{opacity}"')
        else:
            s+=path(f'M{cx-120} {cy+24} C{cx-40} {cy+24} {cx+40} {cy-24} {cx+120} {cy-24}',C['line'],3,extra=f'opacity="{1 if col<2 else .15}"')
            if col==0:s+=path(f'M{cx-16} {cy+4.77} C{cx-5.33} {cy+1.59} {cx+5.33} {cy-1.59} {cx+16} {cy-4.77}',C['blue'],4)
            else:s+=circle(cx+120,cy-24,12,C['ink'])+icon('check',cx+112,cy-32,16,C['white'])
(OUT/'08-keyframes.svg').write_text(svg(s,1440,1192,'Motion keyframes'),encoding='utf-8')
(OUT/'manifest.json').write_text(json.dumps(manifest,indent=2),encoding='utf-8')
(OUT/'motion.json').write_text(json.dumps(dict(enter=[.16,1,.3,1],exit=[.4,0,1,1],activity_ms=1800,transitions=[dict(name=a,duration=b,start=c,end=d,reduced=e) for a,b,c,d,e in motion]),indent=2),encoding='utf-8')
mapping=[('device ACTIVE','Authorization valid','Reachability still decides online / offline.'),('device REVOKED','Locked / blocked','Do not send. Apply the server cancellation.'),('transfer PENDING','Waiting / Receive request','Wait for the recipient action.'),('transfer ACCEPTED','Connecting','Acceptance is not a delivery receipt.'),('task QUEUED','Queued','Keep the path static.'),('task RUNNING','Sending / Receiving','One activity segment. Reverse for receive.'),('task PAUSED','Paused','Only this state can resume the same task.'),('task / transfer FAILED','Try again','Start a new attempt. Do not resume FAILED.'),('REJECTED / CANCELLED','Terminal feedback','Respect the actor; do not auto resend.'),('transfer EXPIRED','Request expired','New request only if the file is still valid.'),('file EXPIRED / DELETED','File unavailable','Choose a valid file again.'),('UPLOAD COMPLETED','Stored in cloud','Stored does not mean received.'),('Verified delivery receipt','Delivered / Received','Success marker belongs to the receiver.'),('task DELETED','Hidden from history','Remove after acknowledgement, then reflow.'),('UI dragging / hover','Drop allowed / blocked','Capture radius +24px. Exit hysteresis +8px.'),('UI staged / submitting','Ready to send','Drop stages files; Send creates the request.'),('UI verifying / HTTP 409','Confirming / refreshed state','Latest version wins. Never invent success.')]
s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('State authority and transitions',48,104,20,C['ink'],600)+rect(48,152,1344,1,C['line'])
for x,label in [(48,'Source'),(432,'UI'),(736,'Rule')]:s+=text(label,x,184,14,C['muted'],600)
for i,(source,label,rule_) in enumerate(mapping):
    y=232+i*44
    for x,copy in [(48,source),(432,label),(736,rule_)]:s+=text(copy,x,y,12,C['ink'] if x<736 else C['muted'])
    s+=rect(48,y+32,1344,1,C['line'])
s+=text('Source: devices.go / transfer.go / tasks.go / p2p.go / pull.go / SQL migrations 001, 003, 005.',48,1028,12,C['muted'])
s+=text('Server state, reachability and local interaction are separate dimensions. No new backend enums.',48,1072,14,C['muted'])
(OUT/'04-state-mapping.svg').write_text(svg(s,1440,1144,'State authority mapping'),encoding='utf-8')
print(f'Generated {len(manifest)} state screens and 4 design boards in {OUT}')
