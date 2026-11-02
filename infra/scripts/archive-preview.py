"""Archive one prepared directory; Unix launchers retain their executable mode."""
from pathlib import Path
import argparse, gzip, tarfile, zipfile

parser=argparse.ArgumentParser()
parser.add_argument('source',type=Path)
parser.add_argument('output',type=Path)
args=parser.parse_args()
source=args.source.resolve()
if args.output.name.endswith('.zip'):
    with zipfile.ZipFile(args.output,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
        for p in sorted(source.rglob('*')):
            if p.is_file():archive.write(p,p.relative_to(source).as_posix())
else:
    with args.output.open('wb') as raw,gzip.GzipFile(filename='',mode='wb',fileobj=raw,mtime=0) as compressed,tarfile.open(fileobj=compressed,mode='w') as archive:
        for p in sorted(source.rglob('*')):
            if not p.is_file():continue
            info=archive.gettarinfo(str(p),p.relative_to(source).as_posix())
            info.uid=info.gid=0;info.uname=info.gname='';info.mtime=0
            info.mode=0o755 if p.name in ['pinelink','pinelinkd'] else 0o644
            with p.open('rb') as stream:archive.addfile(info,stream)
