"""Direct and relay share delivery semantics; cloud storage remains an independent fact."""
from pathlib import Path
import runpy,json

v=runpy.run_path(str(Path(__file__).with_name('build-18-semantic-spacing.py')))
C=v['C']; rect=v['rect']; path=v['path']; group=v['group']; svg=v['svg']; txt=v['txt']; icon=v['icon']; node=v['node']; badge=v['badge']; curve=v['curve']; fileicon=v['fileicon']
OUT=Path(__file__).parent/'20-direct-relay';OUT.mkdir(exist_ok=True)

def graph(kind,m=False):
    state='reconnecting' if kind=='switching' else 'delivered' if kind=='delivered-copy-unavailable' else 'sending'
    return v['transfer'](state,m)

def details(kind,m=False):
    delivered=kind=='delivered-copy-unavailable'; direct=kind=='direct-details'
    pane,x,y,w,px,py,inner=v['surface'](m,416 if m else 376)
    pane+=group('Header / peer',txt('MacBook Pro',px,py,20,C['ink'],600)+badge('check' if delivered else 'send',px+inner-8,py+14,'Delivered' if delivered else 'Sending'))
    pane+=group('File identity',fileicon('video',px,py+52,40)+txt('Product demo.mp4',px+52,py+62))
    pane+=group('Delivery / recipient acknowledged bytes',txt('100%' if delivered else '62%',px,py+116,24,C['ink'],600)+txt('512 MB of 512 MB' if delivered else '320 MB of 512 MB',px+inner,py+128,13,C['muted'],400,'end')+txt('Delivered' if delivered else 'Sending',px,py+152,13,C['muted']))
    pane+=group('Connection / current route',txt('Connection',px,py+196,13,C['muted'])+txt('Direct' if direct else 'Via relay',px+120,py+196))
    pane+=group('Cloud copy / independent storage fact',txt('Cloud copy',px,py+228,13,C['muted'])+txt('Not kept' if direct else 'Unavailable' if delivered else 'Uploading · 88%',px+120,py+228))
    cc=v['controls'](px,py+280,inner,['Close'] if delivered else ['Cancel transfer','Pause'])
    return v['end'](v['backdrop'](graph(kind,m),m)+pane,m),cc

CASES=[
 ('direct-running','Direct / recipient progress','Two peer nodes. Blue = recipient acknowledgement; no cloud-upload stage.'),
 ('direct-details','Direct / delivery details','Direct path, no retained cloud copy. Completion still requires verification and receipt.'),
 ('relay-running','Relay live / recipient progress','Same peer graph and delivery metric as Direct. Relay is a route, not the recipient.'),
 ('relay-details','Relay live / independent cloud copy','Delivery 62%; spool upload 88%. Never add, average or substitute these metrics.'),
 ('switching','Direct to relay / preserve delivery progress','Same attempt and receiver ledger. Keep 62%; stop sheen only during a real observation gap.'),
 ('delivered-copy-unavailable','Delivered / cloud copy unavailable','Delivery receipt stays successful even if the optional cloud copy fails.'),
]
manifest=[]
for i,(kind,label,note) in enumerate(CASES):
    for m in [False,True]:
        body,controls=details(kind,m) if kind.endswith('details') or kind=='delivered-copy-unavailable' else (graph(kind,m),[])
        width,height=(390,844) if m else (1200,800)
        name=('Mobile' if m else 'Desktop')+' / '+label;file=f'18-{"mobile" if m else "desktop"}-{kind}.svg'
        (OUT/file).write_text(svg(body,width,height,name),encoding='utf-8')
        manifest.append(dict(page='flow',name=name,file=file,x=(i%3)*1680+(1264 if m else 0),y=(i//3)*1000,width=width,height=height,controls=controls,note=note))

def board(title,subtitle):
    return v['logo'](48,40,36)+txt('pinelink',100,40,32,C['ink'],600)+txt(title,48,112,24,C['ink'],600)+txt(subtitle,48,156,15,C['muted'])

s=board('Two routes / one delivery contract','Direct and live relay show peers. Cloud nodes represent an actual storage destination or source.')
for i,(title,source,target,metric,end,sub) in enumerate([
 ('Direct','This device','MacBook Pro','Delivery · 62%','100% → verify → receipt','No server copy required.'),
 ('Relay / live','This device','MacBook Pro','Delivery · 62%','100% → verify → receipt','Spool · 88% stays in details.'),
 ('Cloud / upload','This device','Cloud','Upload · 62%','100% → seal → Stored','Stored is not Delivered.'),
 ('Cloud / receive','This device','Cloud','Delivery · 62%','100% → verify → receipt','Cloud is the actual source.'),
]):
    x=48+i*336;cx=x+144;top,bottom=304,552;pts=[(cx,bottom-48),(cx-144,464),(cx-144,400),(cx,top+32)]
    if i==3:pts=list(reversed(pts))
    s+=txt(title,x,224,20,C['ink'],600)
    s+=path(curve(pts,1),C['line'],3)+path(curve(pts,.625),C['blue'],3)+path(curve(pts,.36,.30),C['sheen'],3)
    s+=node('cloud' if target=='Cloud' else 'laptop',target,cx,top,64,status=None)+node('laptop',source,cx,bottom,96,own=True,status=None)
    s+=txt(metric,x,664,20,C['ink'],600)+txt(end,x,708,15)+txt(sub,x,744,13,C['muted'])
    if i<3:s+=rect(x+312,224,1,556,C['line'])
s+=txt('A route change never changes the delivery metric. A phase change may change the metric and must replace its identity.',48,824,15,C['ink'],600)
s+=txt('Existing cloud preupload screens apply to the cloud phase only. Live relay does not automatically become a Cloud node.',48,860,15,C['muted'])
(OUT/'19-route-semantics.svg').write_text(svg(s,1440,928,'Routes and progress meaning'),encoding='utf-8')
manifest.append(dict(page='contract',name='Contract / route and cloud semantics',file='19-route-semantics.svg',x=0,y=0,width=1440,height=928,controls=[]))

s=board('Path continuity / storage does not decide delivery','Backend projection and motion contract / proposal only, not implemented')
rows=[
 ('Separate facts','delivery_progress · spool_progress · active_route','Recipient ACKs, durable cloud ACKs and transport telemetry have different authorities.'),
 ('Direct ↔ relay','Same transfer / attempt / receiver checkpoint','Deduplicate by content + chunk identity. Route epoch orders route telemetry, not byte totals.'),
 ('Do not reset','Keep the blue boundary and the peer identities','During a real gap, fade sheen in 80ms. Resume in 120ms after fresh progress.'),
 ('Cloud phase → peer','Change display_metric_id and the destination identity','Exit old path in 120ms; replace node in 200ms; enter new leg in 180ms.'),
 ('Independent outcomes','Delivered may succeed while cloud sealing fails','Preserve the receipt. Mark only the cloud copy unavailable in details/history.'),
 ('One receipt rule','Direct / relay / cloud pull all verify and commit locally','100% acknowledged bytes is not the delivery receipt. Failed verification never gets a check.'),
]
for i,(title,a,b) in enumerate(rows):
    yy=224+i*124
    s+=txt(title,48,yy,20,C['ink'],600)+txt(a,392,yy,15)+txt(b,392,yy+36,15,C['muted'])+rect(48,yy+92,1344,1,C['line'])
s+=txt('Reduced motion: immediate identity and state changes, no sheen or travel. Route switching never extends retention.',48,1016,15,C['ink'],600)
(OUT/'19-route-continuity.svg').write_text(svg(s,1440,1080,'Route continuity and independent completion'),encoding='utf-8')
manifest.append(dict(page='contract',name='Contract / route handoff and completion',file='19-route-continuity.svg',x=1536,y=0,width=1440,height=1080,controls=[]))
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
(OUT/'import.json').write_text(json.dumps([dict(a,svg=(OUT/a['file']).read_text(encoding='utf-8')) for a in manifest],ensure_ascii=False),encoding='utf-8')
print('Built',len(manifest),'direct / relay proposal frames.')
