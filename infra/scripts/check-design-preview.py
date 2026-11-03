"""Render-only Helm checks for the opt-in, staging-only design preview."""
import argparse,json,subprocess
from pathlib import Path

parser=argparse.ArgumentParser();parser.add_argument('--helm',required=True);parser.add_argument('--chart',required=True);parser.add_argument('--output',required=True);args=parser.parse_args()
base=[args.helm,'template','pinelink',args.chart,'--namespace','pinelink-staging'];checks=[]
def run(name,extra,ok=True):
    result=subprocess.run(base+extra,capture_output=True,text=True)
    passed=(result.returncode==0)==ok;checks.append({'name':name,'passed':passed,'exit':result.returncode})
    if not passed:raise RuntimeError(name+': '+result.stderr[-1200:])
    return result.stdout
lint=subprocess.run([args.helm,'lint',args.chart],capture_output=True,text=True);assert lint.returncode==0,lint.stderr
default=run('default preview disabled',[])
assert '/usr/share/nginx/html/designs' not in default
assert default.count('pinelink.web-config-sha256:')==1
enabled=run('staging preview enabled',['--set','designPreview.enabled=true','--set','designPreview.configMap=design-fixture'])
assert enabled.count('mountPath: /usr/share/nginx/html/designs')==1
assert 'configMap: {name: design-fixture}' in enabled
assert "sandbox allow-scripts; default-src 'none'" in enabled
assert 'try_files $uri =404' in enabled
assert 'private, no-store, no-transform' in enabled
run('production rejects enabled preview',['--set','environment=production','--set','designPreview.enabled=true','--set','designPreview.configMap=design-fixture'],False)
run('enabled preview needs config map',['--set','designPreview.enabled=true'],False)
production=run('production without preview still renders',['--set','environment=production'])
assert '/usr/share/nginx/html/designs' not in production
run('template guard survives skipped schema',['--skip-schema-validation','--set','environment=production','--set','designPreview.enabled=true','--set','designPreview.configMap=design-fixture'],False)
web_version='1234567890abcdef1234567890abcdef12345678'
web_only=run('web revision can advance independently',['--set-string','webVersion='+web_version])
assert web_only.count('pinelink.source-revision: "'+web_version+'"')==1
assert web_only.count('pinelink.source-revision: "development"')==2
run('web revision rejects invalid source hash',['--set-string','webVersion=latest'],False)
version=subprocess.check_output([args.helm,'version','--short'],text=True).strip()
Path(args.output).write_text(json.dumps({'helm':version,'lint_passed':True,'checks':checks,'passed':True,'render_only':True},indent=2)+'\n',encoding='utf-8')
