from pathlib import Path
import runpy,json,math

v=runpy.run_path(str(Path(__file__).with_name('build-16-progress.py')))
n=v['ns'];C=v['C'];p=v['p'];rect=v['rect'];circle=v['circle'];path=v['path'];text=v['text'];group=v['group'];svg=v['svg'];node=v['node'];header=v['header'];icon=v['icon'];logo=v['logo'];curve=v['progress_curve'];fileicon=n['fileicon']
OUT=Path(__file__).parent/'17-history-cloud';OUT.mkdir(exist_ok=True)
FILES=[
 ('doc','Design source.fig','Reusable','cloud'),('folder','Photos','Reusable','local'),('video','Product demo.mp4','One-time',None),
 ('archive','Fonts.zip','Expired','cloud'),('folder','Project files','Reusable','cloud'),('doc','Contract.pdf','One-time',None),
 ('doc','Presentation.key','Reusable','local'),('folder','Offline backup','Expired','cloud'),('doc','Iteration notes.pdf','Reusable','cloud')]

def history(m=False):
    s=header(m,'records') if m else header(False,'focused')+p['graph'](False,.13,omit='MacBook Pro')+node('laptop','MacBook Pro',312,312,96,selected=True)
    if m:x,y,w=20,136,350;s+=text('MacBook Pro',x,y,20,C['ink'],600);tabs=184;top=264
    else:
        x,y,w=536,152,480;s+=rect(508,132,536,552,C['ink'],28,extra='opacity=".025"')+rect(512,128,528,552,C['white'],24,C['line'])
        s+=text('Transfer history',x,y,20,C['ink'],600)+text('MacBook Pro',x,y+36,13,C['muted'])+n['iconbutton']('close',968,144);tabs=208;top=280
    widths=[40,92,92,80];xx=x
    for label,ww in zip(['All','Reusable','One-time','Expired'],widths):
        s+=text(label,xx+ww/2,tabs+12,13,C['ink'] if label=='All' else C['muted'],600 if label=='All' else 400,'middle')
        if label=='All':s+=rect(xx+8,tabs+42,ww-16,2,C['ink'],1)
        xx+=ww+8
    gap=16 if m else 24;cw=(w-2*gap)/3
    for i,(kind,name,state,source) in enumerate(FILES):
        cx=x+(i%3)*(cw+gap)+cw/2;yy=top+(i//3)*128
        tile=group('File preview',fileicon(kind,cx-24,yy,48),'opacity=".45"' if state=='Expired' else '')
        if source=='cloud':tile+=circle(cx+20,yy+43,10,C['white'])+icon('cloud',cx+12,yy+35,16,C['muted'])
        if state=='One-time':tile+=circle(cx+20,yy+43,10,C['white'])+circle(cx+20,yy+43,8,C['ink'])+text('1',cx+20,yy+34,12,C['white'],600,'middle')
        display=name
        if m and len(display)>14:display=display[:12]+'…'
        tile+=text(display,cx,yy+60,13 if m else 15,C['muted'] if state=='Expired' else C['ink'],400,'middle')
        tile+=text(state,cx,yy+88,13,C['muted'],400,'middle')
        s+=group('History item / '+name+' / '+state,tile)
    return s

def record(kind,m=False):
    s=group('history backdrop',history(m),'opacity=".13"')
    if m:s+=rect(0,48,390,796,C['ink'],0,extra='opacity=".06"')
    x,y,w,h=(0,432,390,412) if m else (400,200,400,400)
    s+=n['sheet_surface'](x,y,w,h,m)
    if m:s+=rect(179,y+12,32,4,C['border'],2)
    px=x+(20 if m else 24);py=y+(40 if m else 24);inner=w-(40 if m else 48)
    title,sub,file,detail,primary={
      'reusable':('Design source.fig','Reusable','Design source.fig','Available until Nov 2, 16:00','Restore'),
      'one-time':('One-time transfer','No reusable copy','Product demo.mp4','Choose the file to send again.','Choose files'),
      'expired':('Copy expired','Transfer record kept','Fonts.zip','Choose the file to send again.','Choose files'),
      'keep':('Keep available','Design source.fig','Design source.fig','Keep this copy beyond its current expiry.','Keep available'),
    }[kind]
    s+=text(title,px,py,20,C['ink'],600)+text(sub,px,py+36,15,C['muted'])
    if kind=='reusable':s+=n['iconbutton']('more',x+w-68,py-8)
    s+=fileicon('archive' if kind=='expired' else 'video' if kind=='one-time' else 'doc',px,py+92,40)
    # The filename is already the title in the reusable case.
    s+=text('Cloud copy' if kind=='reusable' else file,px+56,py+100,15,C['ink'])+text(detail,px,py+176,13,C['muted'])
    by=py+272;bw=(inner-12)/2
    controls=[dict(label='Close' if kind!='keep' else 'Cancel',x=px,y=by,width=bw,primary=False),dict(label=primary,x=px+bw+12,y=by,width=bw,primary=True)]
    if m:s+=rect(0,0,390,48,C['white'])+n['systembars']()
    return s,controls

def restore(kind,m=False):
    ox,oy,tx,ty,sz=(195,680,195,248,80) if m else (504,648,696,248,96);bend=56 if m else 376
    points=[(tx,ty+(28 if m else 32)),(bend,ty+176),(bend,oy-176),(ox,oy-sz/2)]
    s=header(m,'focused');pct=1 if kind in ['verifying','restored'] else .625
    if kind!='restored':
        d=f'M{points[0][0]} {points[0][1]} C'+' '.join(str(q) for pt in points[1:] for q in pt)
        s+=path(d,C['line'],3)+path(curve(points,pct),C['blue'],3)
        if kind=='restoring':s+=path(curve(points,.42,.36),C['sheen'],3)
    s+=node('cloud','Cloud',tx,ty,56 if m else 64,status=None)+node('phone' if m else 'laptop','This device',ox,oy,sz,own=True,status=None)
    if kind=='restored':s+=circle(ox+28,oy+28,14,C['white'])+circle(ox+28,oy+28,11,C['ink'])+icon('check',ox+21,oy+21,14,C['white'],2)
    label={'restoring':'','paused':'Paused','verifying':'Verifying file','restored':'Restored'}[kind]
    if label:s+=text(label,156 if m else 744,416,15,C['muted'])
    return s,[]

manifest=[]
for dev,m,w,h in [('desktop',False,1200,800),('mobile',True,390,844)]:
    for kind in ['history','reusable','one-time','expired','keep','restoring','paused','verifying','restored']:
        body,controls=(history(m),[]) if kind=='history' else record(kind,m) if kind in ['reusable','one-time','expired','keep'] else restore(kind,m)
        name=dev+'-'+kind;(OUT/(name+'.svg')).write_text(svg(body,w,h,name),encoding='utf-8')
        manifest.append(dict(name=name,file=name+'.svg',device=dev,kind=kind,width=w,height=h,controls=controls,page='history' if kind in ['history','reusable','one-time','expired','keep'] else 'restore'))

s=logo(48,40,36)+text('pinelink',100,40,32,C['ink'],600)+text('History survives. Availability changes.',48,104,24,C['ink'],600)+rect(48,160,1344,1,C['line'])
rows=[
 ('Reusable','An authorized cloud copy or verified local bookmark exists. Restore / Send again follows capabilities.'),
 ('One-time','This transfer has no retained reusable copy. Keep its record; Choose files starts a new transfer.'),
 ('Expired','The retained copy passed expiry. Keep the record; Restore / Keep available is unavailable.'),
 ('Cloud source','A logical source for this restore, not a peer device or presence claim. Show only for an authorized copy.'),
 ('Retention','Availability and retention policy are independent. A temporary copy can be reusable until expiry.'),
 ('Keep available','Explicit promotion before expiry, with ownership / quota / entitlement checks. No invented pricing.'),
 ('Restore','Create a fresh PullSession. Cloud → this device uses the accepted blue arc / bounded light motion.'),
 ('Verification','Restored requires local decrypt / integrity checks and commit. 100% alone is not success.'),
 ('History visibility','A past receipt is not permission to fetch again. Re-check grants, copy state and encrypted envelope.'),
 ('Destructive actions','Remove record and Delete cloud copy are separate operations with explicit wording.'),
]
for i,(a,b) in enumerate(rows):
    yy=208+i*64;s+=text(a,48,yy,15,C['ink'],600)+text(b,264,yy,15,C['muted'])+rect(48,yy+44,1344,1,C['line'])
s+=text('Commercial entitlements decide capabilities; never turn a missing or expired copy into a purchasable restore.',48,904,15,C['ink'],600)
(OUT/'14-history-contract.svg').write_text(svg(s,1440,976,'History and retention contract'),encoding='utf-8')
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
print('Generated 18 history / cloud restore screens and the availability contract.')
