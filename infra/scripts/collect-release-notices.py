"""Collect exact dependency license files for a developer preview archive."""
from pathlib import Path
from urllib.parse import urlparse, unquote
import argparse, json, re, shutil, subprocess

ROOT=Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser()
parser.add_argument('--output',required=True)
args=parser.parse_args()
out=Path(args.output).resolve()
out.mkdir(parents=True,exist_ok=True)
records=[]
missing=[]

def notices(kind,name,version,directory,declared=None):
    directory=Path(directory)
    candidates=[p for p in directory.iterdir() if p.is_file() and re.match(r'^(?:licen[cs]e|copying|notice|authors|patents)(?:$|[._-])',p.name,re.I)]
    if kind=='font' and (directory/'OFL.txt').is_file():
        candidates.append(directory/'OFL.txt')
        if (directory/'provenance.json').is_file():candidates.append(directory/'provenance.json')
    # This tarball declares MIT but omits its file. Package and pinned upstream
    # notice provenance are recorded beside the reviewed copy.
    reviewed={('npm','react-remove-scroll-bar','2.3.8'):ROOT/'third_party/react-remove-scroll-bar-2.3.8'}
    if not candidates and (kind,name,version) in reviewed:
        directory=reviewed[(kind,name,version)]
        candidates=[directory/'LICENSE'] if (directory/'LICENSE').is_file() else []
        if candidates and (directory/'provenance.json').is_file():candidates.append(directory/'provenance.json')
    if not candidates:
        missing.append({'kind':kind,'name':name,'version':version,'declared':declared})
        return
    slug=re.sub(r'[^A-Za-z0-9._-]','_',name+'@'+version)
    target=out/kind/slug
    target.mkdir(parents=True,exist_ok=True)
    for p in candidates:shutil.copyfile(p,target/p.name)
    records.append({'kind':kind,'name':name,'version':version,'declared':declared,'files':[p.relative_to(out).as_posix() for p in sorted(target.iterdir())]})

def json_sequence(text):
    decoder=json.JSONDecoder();index=0
    while index<len(text):
        if text[index:].strip()=='':break
        index+=len(text[index:])-len(text[index:].lstrip())
        value,index=decoder.raw_decode(text,index)
        yield value

modules={}
import os
for module_name,command,targets in [
    ('backend','pinelinkd',[('linux','amd64'),('linux','arm64')]),
    ('cli','pinelink',[('windows','amd64'),('linux','amd64'),('linux','arm64'),('darwin','amd64'),('darwin','arm64')]),
]:
    for goos,goarch in targets:
        environment={**os.environ,'GOOS':goos,'GOARCH':goarch,'CGO_ENABLED':'0'}
        raw=subprocess.check_output(['go','list','-deps','-json','./cmd/'+command],cwd=ROOT/module_name,env=environment,text=True,encoding='utf-8')
        for package in json_sequence(raw):
            module=package.get('Module')
            if module and not module.get('Main') and not module['Path'].startswith('github.com/simiooo/pinelink/'):
                modules[(module['Path'],module['Version'])]=module
for module in modules.values():
    notices('go',module['Path'],module['Version'],module['Dir'])
goroot=subprocess.check_output(['go','env','GOROOT'],text=True).strip()
goversion=subprocess.check_output(['go','env','GOVERSION'],text=True).strip()
notices('go','Go standard library and runtime',goversion,goroot)

lock=json.loads((ROOT/'web/package-lock.json').read_text(encoding='utf-8'))
for path,entry in lock['packages'].items():
    if not path:continue
    directory=ROOT/'web'/path
    package=directory/'package.json'
    if not package.is_file():continue
    metadata=json.loads(package.read_text(encoding='utf-8'))
    if entry.get('dev') and not metadata['name'].startswith('workbox-'):continue
    notices('npm',metadata['name'],metadata['version'],directory,metadata.get('license'))

notices('source','shadcn-ui-components','4.23.0',ROOT/'web/src/components/ui','MIT')
notices('font','Noto Sans SC','a85815a42757',ROOT/'mobile/assets/fonts/noto-sans-sc','OFL-1.1')
notices('font','Nunito','Variable',ROOT/'web/public/fonts','OFL-1.1')

config=ROOT/'mobile/.dart_tool/package_config.json'
if config.is_file():
    sdk=None
    for package in json.loads(config.read_text(encoding='utf-8'))['packages']:
        uri=package['rootUri']
        if uri.startswith('file:'):
            path=unquote(urlparse(uri).path)
            if re.match(r'^/[A-Za-z]:/',path):path=path[1:]
            directory=Path(path)
        else:directory=(config.parent/uri).resolve()
        if package['name']=='pinelink_mobile':continue
        if package['name'].startswith('flutter') and directory.parent.name=='packages':
            sdk=directory.parent.parent
            continue
        pubspec=directory/'pubspec.yaml'
        version='sdk'
        if pubspec.is_file():
            match=re.search(r'^version:\s*[\'\"]?([^\s\'\"]+)',pubspec.read_text(encoding='utf-8'),re.M)
            if match:version=match[1]
        notices('dart',package['name'],version,directory)
    if sdk:
        notices('flutter','Flutter SDK',json.loads((ROOT/'.fvmrc').read_text())['flutter'],sdk)
        notices('flutter','Dart SDK',json.loads((ROOT/'mobile/.dart_tool/package_config.json').read_text())['generatorVersion'],sdk/'bin/cache/dart-sdk')

records.sort(key=lambda r:(r['kind'],r['name']))
(out/'dependency-licenses.json').write_text(json.dumps({'dependencies':records,'missing':missing},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
lines=['# Third-party notices','','License files for the dependency versions used by this preview build. Docker images are obtained separately from their registries.','', '| Kind | Component | Version | License files |','| --- | --- | --- | --- |']
for r in records:
    links=', '.join(f'[{Path(p).name}]({p})' for p in r['files'])
    lines.append(f"| {r['kind']} | {r['name']} | {r['version']} | {links} |")
(out/'THIRD_PARTY_NOTICES.md').write_text('\n'.join(lines)+'\n',encoding='utf-8')
print(json.dumps({'components':len(records),'missing':missing},ensure_ascii=False))
if missing:raise SystemExit('Missing license files; resolve before publishing')
