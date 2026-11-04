"""Real browser processes/net namespaces; no shared firewall or production changes.

Run on a Linux Docker build host with an already built web/dist. Browser image
must contain Node, the project's Playwright browsers/dependencies and iptables.
Only per-run containers receive NET_ADMIN; the driver never mounts Docker's
socket. Fixture mode also needs the pinned Go SDK and builds accountfixture.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

p=argparse.ArgumentParser()
p.add_argument('--source',required=True)
p.add_argument('--output',required=True)
p.add_argument('--image',default='pinelink.local/direct-browser-checks:sim947')
p.add_argument('--browser-cache',required=True)
p.add_argument('--firefox-cache')
p.add_argument('--browser-b',choices=['chromium','firefox'],default='chromium')
p.add_argument('--origin',default='http://localhost:5184')
p.add_argument('--cases',default='direct,empty,pending-close,blocked,drop,pause,cancel')
p.add_argument('--go-toolchain')
p.add_argument('--go-mod-cache')
p.add_argument('--go-build-cache')
p.add_argument('--serverless',action='store_true',help='Cache PWA, deny public/API/DNS traffic, use out-of-band local pairing')
p.add_argument('--standalone',action='store_true',help='Serve the first app load from the bundled loopback CLI; no center fixture')
a=p.parse_args()
assert not a.standalone or a.serverless and a.origin.startswith('http://localhost:')
source=Path(a.source).resolve();out=Path(a.output).resolve()
assert (source/'web/package.json').is_file() and (source/'web/dist/index.html').is_file()
assert out.is_relative_to(source/'.artifacts'),'Evidence must stay in this checkout'
out.mkdir(parents=True,exist_ok=True)
run=uuid.uuid4().hex[:10];prefix='pinelink-direct-'+run;network=prefix+'-net'
labels=['--label','pinelink.issue='+('SIM-955' if a.serverless else 'SIM-947'),'--label','pinelink.direct_run='+run]
containers=[];blocked=False;peers={};pid=None

def call(args,check=True):
    return subprocess.run(['docker',*args],capture_output=True,check=check,text=True)
def inspect(name):
    return json.loads(call(['inspect',name]).stdout)[0]
def owned(name):
    row=inspect(name);assert row['Config']['Labels'].get('pinelink.direct_run')==run
    return row
def start(slot,command,env=(),caps=False,memory='1536m',work='/source/web'):
    name=prefix+'-'+slot
    args=['run','-d','--init','--name',name,*labels,'--network',network,'--user','1000:1000',
        '--cpus','2','--memory',memory,'--pids-limit','512','--cap-drop','ALL',
        '--security-opt','no-new-privileges','--mount','type=bind,src='+str(source)+',dst=/source,readonly',
        '--mount','type=bind,src='+str(out)+',dst=/output','--workdir',work]
    if caps:args+=['--cap-add','NET_ADMIN','--cap-add','NET_RAW']
    for key,value in env:args+=['--env',key+'='+str(value)]
    args += [a.image,*command]
    call(args);containers.append(name);return name
def set_network(value):
    global blocked
    for slot,peer in peers.items():
        owned(peer['name'])
        other=peers['b' if slot=='a' else 'a']
        for address in other['addresses']:
            rule=['OUTPUT','-d',address+'/32','-j','REJECT']
            exists=call(['exec','-u','0',peer['name'],'iptables','-C',*rule],False).returncode==0
            if value and not exists:call(['exec','-u','0',peer['name'],'iptables','-I',*rule])
            if not value and exists:call(['exec','-u','0',peer['name'],'iptables','-D',*rule])
    blocked=value

def set_offline(value):
    evidence={}
    control=owned(prefix+'-driver')
    controller_addresses=[v['IPAddress'] for v in control['NetworkSettings']['Networks'].values() if v['IPAddress']]
    for slot,peer in peers.items():
        owned(peer['name'])
        for binary in ['iptables','ip6tables']:
            exists=call(['exec','-u','0',peer['name'],binary,'-S','PL_OFFLINE'],False).returncode==0
            if exists:
                call(['exec','-u','0',peer['name'],binary,'-D','OUTPUT','-j','PL_OFFLINE'],False)
                call(['exec','-u','0',peer['name'],binary,'-F','PL_OFFLINE'])
                call(['exec','-u','0',peer['name'],binary,'-X','PL_OFFLINE'])
            if not value:continue
            call(['exec','-u','0',peer['name'],binary,'-N','PL_OFFLINE'])
            if binary=='iptables':
                for address in peers['b' if slot=='a' else 'a']['addresses']+controller_addresses:
                    call(['exec','-u','0',peer['name'],binary,'-A','PL_OFFLINE','-d',address+'/32','-j','ACCEPT'])
            multicast='224.0.0.251/32' if binary=='iptables' else 'ff02::fb/128'
            call(['exec','-u','0',peer['name'],binary,'-A','PL_OFFLINE','-d',multicast,'-p','udp','--dport','5353','-j','ACCEPT'])
            call(['exec','-u','0',peer['name'],binary,'-A','PL_OFFLINE','-j','REJECT'])
            call(['exec','-u','0',peer['name'],binary,'-I','OUTPUT','1','-j','PL_OFFLINE'])
        evidence[slot]=call(['exec','-u','0',peer['name'],'iptables','-S','PL_OFFLINE'],False).stdout
    return evidence

try:
    call(['network','create',*labels,network])
    api=None
    if a.standalone:
        assert a.go_toolchain,'Standalone mode requires the pinned Go SDK'
        binary='/source/.artifacts/local-web-'+run
        cmd=['run','--rm',*labels,'--user','1000:1000','--cpus','2','--memory','3g',
            '--mount','type=bind,src='+str(source)+',dst=/source',
            '--mount','type=bind,src='+a.go_toolchain+',dst=/opt/go,readonly',
            '--mount','type=bind,src='+a.go_mod_cache+',dst=/go-mod',
            '--mount','type=bind,src='+a.go_build_cache+',dst=/go-build',
            '--env','GOTOOLCHAIN=local','--env','GOMAXPROCS=2','--env','CGO_ENABLED=0',
            '--env','GOMODCACHE=/go-mod','--env','GOCACHE=/go-build','--workdir','/source/cli',
            a.image,'/opt/go/bin/go','build','-p','2','-o',binary,'./cmd/pinelink']
        result=call(cmd,False);(out/'launcher-build.log').write_text(result.stdout+result.stderr);assert result.returncode==0,'Standalone launcher build failed'
    elif a.origin.startswith('http://localhost:'):
        assert a.go_toolchain,'Fixture mode requires the pinned Go SDK'
        db=prefix+'-db'
        call(['run','-d','--name',db,*labels,'--network',network,'--cpus','1','--memory','512m',
            '--pids-limit','128','--env','POSTGRES_USER=pinelink_test',
            '--env','POSTGRES_PASSWORD=isolated_direct_tests_only','--env','POSTGRES_DB=pinelink_test','postgres:17-alpine'])
        containers.append(db)
        for _ in range(100):
            if call(['exec',db,'pg_isready','-U','pinelink_test','-d','pinelink_test'],False).returncode==0:break
            time.sleep(.1)
        else:raise RuntimeError('Isolated PostgreSQL not ready')
        binary='/source/.artifacts/direct-fixture-'+run
        mod=Path(a.go_mod_cache or source/'.artifacts/go-mod');build=Path(a.go_build_cache or source/'.artifacts/go-build')
        mod.mkdir(parents=True,exist_ok=True);build.mkdir(parents=True,exist_ok=True)
        cmd=['run','--rm',*labels,'--user','1000:1000','--cpus','2','--memory','3g',
            '--mount','type=bind,src='+str(source)+',dst=/source',
            '--mount','type=bind,src='+a.go_toolchain+',dst=/opt/go,readonly',
            '--mount','type=bind,src='+str(mod)+',dst=/go-mod',
            '--mount','type=bind,src='+str(build)+',dst=/go-build',
            '--env','GOTOOLCHAIN=local','--env','GOMAXPROCS=2','--env','CGO_ENABLED=0',
            '--env','GOMODCACHE=/go-mod','--env','GOCACHE=/go-build','--workdir','/source/backend',
            a.image,'/opt/go/bin/go','build','-p','2','-o',binary,'./cmd/accountfixture']
        result=call(cmd,False);(out/'fixture-build.log').write_text(result.stdout+result.stderr);assert result.returncode==0,'Fixture build failed'
        fixture_name=prefix+'-fixture'
        start('fixture',['node','/source/web/e2e/direct-fixture-bridge.mjs'],env=[
            ('PINELINK_DIRECT_FIXTURE_BINARY',binary),
            ('PINELINK_SERVICE_TEST_DATABASE_URL','postgres://pinelink_test:isolated_direct_tests_only@'+db+'/pinelink_test?sslmode=disable')],work='/source/backend')
        api='http://'+fixture_name+':8094'
    for slot,kind in [('a','chromium'),('b',a.browser_b)]:
        name=prefix+'-'+slot;cache=a.firefox_cache if kind=='firefox' else a.browser_cache
        assert cache
        browser_env=[('PINELINK_DIRECT_SLOT',slot),('PINELINK_DIRECT_PEER',name),
            ('PINELINK_DIRECT_BROWSER',kind),('PINELINK_DIRECT_ORIGIN',a.origin),
            ('PLAYWRIGHT_BROWSERS_PATH','/browser-cache'),('LANG','C.UTF-8'),('LC_ALL','C.UTF-8')]
        if api:browser_env.append(('PINELINK_BROWSER_API_BASE',api))
        if a.standalone:browser_env.append(('PINELINK_LOCAL_WEB_BINARY',binary))
        # Cache is read-only; browser state/downloads live in this container.
        # start() supports the extra mount through a tiny image-neutral command.
        args=['run','-d','--init','--name',name,*labels,'--network',network,'--user','1000:1000',
            '--cpus','2','--memory','2g','--pids-limit','512','--cap-drop','ALL','--cap-add','NET_ADMIN','--cap-add','NET_RAW',
            '--security-opt','no-new-privileges','--mount','type=bind,src='+str(source)+',dst=/source,readonly',
            '--mount','type=bind,src='+str(out)+',dst=/output',
            '--mount','type=bind,src='+str(Path(cache).resolve())+',dst=/browser-cache,readonly','--workdir','/source/web']
        for key,value in browser_env:args+=['--env',key+'='+str(value)]
        call(args+[a.image,'node','e2e/direct-browser-host.mjs']);containers.append(name)
        row=owned(name);addresses=[v['IPAddress'] for v in row['NetworkSettings']['Networks'].values() if v['IPAddress']]
        namespace=call(['exec',name,'readlink','/proc/1/ns/net']).stdout.strip()
        peers[slot]={'name':name,'addresses':addresses,'namespace':namespace}
    assert peers['a']['namespace']!=peers['b']['namespace'] and set(peers['a']['addresses']).isdisjoint(peers['b']['addresses'])
    for _ in range(300):
        if all((out/(s+'.json')).is_file() for s in ['a','b']):break
        for name in [peers['a']['name'],peers['b']['name']]:
            if not owned(name)['State']['Running']:
                (out/(name+'-startup.log')).write_text(call(['logs',name],False).stdout+call(['logs',name],False).stderr)
                raise RuntimeError('Browser host failed; inspect startup evidence')
        time.sleep(.2)
    else:raise RuntimeError('Browser websocket endpoints not ready')
    env=[('PINELINK_DIRECT_OUTPUT','/output'),('PINELINK_DIRECT_ORIGIN',a.origin),('PINELINK_DIRECT_CASES',a.cases)]
    if api:env.append(('PINELINK_BROWSER_API_BASE',api))
    driver=prefix+'-driver'
    cmd=['run','--rm','--name',driver,*labels,'--network',network,'--user','1000:1000','--cpus','2',
        '--memory','2g','--pids-limit','512','--cap-drop','ALL','--security-opt','no-new-privileges',
        '--mount','type=bind,src='+str(source)+',dst=/source,readonly','--mount','type=bind,src='+str(out)+',dst=/output','--workdir','/source/web']
    for key,value in env:cmd+=['--env',key+'='+value]
    cmd += [a.image,'node','e2e/serverless.mjs' if a.serverless else 'e2e/direct.mjs'];containers.append(driver)
    with (out/'browser.log').open('w') as log:
        pid=subprocess.Popen(['docker',*cmd],stdout=log,stderr=subprocess.STDOUT,text=True)
        started=time.monotonic();handled=set()
        while pid.poll() is None:
            if time.monotonic()-started>900:raise TimeoutError('Bounded browser test timeout')
            request=out/'network-request.json'
            if request.exists():
                body=json.loads(request.read_text())
                if body['id'] not in handled:
                    assert isinstance(body['blocked'],bool)
                    firewall=set_offline(body['blocked']) if body.get('action')=='offline' else (set_network(body['blocked']) or {})
                    handled.add(body['id'])
                    tmp=out/'network-result.tmp';tmp.write_text(json.dumps({'id':body['id'],'applied':True,'firewall':firewall}));tmp.replace(out/'network-result.json')
                    print('Namespace-local network action applied: '+body.get('action','peer'),flush=True)
            time.sleep(.1)
    assert pid.returncode==0,'Real browser test failed; inspect '+str(out/'browser.log')
    report=json.loads((out/'verification.json').read_text());assert report['passed']
    report.update({'execution_host':'Fedora','namespace_evidence':peers,'kernel_network_failures_tested':a.serverless or 'blocked' in a.cases or 'drop' in a.cases})
    (out/'verification.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'passed':True,'checks':len(report['checks']),'origin':a.origin,'output':str(out)}),flush=True)
finally:
    if pid and pid.poll() is None:pid.terminate()
    for name in reversed(containers):
        if call(['inspect',name],False).returncode==0:
            owned(name);logs=call(['logs',name],False);(out/(name+'.log')).write_text(logs.stdout+logs.stderr)
            call(['stop','--time','8',name],False);call(['rm','-v',name],False)
    if call(['network','inspect',network],False).returncode==0:
        row=json.loads(call(['network','inspect',network]).stdout)[0]
        assert row['Labels'].get('pinelink.direct_run')==run and not row.get('Containers')
        call(['network','rm',network])
