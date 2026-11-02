"""Stream vault values to namespaced Secrets; require cluster encryption first."""
import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--state', type=Path, required=True)
    parser.add_argument('--environment', choices=['staging', 'production'], required=True)
    parser.add_argument('--ssh-host', required=True)
    parser.add_argument('--openssl', default=shutil.which('openssl') or str(Path(os.environ.get('ProgramFiles', '/nonexistent')) / 'Git/usr/bin/openssl.exe'))
    args = parser.parse_args()
    state = json.loads(args.state.read_text(encoding='utf-8-sig'))
    record = state['environments'][args.environment]
    namespace = 'pinelink-' + args.environment
    if record.get('namespace') != namespace or not record.get('provisioned'):
        raise SystemExit('Environment is not fully provisioned')
    ssh = ['ssh', '-o', 'BatchMode=yes', args.ssh_host]
    status = subprocess.run(ssh + ['k3s secrets-encrypt status'], capture_output=True, text=True)
    if status.returncode or 'Encryption Status: Enabled' not in status.stdout:
        raise SystemExit('Cluster Secret encryption is not enabled; no credentials were sent')
    result = subprocess.run(ssh + ['k3s kubectl get namespace ' + namespace + ' -o json'], capture_output=True)
    if result.returncode:
        raise SystemExit('Namespace unavailable')
    labels = json.loads(result.stdout)['metadata'].get('labels', {})
    if labels.get('app.kubernetes.io/part-of') != 'pinelink' or labels.get('pinelink.environment') != args.environment:
        raise SystemExit('Namespace ownership check failed')

    def resolve(purpose):
        reference = record[purpose + '_reference']
        response = subprocess.run(['nuto', '--env', 'staging', '--project', 'Pinelink', 'secret', 'resolve', reference, '--raw'], capture_output=True)
        if response.returncode:
            raise SystemExit('Vault read failed or needs authorization; no value logged')
        return json.loads(response.stdout)

    app, r2, tunnel, edge = resolve('app'), resolve('r2'), resolve('tunnel'), resolve('edge')
    # Only the public Ed25519 key enters the browser's runtime ConfigMap. The
    # private seed stays in the operator's memory and the API runtime Secret.
    seed = base64.b64decode(app['api']['PINELINK_HISTORY_SIGNING_SEED'], validate=True)
    if len(seed) != 32:
        raise SystemExit('Invalid history signing seed')
    public = subprocess.run([args.openssl, 'pkey', '-inform', 'DER', '-pubout', '-outform', 'DER'],
                            input=bytes.fromhex('302e020100300506032b657004220420') + seed, capture_output=True)
    prefix = bytes.fromhex('302a300506032b6570032100')
    if public.returncode or len(public.stdout) != 44 or not public.stdout.startswith(prefix):
        raise SystemExit('Public profile derivation failed; no key values logged')
    profile = {'version': 1, 'api_url': '/api', 'history_public_key': base64.b64encode(public.stdout[len(prefix):]).decode()}
    public_config = {'apiVersion': 'v1', 'kind': 'ConfigMap',
                     'metadata': {'name': 'pinelink-service-profile', 'namespace': namespace,
                                  'labels': {'app.kubernetes.io/part-of': 'pinelink', 'pinelink.environment': args.environment}},
                     'data': {'service-config.json': json.dumps(profile)}}
    applied = subprocess.run(ssh + ['k3s kubectl apply --server-side --field-manager=pinelink-ops -n ' + namespace + ' -f -'],
                             input=json.dumps(public_config).encode(), capture_output=True)
    if applied.returncode:
        raise SystemExit('Public profile apply failed; no key values logged')
    print(json.dumps({'namespace': namespace, 'public_profile': 'pinelink-service-profile', 'applied': True}))
    api = dict(app['api'])
    api.update(PINELINK_S3_ENDPOINT=r2['endpoint'], PINELINK_S3_BUCKET=r2['bucket'],
               PINELINK_S3_ACCESS_KEY=r2['access_key'], PINELINK_S3_SECRET_KEY=r2['secret_key'])
    for component, values in [('api', api), ('postgres', app['postgres']), ('nats', app['nats']), ('tunnel', tunnel), ('edge', {'htpasswd': edge['htpasswd']})]:
        document = {'apiVersion': 'v1', 'kind': 'Secret', 'type': 'Opaque',
                    'metadata': {'name': 'pinelink-' + component, 'namespace': namespace,
                                 'labels': {'app.kubernetes.io/part-of': 'pinelink', 'pinelink.environment': args.environment}},
                    'data': {key: base64.b64encode(value.encode()).decode() for key, value in values.items()}}
        response = subprocess.run(ssh + ['k3s kubectl apply --server-side --field-manager=pinelink-ops -n ' + namespace + ' -f -'],
                                  input=json.dumps(document).encode(), capture_output=True)
        if response.returncode:
            # Do not forward provider error bodies that may echo Secret data.
            raise SystemExit('Namespaced Secret apply failed; values were not logged')
        print(json.dumps({'namespace': namespace, 'secret': 'pinelink-' + component, 'applied': True}))


if __name__ == '__main__':
    main()
