"""Precache a coherent Flutter release; authenticated API calls stay outside SW."""
import hashlib
import json
from pathlib import Path
import sys

root=Path(sys.argv[1]).resolve()
if not all((root/name).is_file() for name in ['index.html','flutter_bootstrap.js','main.dart.js']):
    raise SystemExit('Expected a built Flutter release directory; source files were not changed')
# The dependency graph can contain absolute workstation/cache paths. Filtering
# it out of precaching alone still leaves it publicly downloadable in an archive.
for generated in root.rglob('*'):
    if generated.is_file() and generated.suffix in {'.map','.dart','.deps'}:
        if not generated.resolve().is_relative_to(root):
            raise SystemExit('Generated metadata escaped the release directory')
        generated.unlink()
files=sorted(p for p in root.rglob('*') if p.is_file() and p.suffix not in {'.map','.dart','.deps'} and p.name not in {'pinelink-sw.js','flutter_service_worker.js'})
version=hashlib.sha256(b''.join(p.relative_to(root).as_posix().encode()+hashlib.sha256(p.read_bytes()).digest() for p in files)).hexdigest()[:24]
paths=['./']+[p.relative_to(root).as_posix() for p in files]
source="""const VERSION=__VERSION__, FILES=__FILES__;
self.addEventListener('install',event=>event.waitUntil(caches.open(VERSION).then(cache=>cache.addAll(FILES))));
self.addEventListener('activate',event=>event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(k=>k.startsWith('pinelink-flutter-')&&k!==VERSION).map(k=>caches.delete(k))))));
self.addEventListener('fetch',event=>{
 const url=new URL(event.request.url);
 if(event.request.method!=='GET'||url.origin!==self.location.origin||url.pathname.startsWith('/api/'))return;
 event.respondWith(caches.open(VERSION).then(async cache=>{
   const key=event.request.mode==='navigate'?'./':url.pathname;
   const cached=await cache.match(key,{ignoreSearch:true});
   return cached||fetch(event.request);
 }));
});
""".replace('__VERSION__',json.dumps('pinelink-flutter-'+version)).replace('__FILES__',json.dumps(paths))
(root/'pinelink-sw.js').write_text(source,encoding='utf-8')
print(json.dumps({'cache':version,'files':len(paths),'bytes':sum(p.stat().st_size for p in files)}))
