"""Stream vault values to namespaced Secrets; require cluster encryption first."""
import argparse
import base64
import json
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--state', type=Path, required=True)
    parser.add_argument('--environment', choices=['staging', 'production'], required=True)
    parser.add_argument('--ssh-host', required=True)
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
