"""Token-based inventory; constructor/handler sites, not a Dart type analyzer."""
import json,re,sys
from pathlib import Path
root=Path(sys.argv[1] if len(sys.argv)>1 else '.').resolve()
files=[];classes=[];calls=[];handlers=[];motion=[]
def mask(text):
    # Preserve offsets/newlines while skipping comments and quoted literals.
    pattern=r'//[^\n]*|/\*[\s\S]*?\*/|r?(?:\x27\x27\x27[\s\S]*?\x27\x27\x27|"""[\s\S]*?"""|\x27(?:\\.|[^\x27\\])*\x27|"(?:\\.|[^"\\])*")'
    return re.sub(pattern,lambda m:re.sub(r'[^\n]',' ',m.group()),text)
for file in sorted((root/'mobile/lib').rglob('*.dart')):
    if file.name.endswith('.g.dart'):continue
    raw=file.read_text('utf-8');code=mask(raw);path=file.relative_to(root).as_posix()
    def location(m):return {'file':path,'line':raw.count('\n',0,m.start())+1}
    for m in re.finditer(r'\bclass\s+(\w+)\s+(?:extends\s+([\w<>]+))?',code):
        base=m.group(2) or '';classes.append({'name':m.group(1),'base':base,'ui':bool(re.search(r'(?:Widget|State|Painter)',base)),**location(m)})
    for m in re.finditer(r'\b(_?[A-Z]\w*)(?:<[^<>\n]*>)?(?:\.([a-zA-Z_]\w*))?\s*\(',code):
        calls.append({'name':m.group(1),'constructor':m.group(2),**location(m)})
    for m in re.finditer(r'\b(on(?:Pressed|Tap|LongPress|Changed|Selected|Submitted|Hover|FocusChange|Pointer\w*|Pan\w*|Drag\w*))\s*:',code):handlers.append({'event':m.group(1),**location(m)})
    for m in re.finditer(r'\b(AnimationController|Animated\w+|Tween\w*|\w*TickerProvider\w*|TickerMode|disableAnimations(?:Of)?)\b',code):motion.append({'symbol':m.group(1),**location(m)})
    files.append({'file':path,'lines':raw.count('\n')+1})
data={'method':'Dart token inventory; comments/literals/generated sources excluded. Calls include non-widget constructors and are explicitly classified during review, not counted as widgets automatically.','files':files,'classes':classes,'constructors':calls,'handlers':handlers,'motion':motion}
result=json.dumps(data,indent=2,ensure_ascii=False)+'\n'
if len(sys.argv)>2:Path(sys.argv[2]).write_text(result,encoding='utf-8',newline='\n')
else:print(result)
