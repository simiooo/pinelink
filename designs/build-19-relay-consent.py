"""Additive relay-consent proposal. Uses the accepted v18 semantic layout helpers."""
from pathlib import Path
from html import escape
import runpy, json

v = runpy.run_path(str(Path(__file__).with_name('build-18-semantic-spacing.py')))
C=v['C']; p=v['p']; ns=v['ns']
rect=v['rect']; path=v['path']; group=v['group']; svg=v['svg']; txt=v['txt']; icon=v['icon']; node=v['node']; badge=v['badge']; curve=v['curve']; fileicon=v['fileicon']
OUT=Path(__file__).parent/'19-relay-consent'; OUT.mkdir(exist_ok=True)

def graph(kind, m=False):
    ox,oy,tx,ty,size=(195,680,195,248,80) if m else (504,648,696,248,96)
    points=[(ox,oy-size/2),(56 if m else 376,oy-176),(56 if m else 376,ty+176),(tx,ty+(28 if m else 32))]
    receive=kind in ['receiving','received','request']
    if receive: points=list(reversed(points))
    s=v['header'](m)
    active=kind in ['uploading','receiving','details']
    if active:
        s+=path(curve(points,1),C['line'],3)+path(curve(points,.625),C['blue'],3)
        s+=path(curve(points,.36,.30),C['sheen'],3)
    s+=node('cloud','Cloud',tx,ty,56 if m else 64,status=None)
    s+=node('phone' if m else 'laptop','This device',ox,oy,size,own=True,status=None)
    b=('clock','Awaiting MacBook Pro') if kind=='staged' else ('lock','Accept to receive') if kind=='request' else ('check','Received') if kind=='received' else None
    if b:
        bx,by=(ox+size*.36,oy+size*.36) if kind=='received' else (tx+22,ty+22)
        s+=badge(b[0],bx,by,b[1])
    return s

def panel(kind,m=False):
    if kind=='details':
        h=340 if m else 300
        pane,x,y,w,px,py,inner=v['surface'](m,h)
        pane+=group('Header / destination',txt('MacBook Pro',px,py,20,C['ink'],600)+badge('cloud',px+inner-8,py+14,'Uploading to cloud'))
        pane+=group('File identity',fileicon('video',px,py+52,40)+txt('Product demo.mp4',px+52,py+62))
        pane+=group('Upload leg / acknowledged bytes',txt('62%',px,py+116,24,C['ink'],600)+txt('320 MB of 512 MB',px+inner,py+128,13,C['muted'],400,'end')+txt('Uploading to cloud',px,py+152,13,C['muted']))
        return pane,v['controls'](px,py+204,inner,['Cancel transfer','Pause'])
    if kind=='request':
        head=28; body=48; by_offset=head+24+body+32; h=by_offset+48+(88 if m else 48)
        pane,x,y,w,px,py,inner=v['surface'](m,h)
        pane+=group('Header / sender identity',txt('MacBook Pro',px,py,20,C['ink'],600))
        pane+=group('File identity / metadata only',fileicon('video',px,py+52,48)+badge('lock',px+40,py+92,'Accept to receive')+txt('Product demo.mp4',px+60,py+66))
        return pane,v['controls'](px,py+by_offset,inner,['Decline','Accept'])
    title,reason,mark=('Request declined','Send a new request to continue.','ban') if kind=='declined' else ('Request expired','Send a new request to continue.','clock')
    h=56+32+48+(88 if m else 48)
    pane,x,y,w,px,py,inner=v['surface'](m,h)
    pane+=group('Header / required reason',icon(mark,px,py+2,24,C['ink'])+txt(title,px+36,py,20,C['ink'],600)+txt(reason,px+36,py+36,15,C['muted']))
    return pane,v['controls'](px,py+88,inner,['Close','Send again'])

SCENARIOS=[
 ('uploading','Sender / uploading before consent','Pending recipient; durable uplink acknowledgements drive the blue prefix.'),
 ('staged','Sender / stored, awaiting recipient','Upload sealed. Clock badge: waiting. No sheen and no delivered receipt.'),
 ('request','Recipient / request during upload','Accept and Decline remain available. Metadata only; no byte or key access.'),
 ('receiving','Recipient / accepted, receiving','Cloud is the source. A new downlink progress leg starts from its own byte count.'),
 ('details','Sender / upload details','Per-leg metrics live in the secondary surface, not on the main graph.'),
 ('declined','Sender / declined','Revoke immediately. Keep history. Release this delivery reference and queue GC.'),
 ('expired','Sender / expired','Same cleanup path. Existing request and content expiry clocks stay unchanged.'),
 ('received','Recipient / received','Only after decryption, integrity verification, local commit and delivery receipt.'),
]
manifest=[]
for i,(kind,label,note) in enumerate(SCENARIOS):
    for m in [False,True]:
        body=graph(kind,m); controls=[]
        if kind in ['request','details','declined','expired']:
            pane,controls=panel(kind,m)
            body=v['end'](v['backdrop'](body,m)+pane,m)
        width,height=(390,844) if m else (1200,800)
        name=('Mobile' if m else 'Desktop')+' / '+label
        file=f'16-{"mobile" if m else "desktop"}-{kind}.svg'
        (OUT/file).write_text(svg(body,width,height,name),encoding='utf-8')
        manifest.append(dict(page='flow',name=name,file=file,x=(i%4)*1680+(1264 if m else 0),y=(i//4)*1000,width=width,height=height,controls=controls,note=note))

def board(title,subtitle):
    return v['logo'](48,40,36)+txt('pinelink',100,40,32,C['ink'],600)+txt(title,48,112,24,C['ink'],600)+txt(subtitle,48,156,15,C['muted'])

s=board('Relay / consent is independent from upload','Product states and motion / additive proposal, not an implemented backend contract')
rows=[
 ('01','Upload before consent','This device → Cloud','Blue = durable uplink bytes. Sheen stays inside that prefix.', 'Sender may upload while the recipient decision is PENDING.'),
 ('02','Stored, awaiting recipient','Clock badge on Cloud','Seal acknowledged → sheen fades in 80ms; curve exits in 180ms.', 'Stored is not Delivered. Sender can go offline after durable seal.'),
 ('03','Accept and receive','Cloud → This device','Consent commits → lock exits in 120ms; downlink enters in 180ms.', 'An accepted recipient may receive available chunks before full seal.'),
 ('04','Decline or expire','Access removed immediately','Sheen stops; curve exits in 120ms; history remains.', 'Release this share only. Physical deletion waits for zero valid references.'),
 ('05','Receive and verify','Receipt before completion','Full bytes → static verifying badge → receipt → quiet exit.', 'No automatic 100% completion before integrity and local commit.'),
]
for i,(n,title,axis,motion,truth) in enumerate(rows):
    y=224+i*148
    s+=txt(n,48,y,24,C['blue'],600)+txt(title,144,y,20,C['ink'],600)+txt(axis,144,y+36,15,C['muted'])
    s+=txt(motion,568,y,15,C['ink'])+txt(truth,568,y+36,15,C['muted'])+rect(48,y+112,1344,1,C['line'])
s+=txt('Reduced motion: immediate state changes; no sheen, travel or scale animation. No fake byte progress.',48,988,15,C['ink'],600)
(OUT/'17-relay-motion.svg').write_text(svg(s,1440,1056,'Relay consent and motion contract'),encoding='utf-8')
manifest.append(dict(page='contract',name='Relay / state transitions and motion',file='17-relay-motion.svg',x=0,y=0,width=1440,height=1056,controls=[]))

s=board('Reference-safe deletion / one atomic authority','Database transaction revokes access; a fenced worker deletes immutable object generations.')
rows=[
 ('References','Delivery staging · private library · explicit persistent keep','Reference ledger is authoritative. A cached ref_count is never the sole proof.'),
 ('Terminal transaction','Transfer state + revoke grant + release reference + outbox','Idempotency key prevents double release. Delivery-owned FileEntry is released too.'),
 ('Serialize decisions','Lock object generation → transfer → references and leases','Attach, Keep, release and GC use the same parent lock and a global lock order.'),
 ('GC eligibility','No live references, retention holds or valid I/O leases','CAS to DELETING with a generation and delete token. New attaches must fail.'),
 ('Physical deletion','Retry a sealed key manifest; finalize only the matching token','Immutable generation-specific keys isolate stale GC and late upload writers.'),
 ('Races and recovery','Reject vs seal · Keep vs GC · duplicate or delayed workers','No resurrection. Orphan sweep handles a late successful object PUT.'),
]
for i,(title,a,b) in enumerate(rows):
    y=224+i*120
    s+=txt(title,48,y,20,C['ink'],600)+txt(a,440,y,15,C['ink'])+txt(b,440,y+36,15,C['muted'])+rect(48,y+88,1344,1,C['line'])
s+=txt('Existing expiry clocks stay unchanged. Rejection removes this share, not another valid private or shared copy.',48,992,15,C['ink'],600)
(OUT/'17-atomic-lifecycle.svg').write_text(svg(s,1440,1056,'Atomic references and garbage collection'),encoding='utf-8')
manifest.append(dict(page='contract',name='Architecture / atomic references and GC',file='17-atomic-lifecycle.svg',x=1536,y=0,width=1440,height=1056,controls=[]))
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
(OUT/'import.json').write_text(json.dumps([dict(a,svg=(OUT/a['file']).read_text(encoding='utf-8')) for a in manifest],ensure_ascii=False),encoding='utf-8')
print('Built',len(manifest),'editable proposal frames; baseline v18 preserved.')
