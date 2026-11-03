"""Account for design targets without claiming that unbuilt features are live."""
import json
from pathlib import Path

base = Path(__file__).resolve().parent
catalog = json.loads((base / 'catalog.json').read_text(encoding='utf-8'))
bindings = {}

def bind(codes, status, web, flutter, limitation=''):
    for code in codes.split():
        bindings[code] = dict(status=status, web=web, flutter=flutter, limitation=limitation)

bind('M001 M002 M003 M004 M005 M006 M008 M009 M012 M013 M016 M017 M018',
     'bound-existing', 'web/src/motion/components.tsx; web/src/components/AppShell.tsx; web/src/components/SendPreparation.tsx',
     'mobile/lib/src/motion/motion.dart; mobile/lib/src/components/app_shell.dart',
     'Existing controls and feedback only; this does not imply every future feature instance exists. Native menus/tooltips retain platform behavior.')
bind('M029 M030 M047 M048 M049 M050 M051 M052 M053 M054 M056 M059 M060',
     'bound-existing', 'web/src/motion/graph.tsx; web/src/routes/tasks.tsx',
     'mobile/lib/src/motion/transfer_graph.dart; mobile/lib/src/pages/tasks_page.dart',
     'Driven by existing task snapshots and controls. Verification is awaiting receipt, never a timer-generated completion. Cloud stored and delivered remain separate.')
bind('M036 M037 M039 M042', 'bound-existing', 'web/src/motion/dropzone.tsx; web/src/routes/index.tsx',
     'Native file picker; node drop feature dependent',
     'Web preparation target only. No node-near arbitration or upload is claimed.')
bind('M061 M062 M063 M064 M066 M068', 'bound-existing',
     'web/src/motion/surface.tsx; web/src/components/AppShell.tsx',
     'mobile/lib/src/motion/surface.dart; mobile/lib/src/motion/motion.dart',
     'Actual task details and existing route changes; file-manager history panels remain feature dependent.')
bind('M076 M099 M106 M109', 'bound-existing',
     'web/src/routes/settings.tsx; web/src/components/AppShell.tsx',
     'mobile/lib/src/pages/settings_page.dart; mobile/lib/src/components/app_shell.dart',
     'Current persistence/revoke/language/quota controls, not the future peer-trust flow.')
bind('M007 M011 M014 M015 M067 M069 M070 M071 M074', 'foundation-ready',
     'web/src/components/ui; web/src/motion/components.tsx', 'mobile/lib/src/motion',
     'Primitives, keyed measurements and feedback are available and tested; the matching feature page is not yet bound. Native context-menu placement remains platform owned.')

rows=[]
for target in catalog['targets']:
    row={'code':target['code'], 'id':target['id'], 'recipe':target['recipe'], 'design_mode':target['mode']}
    row.update(bindings.get(target['code'], dict(status='feature-dependent', web='', flutter='',
        limitation='Requires the actual device/file/account/peer/text/offline-service feature in its owning engineering unit. The v29 prototype is design evidence only.')))
    rows.append(row)
assert len(rows)==109 and len({row['code'] for row in rows})==109
summary={status:sum(row['status']==status for row in rows) for status in sorted({row['status'] for row in rows})}
result={'schema':1, 'design_version':29, 'adopted_on':'2026-11-03',
        'scope':'Shared motion and existing client bindings; independent unbuilt features are not counted as delivered.',
        'summary':summary, 'targets':rows}
(base/'runtime-bindings.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(summary))
