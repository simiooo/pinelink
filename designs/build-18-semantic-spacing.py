from pathlib import Path
from html import escape
from PIL import ImageFont
import runpy,json,math

v=runpy.run_path(str(Path(__file__).with_name('build-17-history-cloud.py')))
ns=v['n'];p=v['p'];C=v['C'];rect=v['rect'];circle=v['circle'];path=v['path'];group=v['group'];svg=v['svg'];icon=v['icon'];fileicon=v['fileicon'];curve=v['curve'];logo=v['logo']
OUT=Path(__file__).parent/'18-semantic-spacing';OUT.mkdir(exist_ok=True)
FONTS={w:ImageFont.truetype('C:/Windows/Fonts/'+('seguisb.ttf' if w==600 else 'segoeui.ttf'),15) for w in [400,600]}
ICONS=icon.__globals__['ICONS']
ICONS.update(clock='M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M12 7v5l3 2',repeat='M20 7H7a4 4 0 0 0-4 4 M17 4l3 3-3 3 M4 17h13a4 4 0 0 0 4-4 M7 20l-3-3 3-3',shield='M12 3l8 3v6c0 5-8 9-8 9s-8-4-8-9V6z',ban='M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M6 6l12 12',lock='M6 11h12v10H6z M8 11V7a4 4 0 0 1 8 0v4',one='M9 8l3-2v12 M8 18h8',refresh='M20 5v6h-6 M20 11a8 8 0 1 0-1 6')
def txt(s,x,y,size=15,color=None,weight=400,anchor='start'):
    s=p['tr'](s);lh={13:20,15:20,16:24,20:28,24:32,32:40}.get(size,20)
    return f'<text x="{x}" y="{y+(lh-size)/2+size*.81}" font-family="Segoe UI" font-size="{size}" font-weight="{weight}" fill="{color or C["ink"]}" text-anchor="{anchor}">{escape(s)}</text>'
def fit(s,width,size=15,weight=400):
    f=ImageFont.truetype('C:/Windows/Fonts/'+('seguisb.ttf' if weight==600 else 'segoeui.ttf'),size)
    if f.getlength(s)<=width:return s
    while s and f.getlength(s+'…')>width:s=s[:-1]
    return s.rstrip()+'…'
def badge(kind,x,y,label,size=16):
    return group('Status badge / '+label,circle(x,y,size/2,C['white'],C['border'],1)+icon(kind,x-size*.375,y-size*.375,size*.75,C['ink'],1.5))
def node(kind,label,x,y,size=64,selected=False,own=False,status='online',opacity=1):
    # Identity is one group: disc + 8px + name. Caption mask follows actual text width.
    label=p['tr'](label);r=size/2;s=circle(x,y,r,C['white'] if selected else C['surface'],C['blue'] if selected else None,2)+icon(kind,x-size/4,y-size/4,size/2,C['ink'],1.7)
    if status:
        bx,by=x+r*.72,y+r*.72
        if status=='cloud':s+=badge('cloud',bx,by,'Cloud copy')
        else:s+=circle(bx,by,7,C['white'])+circle(bx,by,4,C['white'] if status=='waiting' else C['faint'] if status=='offline' else C['ink'],C['muted'] if status=='waiting' else None,1.5)
    ly=y+r+8;f=FONTS[600 if own or selected else 400];ww=math.ceil((f.getlength(label)+16)/4)*4
    s+=rect(x-ww/2,ly-2,ww,24,C['white'])+txt(label,x,ly,15,C['ink'] if own or selected else C['muted'],600 if own or selected else 400,'middle')
    return group('Identity / '+label,s,f'opacity="{opacity}"')
p['node']=node
def header(m=False,mode='focused'):
    if mode!='history':return ns['header'](m,mode)
    return p['systembars']()+ns['iconbutton']('back',4,56)+txt('MacBook Pro',195,66,20,C['ink'],600,'middle')+ns['iconbutton']('more',338,56)
def default(m=False):return header(m,'normal')+p['graph'](m)
def selected(m=False):
    cx,cy=(195,352) if m else (600,328)
    return header(m)+p['graph'](m,.13,omit='MacBook Pro')+node('laptop','MacBook Pro',cx,cy,80 if m else 96,selected=True)+ns['iconbutton']('send',cx-136,cy-24,primary=True)+ns['iconbutton']('folder',cx-24,cy-136,soft=True)+ns['iconbutton']('more',cx+88,cy-24,soft=True)
FILES=v['FILES']
def history(m=False):
    if m:s=header(True,'history');x,y,w,tabs,top=16,66,358,128,208
    else:
        x,y,w,tabs,top=528,152,496,196,276
        s=header()+p['graph'](False,.13,omit='MacBook Pro')+node('laptop','MacBook Pro',312,312,96,selected=True)
        s+=rect(508,132,536,464,C['ink'],28,extra='opacity=".025"')+rect(512,128,528,464,C['white'],24,C['line'])
        s+=group('Panel / Header',txt('MacBook Pro',x,y,20,C['ink'],600)+ns['iconbutton']('close',988,142))
    xx=x
    for label in ['All','Reusable','One-time','Expired']:
        size=13 if m else 15;f=ImageFont.truetype('C:/Windows/Fonts/segoeui.ttf',size);tw=f.getlength(label);pad=8 if m else 12;ww=max(48,math.ceil((tw+pad*2)/4)*4)
        content=rect(xx,tabs,ww,48,C['white'],0,extra='opacity="0"')+txt(label,xx+ww/2,tabs+14,size,C['ink'] if label=='All' else C['muted'],600 if label=='All' else 400,'middle')
        if label=='All':content+=rect(xx+(ww-tw)/2,tabs+42,tw,2,C['ink'],1)
        s+=group('Tab / '+label,content);xx+=ww+(0 if m else 8)
    gap=16 if m else 24;cw=(w-gap*2)/3
    for i,(kind,name,state,source) in enumerate(FILES):
        colx=x+(i%3)*(cw+gap);yy=top+(i//3)*108
        bkind='clock' if state=='Expired' else 'one' if state=='One-time' else 'cloud' if source=='cloud' else 'repeat'
        label='Expired cloud copy' if state=='Expired' else 'One-time · no retained copy' if state=='One-time' else 'Reusable · '+('cloud copy' if source=='cloud' else 'on this device')
        tile=group('File image',fileicon(kind,colx,yy,48),'opacity=".45"' if state=='Expired' else '')+badge(bkind,colx+44,yy+40,label)
        tile+=txt(fit(name,cw-8,13 if m else 15),colx,yy+56,13 if m else 15,C['muted'] if state=='Expired' else C['ink'])
        s+=group('File identity / '+name,tile)
    return s

STATE_BADGES={'waiting':'clock','connecting':'clock','queued':'clock','verifying':'shield','confirming':'clock','stored':'check','cloud':'check','delivered':'check','received':'check','restored':'check','paused':'pause','reconnecting':'wifi','failed':'alert','cancelled':'ban','unknown':'download'}
def transfer(kind,m=False,restore=False):
    c=v['v']['CASES'].get(kind,{});ox,oy,tx,ty,sz=(195,680,195,248,80) if m else (504,648,696,248,96);bend=56 if m else 376
    receive=restore or kind in ['receiving','received'];points=[(ox,oy-sz/2),(bend,oy-176),(bend,ty+176),(tx,ty+(28 if m else 32))]
    if receive:points=list(reversed(points))
    fraction=1 if kind in ['verifying','confirming','delivered','received','stored','cloud','restored'] else 0 if kind in ['waiting','connecting','queued','cancelled'] else None if kind=='unknown' else .03125 if kind=='estimating' else .625
    active=kind in ['sending','receiving','estimating','batch','restoring'];done=kind in ['delivered','received','stored','cloud','restored'];cloud=kind in ['stored','cloud']
    s=header(m)
    if not done and kind!='cancelled':
        d=f'M{points[0][0]} {points[0][1]} C'+' '.join(str(q) for pt in points[1:] for q in pt)
        s+=path(d,C['line'],3,extra='stroke-dasharray="4 8"' if kind in ['waiting','connecting','queued','unknown'] else '')
        if fraction:s+=path(curve(points,fraction),C['blue'],3)
        if active and fraction:s+=path(curve(points,min(fraction,fraction*.42+.06),fraction*.42),C['sheen'],3)
    s+=node('cloud' if restore else 'nas' if cloud else 'laptop','Cloud' if restore else 'Cloud vault' if cloud else 'MacBook Pro',tx,ty,56 if m else 64,status=None)+node('phone' if m else 'laptop','This device',ox,oy,sz,own=True,status=None)
    if kind in STATE_BADGES:
        bx,by=(ox+sz*.36,oy+sz*.36) if receive or kind in ['paused','reconnecting','unknown'] else (tx+22,ty+22)
        s+=badge(STATE_BADGES[kind],bx,by,'Verifying file' if restore and kind=='verifying' else 'Restored' if restore and done else kind.capitalize())
    return s

def surface(m,h):
    x,y,w=(0,844-h,390) if m else (400,(800-h)/2,400);pad=16 if m else 20;py=y+(40 if m else 24)
    s=ns['sheet_surface'](x,y,w,h,m)
    if m:s+=rect(179,y+12,32,4,C['border'],2)
    return s,x,y,w,x+pad,py,w-pad*2
def controls(px,by,inner,labels,primary=True):
    if len(labels)==1:return [dict(label=labels[0],x=px,y=by,width=inner,primary=False)]
    bw=(inner-12)/2
    return [dict(label=t,x=px+i*(bw+12),y=by,width=bw,primary=bool(primary and i)) for i,t in enumerate(labels)]
def backdrop(s,m):return group('Inactive context',s,'opacity=".13"')+(rect(0,48,390,796,C['ink'],0,extra='opacity=".06"') if m else '')
def end(s,m):return s+(rect(0,0,390,48,C['white'])+p['systembars']() if m else '')
def panel(kind,m=False):
    cases={
      'receive':('Receive files?','From MacBook Pro',['Decline','Accept'],'download',True),
      'prepared':('Send files?','To MacBook Pro',['Cancel','Send'],'send',True),
      'async':('Save for later?','MacBook Pro is offline.',['Cancel','Store in cloud'],'cloud',True),
      'rejected':('Request declined','MacBook Pro declined this request.',['Close','Choose device'],'ban',False),
      'expired':('Request expired','Send a new request to continue.',['Close','Send again'],'clock',False),
      'failed':('Couldn’t send files','Check your connection and try again.',['Close','Try again'],'alert',False),
      'unavailable':('File unavailable','Choose the file again to continue.',['Close','Choose files'],'ban',False),
      'unreadable':('Couldn’t read the file','Choose the file again to continue.',['Close','Choose files'],'alert',False),
      'cancel-confirm':('Cancel transfer?','The current transfer will stop.',['Keep transfer','Cancel transfer'],'ban',False),
      'task-controls':('MacBook Pro','',['Cancel transfer','Pause'],'pause',False),
    }
    title,reason,labels,ico,hasfile=cases[kind];head_h=28+(8+20 if reason else 0);body_h=48 if hasfile else 0;body_gap=24 if hasfile else 0
    inside=head_h+body_gap+body_h+32+48;h=inside+(88 if m else 48);s,x,y,w,px,py,inner=surface(m,h)
    headerpart=icon(ico,px,py+2,24,C['ink'])+txt(title,px+36,py,20,C['ink'],600)
    if reason:headerpart+=txt(reason,px+36,py+36,15,C['muted'])
    s+=group('Panel / Header / title and reason',headerpart)
    if hasfile:
        fy=py+head_h+24
        s+=group('Panel / File identity',fileicon('doc',px,fy,40)+txt('Design source.fig',px+52,fy+2,15,C['ink'],600)+txt('and 2 more files',px+52,fy+26,13,C['muted']))
    by=py+head_h+body_gap+body_h+32
    return s,controls(px,by,inner,labels)
def dialog(kind,m=False):
    s=default(m) if kind in ['prepared','async','unreadable'] else transfer('sending',m)
    body,c=panel(kind,m);return end(backdrop(s,m)+body,m),c
def record(kind,m=False):
    filename='Product demo.mp4' if kind=='one-time' else 'Fonts.zip' if kind=='expired' else 'Design source.fig'
    if kind=='keep':
        head=56;body=48;h=head+24+body+32+48+(88 if m else 48);pane,x,y,w,px,py,inner=surface(m,h)
        pane+=group('Panel / Header',txt('Keep available?',px,py,20,C['ink'],600)+txt('Keep this copy beyond its current expiry.',px,py+36,15,C['muted']))
        pane+=group('Panel / File identity',fileicon('doc',px,py+80,48)+txt(filename,px+60,py+94,15))
        by=py+head+24+body+32;cc=controls(px,by,inner,['Cancel','Keep available'])
    else:
        info='Available until Nov 2, 16:00' if kind=='reusable' else 'Cloud copy expired. Choose the file again.' if kind=='expired' else 'No retained copy. Choose the file again.'
        h=48+24+20+32+48+(88 if m else 48);pane,x,y,w,px,py,inner=surface(m,h)
        pane+=group('Panel / File identity',fileicon('archive' if kind=='expired' else 'video' if kind=='one-time' else 'doc',px,py,48)+badge('cloud' if kind=='reusable' else 'clock' if kind=='expired' else 'one',px+40,py+40,kind)+txt(fit(filename,inner-120,20,600),px+60,py+10,20,C['ink'],600))
        if kind=='reusable':pane+=ns['iconbutton']('more',px+inner-36,py)
        pane+=group('Panel / Availability reason',txt(info,px,py+72,13,C['muted']))
        cc=controls(px,py+124,inner,['Close','Restore' if kind=='reusable' else 'Choose files'])
    return end(backdrop(history(m),m)+pane,m),cc
def details(kind,m=False):
    c=v['v']['CASES'][kind];rate=c['detail'];h=28+24+40+24+32+(4+20 if rate else 0)+32+48+(88 if m else 48)
    pane,x,y,w,px,py,inner=surface(m,h)
    pane+=group('Panel / Header',txt('MacBook Pro',px,py,20,C['ink'],600)+badge(STATE_BADGES.get(kind,'send'),px+inner-8,py+14,c['title']))
    pane+=group('Panel / File identity',fileicon('video',px,py+52,40)+txt('Product demo.mp4',px+52,py+62,15))
    pane+=group('Panel / Progress metrics',txt(c['value'],px,py+116,24,C['ink'],600)+txt(c['bytes'],px+inner,py+128,13,C['muted'],400,'end')+(txt(rate,px,py+152,13,C['muted']) if rate else ''))
    cc=controls(px,py+148+(24 if rate else 0)+32,inner,['Close'] if kind=='confirming' else ['Cancel transfer','Resume' if kind=='paused' else 'Pause'])
    return end(backdrop(transfer(kind,m),m)+pane,m),cc
def drag(kind,m=False):
    if kind in ['prepared','async','unreadable']:return dialog(kind,m)
    s=header(m)+p['graph'](m,1 if kind=='entered' else .28);tx,ty=(107,248) if m else (312,312)
    if kind=='entered':s+=txt('Drop on a device',195 if m else 600,128 if m else 112,15,C['ink'],600,'middle')
    if kind in ['near','target']:
        s+=circle(tx,ty,56,C['soft'],C['blue'],1.5,extra='stroke-dasharray="3 6"')+node('laptop','MacBook Pro',tx,ty,72,status=None)+badge('plus',tx+26,ty+26,'Drop to send')+txt('Drop to send',tx,ty+80,15,C['blue'],600,'middle')
        gx,gy=tx+(52 if kind=='near' else 12),ty+(40 if kind=='near' else 8)
        s+=group('System drag image',rect(gx,gy,176,48,C['white'],16,C['line'])+fileicon('doc',gx+8,gy+10,28)+txt('Design source.fig',gx+44,gy+14,13))
    if kind in ['offline','revoked','self']:
        if kind=='self':tx,ty=(195,680) if m else (600,648)
        size=(80 if m else 96) if kind=='self' else (56 if m else 64);label={'offline':'Device is offline','revoked':'Device unavailable','self':'Choose another device'}[kind]
        s+=circle(tx,ty,size/2+24,C['surface'],C['border'],1.5,extra='stroke-dasharray="3 6"')+node('laptop','This device' if kind=='self' else 'MacBook Pro',tx,ty,size,own=kind=='self',status=None)+badge('lock' if kind=='revoked' else 'ban',tx+26,ty+26,label)+txt(label,tx,ty-108 if kind=='self' else ty+80,15,C['ink'],600,'middle')
    return s,[]
def empty(m=False):
    cx,cy=(195,304) if m else (600,320);s=header(m,'empty')
    s+=group('Empty state / Message',icon('wifi',cx-20,cy-20,40,C['faint'])+txt('No devices nearby',cx,cy+44,20,C['ink'],600,'middle')+txt('Connect to the same network.',cx,cy+80,15,C['muted'],400,'middle'))
    s+=node('phone' if m else 'laptop','This device',195 if m else 600,680 if m else 648,80 if m else 96,own=True)
    return s,[dict(label='Add device',x=cx-76,y=cy+132,width=152,primary=True)]

def resolve(a):
    page=a['pageName'];name=a['name'];m=a['w']==390
    if page.startswith(('01','02')):
        key=name.split(' / ',1)[1]
        if key=='Devices':return default(m),[]
        if key=='Selected device':return selected(m),[]
        if key=='Transferring':return transfer('sending',m),[]
        if key=='Transfer history':return history(m),[]
        if key=='Receive request':return dialog('receive',m)
        if key=='Paused':return transfer('paused',m),[]
        if key=='Connection lost':return transfer('reconnecting',m),[]
        if key=='Empty state':return empty(m)
    if page.startswith(('05','06','07')):
        kind=name.split('-',2)[2]
        if page.startswith('06') or kind in ['prepared','async','unreadable']:return drag(kind,m)
        if kind in ['rejected','expired','failed','unavailable','cancel-confirm','task-controls']:return dialog(kind,m)
        return transfer('confirming' if kind=='verifying' else 'cloud' if kind=='stored' else kind,m),[]
    if page.startswith(('09','10')):
        if '-detail-' in name:return details(name.split('-detail-')[1],m)
        return transfer(name.split('-',2)[2],m),[]
    if page.startswith('12'):
        kind=name.split('-',1)[1]
        return (history(m),[]) if kind=='history' else record(kind,m)
    if page.startswith('13'):return transfer(name.split('-',1)[1],m,True),[]
    raise ValueError(a)

inventory=json.loads((OUT/'source-audit.json').read_text(encoding='utf-8'));manifest=[]
for i,a in enumerate(inventory):
    if not a['product']:continue
    body,cc=resolve(a);file=f'{a["pageName"][:2]}-{a["name"].replace(" / ","-").replace(" ","-")}.svg'
    (OUT/file).write_text(svg(body,a['w'],a['h'],a['name']),encoding='utf-8')
    manifest.append(dict(name=a['name'],file=file,pageName=a['pageName'],pageId=a['pageId'],oldId=a['id'],x=a['x'],y=a['y'],width=a['w'],height=a['h'],controls=cc))
(OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2),encoding='utf-8')
tokens={'space':{k:{'$type':'number','$value':val} for k,val in {'metadata':4,'identity':8,'inline':12,'siblings':16,'historyHeadingToTabs':16,'section':24,'actions':32,'historyInline':16,'dialogInline':20,'blockInset':24,'mobileInline':16,'sheetTop':40,'sheetBottom':48,'gridRow':32}.items()},'badge':{'size':{'$type':'number','$value':16},'glyph':{'$type':'number','$value':12}},'tab':{'height':{'$type':'number','$value':48},'desktopPadding':{'$type':'number','$value':12},'mobilePadding':{'$type':'number','$value':8},'desktopGap':{'$type':'number','$value':8},'mobileGap':{'$type':'number','$value':0}}}
(OUT/'semantic.tokens.json').write_text(json.dumps(tokens,indent=2),encoding='utf-8')
s=logo(48,40,36)+txt('pinelink',100,40,32,C['ink'],600)+txt('Semantic spacing / status economy',48,104,24,C['ink'],600)
rows=[('4','Metadata within one identity','Filename and supporting count; metric and rate.'),('8','Object and name','Disc to device name. File icon to filename. Title to required reason.'),('12','Inline identity','Icon and adjacent text in a horizontal file row.'),('24','Distinct content groups','Heading to file. File identity to metrics. History heading / tabs: 16px.'),('32','Action or collection boundary','Tabs to grid. File rows. Content to confirmation actions.'),('16 / 12','Status badge / glyph','One quiet overlay. Hover and focus reveal meaning; tap opens details.')]
for i,(a,b,c) in enumerate(rows):
    yy=192+i*80;s+=txt(a,48,yy,24,C['ink'],600)+txt(b,232,yy,20,C['ink'],600)+txt(c,232,yy+36,15,C['muted'])+rect(48,yy+68,1344,1,C['line'])
s+=txt('History title = device name. Files = icon + name. Dense file groups share a local left axis.',48,720,15,C['ink'],600)+txt('Critical errors keep a short visible reason. Badges reveal routine state on hover / focus / touch.',48,764,15,C['muted'])
(OUT/'15-semantic-contract.svg').write_text(svg(s,1440,832,'Semantic spacing and status rules'),encoding='utf-8')
g=logo(48,40,36)+txt('pinelink',100,40,32,C['ink'],600)+txt('Status lives on the object',48,104,24,C['ink'],600)
for i,(label,mark) in enumerate([('Reusable · on this device','repeat'),('Reusable · cloud copy','cloud'),('One-time · no retained copy','one'),('Expired cloud copy','clock')]):
    cx=216+i*336;yy=288
    g+=fileicon('doc',cx-24,yy,48)+badge(mark,cx+20,yy+40,label)+txt('Design source.fig',cx,yy+56,15,C['ink'],400,'middle')
    g+=rect(cx-144,208,288,40,C['white'],12,C['line'])+txt(label,cx,218,13,C['ink'],400,'middle')
g+=txt('Hover / focus: short status tooltip. Touch: open the same file details. No status caption under the filename.',48,432,15,C['muted'])
for i,(state,mark) in enumerate([('Queued','clock'),('Paused','pause'),('Confirming delivery','shield'),('Delivered','check')]):
    cx=216+i*336;g+=node('laptop','MacBook Pro',cx,584,64,status=None)+badge(mark,cx+22,606,state)+txt(state,cx,680,15,C['muted'],400,'middle')
g+=txt('These labels are specimen annotations. Product graphs keep state in the badge and transfer details.',48,752,15,C['muted'])
(OUT/'15-status-language.svg').write_text(svg(g,1440,816,'Badges and disclosure'),encoding='utf-8')
k=logo(48,40,36)+txt('pinelink',100,40,32,C['ink'],600)+txt('Motion uses the same semantic containers',48,104,24,C['ink'],600)
for row,label in enumerate(['Node entry / 200ms','Drop target / 120ms','Panel entry / 180ms','Delivery / receipt then exit']):
    yy=[192,456,720,1136][row];k+=txt(label,48,yy,20,C['ink'],600)
    for col,step in enumerate(['Start','Middle','End']):
        cx=248+col*464;k+=txt(step,cx,yy+40,13,C['muted'],400,'middle')
        if row==0:k+=group('Node '+step,node('laptop','MacBook Pro',cx,yy+136,64,status=None),f'opacity="{[0,.55,1][col]}"')
        elif row==1:
            if col:k+=circle(cx,yy+136,56,C['soft'],C['blue'],1.5,extra='stroke-dasharray="3 6"')
            k+=node('laptop','MacBook Pro',cx,yy+136,[64,68,72][col],status=None)
            if col:k+=badge('plus',cx+26,yy+162,'Drop to send')
        elif row==2:
            box=rect(cx-200,yy+88,400,248,C['white'],24,C['line'])+txt('Send files?',cx-180,yy+112,20,C['ink'],600)+txt('To MacBook Pro',cx-180,yy+148,15,C['muted'])+fileicon('doc',cx-180,yy+192,40)+txt('Design source.fig',cx-128,yy+202,15)
            k+=group('Panel '+step,box,f'opacity="{[0,.55,1][col]}"')
        else:
            points=[(cx-120,yy+160),(cx-40,yy+160),(cx+40,yy+112),(cx+120,yy+112)]
            k+=path(curve(points,1),C['blue'] if col<2 else C['line'],3)
            if col:k+=badge('check',cx+120,yy+112,'Delivered')
k+=txt('The light pass never changes the acknowledged boundary. Tooltips and compact panels preserve semantic grouping.',48,1416,15,C['muted'])
(OUT/'08-motion-keyframes.svg').write_text(svg(k,1440,1488,'Motion with semantic containers'),encoding='utf-8')
print('Rebuilt',len(manifest),'product screens from shared semantic layouts.')
