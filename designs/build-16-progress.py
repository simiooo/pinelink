from pathlib import Path
import runpy, json, math

ns=runpy.run_path(str(Path(__file__).with_name('build-14-meta-graph.py')))
p=ns['p']
C=ns['C']; rect=ns['rect']; circle=ns['circle']; path=ns['path']; text=ns['text']; group=ns['group']; svg=ns['svg']; node=ns['node']; header=ns['header']; icon=ns['icon']; logo=ns['logo']
C['sheen']='#80B1FF'
OUT=Path(__file__).parent/'16-progress'
OUT.mkdir(exist_ok=True)

CASES={
 'sending':dict(title='Sending',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='12.8 MB/s · About 15 sec left',active=True,action='Pause'),
 'receiving':dict(title='Receiving',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='12.8 MB/s · About 15 sec left',active=True,action='Pause',receive=True),
 'estimating':dict(title='Sending',value='3%',fraction=.03125,bytes='16 MB of 512 MB',detail='Estimating time…',active=True,action='Pause'),
 'paused':dict(title='Paused',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='',action='Resume'),
 'reconnecting':dict(title='Reconnecting',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='Progress saved',action=None),
 'unknown':dict(title='Receiving',value='—',fraction=None,bytes='320 MB received',detail='Total size unavailable',active=True,action='Pause'),
 'queued':dict(title='Queued',value='',fraction=0,bytes='512 MB',detail='',action=None),
 'confirming':dict(title='Confirming delivery',value='100%',fraction=1,bytes='512 MB of 512 MB',detail='Waiting for MacBook Pro',action=None),
 'delivered':dict(title='Delivered',value='100%',fraction=1,bytes='512 MB',detail='',action='Open history',done=True),
 'cloud':dict(title='Stored in cloud',value='100%',fraction=1,bytes='512 MB',detail='',action='Open history',done=True,cloud=True),
 'failed':dict(title='Connection failed',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='Try sending again',action='Try again',failed=True),
 'batch':dict(title='Sending 3 files',value='62%',fraction=.625,bytes='320 MB of 512 MB',detail='12.8 MB/s · About 15 sec left',active=True,action='Pause',batch=True),
}

def progress_curve(points,fraction,start=0):
    # SVG pathLength is not persisted by OpenPencil. Import a real cubic subcurve.
    def lerp(a,b,t):return tuple(a[i]+(b[i]-a[i])*t for i in range(2))
    def split(ps,t):
        a,b,c=[lerp(ps[i],ps[i+1],t) for i in range(3)];d,e=lerp(a,b,t),lerp(b,c,t);f=lerp(d,e,t)
        return [ps[0],a,d,f],[f,e,c,ps[3]]
    def at(t):return split(points,t)[0][-1]
    pts=[at(i/1000) for i in range(1001)];lens=[0]
    for a,b in zip(pts,pts[1:]):lens.append(lens[-1]+math.dist(a,b))
    def time_at(f):
        target=lens[-1]*f;i=next(i for i,n in enumerate(lens) if n>=target)
        return 0 if i==0 else (i-1+(target-lens[i-1])/(lens[i]-lens[i-1]))/1000
    ta,tb=time_at(start),time_at(fraction)
    _,right=split(points,ta);mid,_=split(right,(tb-ta)/(1-ta))
    return f'M{mid[0][0]:.3f} {mid[0][1]:.3f} C'+ ' '.join(f'{v:.3f}' for pt in mid[1:] for v in pt)

def screen(kind,m=False,details=False):
    c=CASES[kind];ox,oy,tx,ty,sz=(195,680,195,248,80) if m else (504,648,696,248,96);bend=56 if m else 376
    points=[(ox,oy-sz/2),(bend,oy-176),(bend,ty+176),(tx,ty+(28 if m else 32))]
    if c.get('receive'):points=list(reversed(points))
    d=f'M{points[0][0]} {points[0][1]} C'+' '.join(str(v) for pt in points[1:] for v in pt)
    s=header(m,'focused')
    if not c.get('done'):
        s+=path(d,C['line'],3,extra='stroke-dasharray="4 8"' if kind in ['queued','unknown'] else '')
        if c['fraction'] and c['fraction']>0:
            s+=path(progress_curve(points,c['fraction']),C['blue'],3)
            if c.get('active'):s+=path(progress_curve(points,min(c['fraction'],c['fraction']*.42+.06),c['fraction']*.42),C['sheen'],3)
    s+=node('nas' if c.get('cloud') else 'laptop','Cloud vault' if c.get('cloud') else 'MacBook Pro',tx,ty,56 if m else 64,status='cloud' if c.get('cloud') else None)
    s+=node('phone' if m else 'laptop','This device',ox,oy,sz,own=True,status=None)
    if c.get('done'):
        s+=circle(tx+22,ty+22,14,C['white'])+circle(tx+22,ty+22,11,C['ink'])+icon('check',tx+15,ty+15,14,C['white'],2)
    if kind in ['paused','reconnecting','unknown','queued','confirming','delivered','cloud','failed']:
        label={'unknown':'Receiving','cloud':'Stored in cloud'}.get(kind,c['title'])
        s+=text(label,156 if m else 744,416,15,C['muted'])
    if not details:return s,[]
    s=group('dimmed transfer context',s,'opacity=".13"')
    if m:s+=rect(0,48,390,796,C['ink'],0,extra='opacity=".06"')
    x,y,w,h=(0,432,390,412) if m else (400,200,400,400)
    s+=ns['sheet_surface'](x,y,w,h,m)
    if m:s+=rect(179,y+12,32,4,C['border'],2)
    px=x+(20 if m else 24);py=y+(40 if m else 24);inner=w-(40 if m else 48)
    s+=text(c['title'],px,py,20,C['ink'],600)+text('From MacBook Pro' if c.get('receive') else 'To MacBook Pro',px,py+36,15,C['muted'])
    s+=ns['fileicon']('video',px,py+92,40)+text('Design source.fig + 2 more' if c.get('batch') else 'Product demo.mp4',px+56,py+100,15,C['ink'])
    s+=text(c['value'],px,py+160,24,C['ink'],600)+text(c['bytes'],px+inner,py+172,13,C['muted'],400,'end')
    s+=text(c['detail'],px,py+208,13,C['muted'])
    by=py+272;controls=[]
    if kind=='confirming':controls=[dict(label='Close',x=px,y=by,width=inner,height=48,mode='text',primary=False)]
    else:
        bw=(inner-12)/2
        controls=[dict(label='Cancel transfer',x=px,y=by,width=bw,height=48,mode='text',primary=False),dict(label='Resume' if kind=='paused' else 'Pause',x=px+bw+12,y=by,width=bw,height=48,mode='text',primary=True)]
    if m:s+=rect(0,0,390,48,C['white'])+ns['systembars']()
    return s,controls

manifest=[]
for dev,m,w,h in [('desktop',False,1200,800),('mobile',True,390,844)]:
    for i,kind in enumerate(CASES):
        body,controls=screen(kind,m);name=f'{dev}-{i+1:02}-{kind}'
        (OUT/(name+'.svg')).write_text(svg(body,w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',width=w,height=h,kind=kind,device=dev,controls=controls))
    for i,kind in enumerate(['sending','paused','confirming','unknown']):
        body,controls=screen(kind,m,True);name=f'{dev}-detail-{kind}'
        (OUT/(name+'.svg')).write_text(svg(body,w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',width=w,height=h,kind=kind,device=dev,details=True,controls=controls))

# Small action specimens keep actual 48px hit areas and tooltip/accessibility names.
ICONS=icon.__globals__['ICONS'];ICONS['refresh']='M20 5v6h-6 M20 11a8 8 0 1 0-1 6'
def action(c):
    if c.get('mode')=='text':return ns['button'](c['label'],c['x'],c['y'],c['primary'],width=c['width'])[0]
    return group('Action-'+c['label'],rect(c['x'],c['y'],48,48,C['white'],24)+icon(c['icon'],c['x']+12,c['y']+12,24,C['ink'],1.8))
for dev,cols in [('desktop',2),('mobile',4)]:
    items=[a for a in manifest if a['device']==dev];iw,ih=items[0]['width'],items[0]['height'];pad,gap,top=48,48,148;rows=math.ceil(len(items)/cols)
    W=pad*2+cols*iw+(cols-1)*gap;H=top+rows*(ih+88)+pad
    s=rect(0,0,W,H,C['surface'])+logo(48,40,32)+text('pinelink',92,40,24,C['ink'],600)+text('Real transfer progress / '+dev,48,88,16,C['muted'])
    for i,a in enumerate(items):
        x,y=pad+(i%cols)*(iw+gap),top+(i//cols)*(ih+88)
        src=(OUT/a['file']).read_text(encoding='utf-8');inner=src[src.index('>')+1:src.rindex('</svg>')]+''.join(action(c) for c in a['controls'])
        s+=text(a['kind'].capitalize(),x,y-36,16,C['ink'],600)+group(a['name'],inner,f'transform="translate({x} {y})"')+rect(x,y,iw,ih,'none',0,C['line'])
    (OUT/(dev+'-board.svg')).write_text(svg(s,W,H,dev+' progress overview'),encoding='utf-8')

s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Progress is data. Activity is motion.',48,104,24,C['ink'],600)+rect(48,160,1344,1,C['line'])
rules=[
 ('Visible hierarchy','Main: two nodes and one 3px curve. Blue = acknowledged bytes; gray = remaining bytes.'),
 ('Details on demand','Click curve / either node. Desktop modal / mobile bottom sheet. All numbers and controls live here.'),
 ('Known total','floor(acknowledged_bytes / total_bytes × 100). Byte counts use one consistent basis.'),
 ('Unknown / empty','Unknown total has no percentage or ETA. Empty known file waits for verification; never divide by zero.'),
 ('Actual completion','100% means bytes acknowledged. Receipt / verified completion unlocks Delivered or Received.'),
 ('Cloud completion','Sealed upload means Stored in cloud. It never implies recipient delivery.'),
 ('Time estimate','Use a 3–8s active sample window; show only after stable throughput. Hide on pause / stale data.'),
 ('Out of order','Snapshot has state_version + progress_revision + sampled_at. Reject stale progress and terminal regressions.'),
 ('Pause / reconnect','Freeze byte progress. Remove throughput / ETA. Resume keeps the same task and starts new samples.'),
 ('Multiple files','Sum acknowledged bytes / total bytes. Do not average per-file percentages. Retry is a new attempt.'),
 ('Motion','Blue extent follows bytes ≤180ms. Soft 1600ms light pass stays clipped inside the blue arc.'),
 ('Accessibility','Progressbar gets actual value. State / 10% milestones announced; no per-frame announcements.'),
]
for i,(a,b) in enumerate(rules):
    y=200+i*64;s+=text(a,48,y,15,C['ink'],600)+text(b,272,y,15,C['muted'])+rect(48,y+44,1344,1,C['line'])
s+=text('This page supersedes the activity-only transfer screens in v14 / v15.',48,1000,15,C['ink'],600)
(OUT/'11-progress-contract.svg').write_text(svg(s,1440,1072,'Progress contract'),encoding='utf-8')
motion=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('Active / paused / reduced motion',48,104,24,C['ink'],600)+text('All three share the same 62.5% acknowledged bytes. Only the light pass changes.',48,148,15,C['muted'])
for i,label in enumerate(['Sending','Paused','Reduced motion']):
    cx=240+i*464;points=[(cx,512),(cx-136,424),(cx-136,368),(cx,296)]
    motion+=text(label,cx,216,16,C['ink'],600,'middle')+path(f'M{cx} 512 C{cx-136} 424 {cx-136} 368 {cx} 296',C['line'],3)+path(progress_curve(points,.625),C['blue'],3)
    if i==0:motion+=path(progress_curve(points,.42,.36),C['sheen'],3)
    motion+=node('laptop','MacBook Pro',cx,272,48,status=None)+node('laptop','This device',cx,544,64,own=True,status=None)
motion+=rect(48,632,1344,1,C['line'])+text('1600ms linear pass · 3px width · no moving progress boundary · light stays inside acknowledged arc',48,664,15,C['muted'])+text('Pause: fade light 80ms. Resume: fade in 120ms. Confirming / hidden / reduced motion: no light pass.',48,704,15,C['muted'])
(OUT/'11-active-paused-motion.svg').write_text(svg(motion,1440,768,'Bounded activity motion'),encoding='utf-8')
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
(OUT/'icon-paths.json').write_text(json.dumps({k:ICONS[k] for k in ['pause','play','history','refresh','more']}),encoding='utf-8')
(OUT/'progress-motion.json').write_text(json.dumps(dict(schema_version=3,arc_ms=180,number_interpolation=False,easing='cubic-bezier(.16,1,.3,1)',progress_loop=False,activity_sheen=dict(duration_ms=1600,color=C['sheen'],width=3,max_path_fraction=.06,clip='acknowledged arc only',pause_fade_ms=80,resume_fade_ms=120),curve_width=3,curve_hit_width=24,report_interval_ms=500,remote_snapshot_ms=1000,stale_after_ms=3000,estimate_min_window_ms=3000,estimate_max_window_ms=8000,reduce_motion=dict(arc_ms=0,activity=False),byte_basis='acknowledged ciphertext bytes in v1; plaintext size remains encrypted'),indent=2),encoding='utf-8')
print(f'Generated {len(manifest)} progress screens and a contract board.')
