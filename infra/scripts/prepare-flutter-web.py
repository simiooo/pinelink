"""Prepare the pinned SQLite WASM and compile Drift's worker before Flutter Web."""
import argparse, hashlib, json, os, subprocess, tempfile, urllib.request
from pathlib import Path

root=Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser();parser.add_argument('--dart',default='dart');args=parser.parse_args()
wasm=root/'mobile/web/sqlite3.wasm'
expected='efd308be92a34f3aa15f289a37f213b0230fd80888a1364326ee29579d892d37'
if not wasm.exists():
    data=urllib.request.urlopen('https://github.com/simolus3/drift/releases/download/drift-2.35.3/sqlite3.wasm',timeout=60).read()
    if hashlib.sha256(data).hexdigest()!=expected:raise SystemExit('SQLite WASM checksum mismatch')
    with tempfile.NamedTemporaryFile(dir=wasm.parent,suffix='.wasm',delete=False) as stream:
        stream.write(data);temporary=Path(stream.name)
    os.replace(temporary,wasm)
if hashlib.sha256(wasm.read_bytes()).hexdigest()!=expected:raise SystemExit('SQLite WASM checksum mismatch')
subprocess.run([args.dart,'compile','js','-O4','--no-source-maps','web/drift_worker.dart','-o','web/drift_worker.js'],cwd=root/'mobile',check=True)
worker=root/'mobile/web/drift_worker.js'
if not worker.is_file() or not worker.stat().st_size:raise SystemExit('Compiled Drift worker missing')
print(json.dumps({'sqlite_wasm_sha256':expected,'worker_sha256':hashlib.sha256(worker.read_bytes()).hexdigest()}))
