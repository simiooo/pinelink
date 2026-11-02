"""Provision private Pinelink buckets/tunnels. Values stay in Nuto Secrets.

The state file contains resource IDs and vault references, never credential values.
Public application CI does not run this operator-only command.
"""
import argparse
import base64
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import shutil
import subprocess
import urllib.error
import urllib.parse
import urllib.request


class ProvisionError(Exception):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        return None


def vault(*parts, value=None):
    result = subprocess.run(
        ['nuto', '--env', 'staging', '--project', 'Pinelink', 'secret', *parts],
        input=value, capture_output=True, timeout=60,
    )
    if result.returncode:
        # CLI metadata may contain an authorization task. Never log input values.
        raise ProvisionError('Nuto secret operation failed or needs authorization; inspect its task metadata')
    return result.stdout


def resolve(reference):
    return vault('resolve', reference, '--raw').decode('utf-8').strip()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--token-reference', required=True)
    parser.add_argument('--zone', required=True)
    parser.add_argument('--staging-host', required=True)
    parser.add_argument('--production-host', required=True)
    parser.add_argument('--state', required=True, type=Path)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--openssl', default=shutil.which('openssl') or str(Path(os.environ.get('ProgramFiles', '/nonexistent')) / 'Git/usr/bin/openssl.exe'))
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    state_file = args.state.resolve()
    if state_file.is_relative_to(root):
        relative = state_file.relative_to(root)
        if subprocess.run(['git', 'check-ignore', '-q', str(relative)], cwd=root).returncode:
            raise ProvisionError('State must be outside Git or in a verified ignored directory')
    token = resolve(args.token_reference)
    if token.startswith('"'):
        token = json.loads(token)
    if isinstance(token, str) and token.startswith('{'):
        fields = json.loads(token)
        token = next((fields[k] for k in ['api_token', 'token', 'CLOUDFLARE_API_TOKEN'] if isinstance(fields.get(k), str)), None)
    if not isinstance(token, str): raise ProvisionError('Expected a Cloudflare API token field')
    for prefix in ['CLOUDFLARE_API_TOKEN=', 'CF_API_TOKEN=']:
        if token.startswith(prefix): token = token[len(prefix):].strip().strip('"\'')
    if not isinstance(token, str) or any(c.isspace() for c in token):
        raise ProvisionError('Expected a Cloudflare API token')
    state = json.loads(state_file.read_text()) if state_file.exists() else {'environments': {}}
    opener = urllib.request.build_opener(NoRedirect())

    def save():
        state_file.parent.mkdir(parents=True, exist_ok=True)
        temporary = state_file.with_suffix('.tmp')
        with os.fdopen(os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), 'w') as stream:
            json.dump(state, stream, indent=2)
        os.replace(temporary, state_file)

    def api(method, path, payload=None, allow_missing=False):
        request = urllib.request.Request(
            'https://api.cloudflare.com/client/v4' + path,
            data=None if payload is None else json.dumps(payload).encode(), method=method,
            headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'},
        )
        try:
            with opener.open(request, timeout=40) as response:
                data = json.load(response)
        except urllib.error.HTTPError as error:
            if allow_missing and error.code == 404:
                return None
            # Response messages can echo provider inputs. Print only codes.
            try:
                codes = [item.get('code') for item in json.load(error).get('errors', [])]
            except (ValueError, OSError):
                codes = []
            operation = re.sub(r'/[a-f0-9]{32}(?=/|$)', '/<id>', path.split('?')[0])
            raise ProvisionError(f'Cloudflare {method} {operation} failed: HTTP {error.code}, codes {codes}') from None
        if not data.get('success'):
            raise ProvisionError('Cloudflare operation unsuccessful')
        return data.get('result')

    zones = api('GET', '/zones?' + urllib.parse.urlencode({'name': args.zone}))
    if len(zones) != 1:
        raise ProvisionError('Expected exactly one authorized zone')
    zone = zones[0]['id']
    account = zones[0]['account']['id']
    if state.get('zone_id', zone) != zone or state.get('account_id', account) != account:
        raise ProvisionError('State belongs to another account/zone')
    state.update(zone_id=zone, account_id=account)
    groups = api('GET', f'/accounts/{account}/tokens/permission_groups')
    bucket_group = next((g['id'] for g in groups if g['name'] == 'Workers R2 Storage Bucket Item Write'), None)
    if bucket_group is None:
        raise ProvisionError('Bucket-scoped R2 token permission unavailable')
    if not args.apply:
        print(json.dumps({'zone': args.zone, 'credential_available': True, 'will_mutate': False,
                          'hosts': [args.staging_host, args.production_host]}))
        return

    # A rule scoped to these hosts prevents a pre-existing cache-everything rule
    # from serving an authenticated response to another visitor.
    cache_rule = {'ref': 'pinelink_private_no_cache', 'description': 'Pinelink private preview: bypass edge cache',
                  'expression': '(http.host in {' + ' '.join(json.dumps(h) for h in [args.staging_host, args.production_host]) + '})',
                  'action': 'set_cache_settings', 'action_parameters': {'cache': False}, 'enabled': True}
    ruleset = api('GET', f'/zones/{zone}/rulesets/phases/http_request_cache_settings/entrypoint', allow_missing=True)
    if ruleset:
        existing = next((r for r in ruleset.get('rules', []) if r.get('ref') == cache_rule['ref']), None)
        if existing and existing['id'] != state.get('cache_rule_id'):
            raise ProvisionError('Cache rule exists without matching ownership; inspect before adopting')
        if existing:
            rule = api('PATCH', f'/zones/{zone}/rulesets/{ruleset["id"]}/rules/{existing["id"]}', cache_rule)
            state['cache_rule_id'] = existing['id']
        else:
            updated = api('POST', f'/zones/{zone}/rulesets/{ruleset["id"]}/rules', cache_rule)
            state['cache_rule_id'] = next(r['id'] for r in updated['rules'] if r.get('ref') == cache_rule['ref'])
        state['cache_ruleset_id'] = ruleset['id']
    else:
        created = api('POST', f'/zones/{zone}/rulesets', {'name':'Pinelink private preview', 'kind':'zone',
                      'phase':'http_request_cache_settings', 'rules':[cache_rule]})
        state['cache_ruleset_id'] = created['id']
        state['cache_rule_id'] = next(r['id'] for r in created['rules'] if r.get('ref') == cache_rule['ref'])
    save()

    def store_credentials(environment, purpose, value):
        record = state['environments'][environment]
        key = purpose + '_reference'
        if key in record:
            return record[key]
        name = f'pinelink-{environment}-{purpose}'
        receipt = json.loads(vault('add', name, '--description',
                                  'Pinelink operator-managed; private runtime credentials, never public CI or Git',
                                  '--value-file', '-', '--json', value=json.dumps(value).encode()))
        reference = receipt.get('reference')
        if not reference:
            raise ProvisionError('Vault write did not return a committed secret reference')
        record[key] = reference
        save()
        return reference

    for environment, host in [('staging', args.staging_host), ('production', args.production_host)]:
        if host != host.lower() or not host.endswith('.' + args.zone) or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-.' for c in host):
            raise ProvisionError('Host must be a lower-case name inside the authorized zone')
        record = state['environments'].setdefault(environment, {})
        if record.get('host', host) != host:
            raise ProvisionError('Refusing to redirect an existing environment')
        record['host'] = host
        record['namespace'] = f'pinelink-{environment}'
        bucket = f'pinelink-{environment}'
        existing = api('GET', f'/accounts/{account}/r2/buckets/{bucket}', allow_missing=True)
        if existing and record.get('bucket') != bucket:
            raise ProvisionError('Bucket exists without an ownership record; inspect before adopting')
        if not existing:
            created = api('POST', f'/accounts/{account}/r2/buckets', {'name': bucket, 'locationHint': 'enam'})
            record.update(bucket=bucket, bucket_created_at=created.get('creation_date'))
            save()
        public = api('GET', f'/accounts/{account}/r2/buckets/{bucket}/domains/managed')
        if public.get('enabled'):
            api('PUT', f'/accounts/{account}/r2/buckets/{bucket}/domains/managed', {'enabled':False})
        custom = api('GET', f'/accounts/{account}/r2/buckets/{bucket}/domains/custom')
        if any(d.get('enabled') for d in custom.get('domains', [])):
            raise ProvisionError('Unexpected public custom domain on a private bucket')

        if 'r2_reference' not in record:
            if 'r2_token_id' in record:
                raise ProvisionError('Unfinished R2 credential handoff; inspect vault/state before rotation')
            child = api('POST', f'/accounts/{account}/tokens', {
                'name': f'pinelink-{environment}-objects',
                'policies': [{'effect': 'allow',
                              'resources': {f'com.cloudflare.edge.r2.bucket.{account}_default_{bucket}': '*'},
                              'permission_groups': [{'id': bucket_group}]}],
            })
            record['r2_token_id'] = child['id']
            save()
            store_credentials(environment, 'r2', {
                'endpoint': account + '.r2.cloudflarestorage.com', 'bucket': bucket,
                'access_key': child['id'], 'secret_key': hashlib.sha256(child['value'].encode()).hexdigest(),
            })
            del child

        if 'tunnel_id' not in record:
            tunnel = api('POST', f'/accounts/{account}/cfd_tunnel',
                         {'name': f'pinelink-{environment}', 'config_src': 'cloudflare'})
            record['tunnel_id'] = tunnel['id']
            save()
        tunnel_id = record['tunnel_id']
        api('PUT', f'/accounts/{account}/cfd_tunnel/{tunnel_id}/configurations', {'config': {'ingress': [
            {'hostname': host, 'service': f'https://pinelink-web.{record["namespace"]}.svc.cluster.local:8080',
             'originRequest': {'httpHostHeader': host, 'originServerName': f'pinelink-web.{record["namespace"]}.svc.cluster.local', 'caPool': '/etc/pinelink/ca/ca.crt'}}, {'service': 'http_status:404'}]}})
        if 'tunnel_reference' not in record:
            store_credentials(environment, 'tunnel', {'token': api('GET', f'/accounts/{account}/cfd_tunnel/{tunnel_id}/token')})

        if 'edge_reference' not in record:
            password = secrets.token_urlsafe(32)
            password_hash = subprocess.run([args.openssl, 'passwd', '-6', '-stdin'], input=password.encode(), capture_output=True)
            if password_hash.returncode: raise ProvisionError('Operator password hashing failed')
            store_credentials(environment, 'edge', {'username': 'pinelink-operator', 'password': password,
                'htpasswd': 'pinelink-operator:' + password_hash.stdout.decode().strip() + '\n'})

        if 'app_reference' not in record:
            database_password = secrets.token_hex(32)
            nats_password = secrets.token_hex(32)
            store_credentials(environment, 'app', {
                'api': {'PINELINK_DATABASE_URL': f'postgres://pinelink:{database_password}@pinelink-postgres:5432/pinelink?sslmode=verify-full&sslrootcert=/etc/pinelink/ca/ca.crt',
                        'PINELINK_NATS_URL': f'tls://pinelink:{nats_password}@pinelink-nats:4222',
                        'PINELINK_HISTORY_SIGNING_SEED': base64.b64encode(secrets.token_bytes(32)).decode(),
                        'PINELINK_CORS_ORIGINS': 'https://' + host},
                'postgres': {'POSTGRES_USER': 'pinelink', 'POSTGRES_DB': 'pinelink', 'POSTGRES_PASSWORD': database_password},
                'nats': {'server.conf': f'port: 4222\nhttp_port: 8222\njetstream {{ store_dir: /data, max_memory_store: 32MB, max_file_store: 768MB }}\nauthorization {{ user: pinelink, password: "{nats_password}" }}\ntls {{ cert_file: "/etc/pinelink/tls/tls.crt", key_file: "/etc/pinelink/tls/tls.key" }}\n'},
            })

        # The chart requires this environment's auth Secret before the web can start.
        records = api('GET', f'/zones/{zone}/dns_records?' + urllib.parse.urlencode({'name': host}))
        desired = {'type': 'CNAME', 'name': host, 'content': tunnel_id + '.cfargotunnel.com',
                   'proxied': True, 'ttl': 1, 'comment': f'Pinelink {environment}; operator managed'}
        if records:
            if len(records) != 1 or records[0]['id'] != record.get('dns_record_id'):
                raise ProvisionError('DNS exists without matching ownership; no overwrite performed')
            api('PUT', f'/zones/{zone}/dns_records/{record["dns_record_id"]}', desired)
        else:
            record['dns_record_id'] = api('POST', f'/zones/{zone}/dns_records', desired)['id']
        record['provisioned'] = True
        save()
        print(json.dumps({'environment': environment, 'host': host, 'private_bucket': True,
                          'access': 'origin-basic-auth', 'secrets_persisted_to_vault': True}))


if __name__ == '__main__':
    try:
        main()
    except ProvisionError as error:
        raise SystemExit(str(error))
