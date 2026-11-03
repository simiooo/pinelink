"""Check the optional preview gate and the application-auth handoff."""
import argparse
import json
import re
import subprocess
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--helm', required=True)
parser.add_argument('--chart', required=True)
parser.add_argument('--output', required=True)
parser.add_argument('--render-dir')
args = parser.parse_args()
base = [args.helm, 'template', 'pinelink', args.chart, '--namespace', 'pinelink-staging']
checks = []


def render(name, extra, success=True):
    result = subprocess.run(base + extra, capture_output=True, text=True)
    assert (result.returncode == 0) == success, name + ': ' + result.stderr[-1200:]
    checks.append({'name': name, 'passed': True})
    return result.stdout


default = render('existing previews keep their password gate', [])
assert 'auth_basic "Pinelink private preview";' in default
assert 'mountPath: /etc/pinelink/auth' in default
assert 'secretName: pinelink-edge' in default
public = render('application identity replaces the preview gate', [
    '--set', 'previewAccess.enabled=false', '--set', 'existingSecrets.accounts=account-fixture',
    '--set-string', 'existingSecrets.edge=',
])
assert 'auth_basic' not in public
assert 'mountPath: /etc/pinelink/auth' not in public
assert 'name: auth\n' not in public
assert 'secretRef: {name: account-fixture}' in public
assert 'PINELINK_DEVICE_AUTH_REQUIRED, value: "true"' in public
assert 'proxy_set_header Authorization "";' in public
assert public.count('proxy_ssl_verify on;') == 3
assert 'ssl_protocols TLSv1.2 TLSv1.3;' in public
assert re.search(r'pinelink.web-config-sha256: "([a-f0-9]+)"', default)[1] != re.search(r'pinelink.web-config-sha256: "([a-f0-9]+)"', public)[1]
render('cannot open access before account configuration exists', ['--set', 'previewAccess.enabled=false'], False)
render('account guard also applies when schema validation is skipped', ['--skip-schema-validation', '--set', 'previewAccess.enabled=false'], False)
render('enabled gate needs its password Secret', ['--set-string', 'existingSecrets.edge='], False)
render('gate flag accepts only a boolean', ['--set-string', 'previewAccess.enabled=false'], False)
production = render('production is protected unless explicitly migrated', ['--set', 'environment=production'])
assert 'auth_basic "Pinelink private preview";' in production
legacy = render('old values without the new flag retain the gate', ['--set-json', 'previewAccess=null'])
assert 'auth_basic "Pinelink private preview";' in legacy

if args.render_dir:
    directory = Path(args.render_dir)
    directory.mkdir(parents=True, exist_ok=True)
    for name, source in [('enabled', default), ('disabled', public)]:
        lines = []
        for line in source.split('  nginx.conf: |\n', 1)[1].splitlines():
            if line and not line.startswith('    '):
                break
            lines.append(line[4:] if line else '')
        (directory / (name + '.conf')).write_text('\n'.join(lines) + '\n', encoding='utf-8')
report = {'passed': True, 'execution': 'Helm rendering', 'checks': checks}
Path(args.output).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': True, 'checks': len(checks)}))
