# Private deployments from a public repository

The public repository contains application code, Dockerfiles, a Helm chart and operator scripts. It contains no live Cloudflare account IDs, credentials, kubeconfigs, private keys, runtime Secret manifests or private deployment state. The images contain the application and public dependencies; secrets enter at runtime.

The official environment names are `staging-pinelink.sqmem.top` and `pinelink.sqmem.top`. Use separate namespaces, buckets, application credentials, history-signing keys, internal certificate authorities, tunnels and operator passwords. Example namespace names are `pinelink-staging` and `pinelink-production`.

## Traffic and storage

DNS is a proxied CNAME to an environment-specific Cloudflare Tunnel. Each connector reaches only its namespace's internal Web service; the chart creates ClusterIP services and no public Ingress, NodePort, host network or host ports. This prevents a direct origin HTTP route from bypassing the edge. The legacy operator-password gate is controlled by `previewAccess.enabled` and remains enabled by default. After account sessions and signed devices are enforced, turn it off to make anonymous mode and account pages directly accessible. Staging uses this application identity flow; older environments retain their existing gate until explicitly migrated.

The central relay uses binary WebSocket frames, which Cloudflare can proxy. Native direct peer-to-peer and fully offline LAN transfer remain separate paths. The WebSocket relay reuses the existing fenced encrypted spool, quota, grant, progress, checksum and receipt logic. Upload completion means stored ciphertext; receiver verification and the receipt still determine delivery. Set `PINELINK_RELAY_TRANSPORT=websocket` at the center; existing direct deployments default to `webrtc`. Older clients need an update to use a center configured for the WebSocket relay. Edge deployments may interrupt sockets; clients can reopen using durable checkpoints.

Internal traffic also uses TLS, including when cluster nodes communicate across public networks. Namespace-scoped cert-manager issuers provide separate Web/API/PostgreSQL/NATS certificates. Clients verify the CA and service name. PostgreSQL rejects non-TLS TCP connections; NATS authenticates over TLS. Projected Secret rotation reloads Web/PostgreSQL/NATS, while the API loads its projected certificate on each TLS handshake. The connector verifies the internal Web certificate. Application pods do not receive CA private keys.

The database-backed outbox survives broker outages, and its NATS connection keeps reconnecting until shutdown. The API health check covers the primary database; Helm also waits for PostgreSQL, NATS, Web and the connector individually. Use `nats-server` from the pinned Alpine image's PATH when wrapping its process for certificate reload. Bound JetStream to 32 MB of memory and 768 MB of file storage within its 128 MiB container and 1 GiB volume; the Go soft memory limit is 96 MiB. Adjust these together when changing capacity.

R2 buckets stay private. A bucket-scoped Object Read & Write credential is sufficient for the application; the Cloudflare administrative token stays with the operator. Set `PINELINK_S3_USE_SSL=true` and `PINELINK_S3_REGION=auto`. The application performs resource-aware expiry and garbage collection. Do not add a bucket-wide time-to-live that can delete an object still referenced by Keep or Restore grants.

## Runtime credentials

The chart accepts existing Secret names and does not render Secret values. It uses separate Secrets for API, PostgreSQL, NATS and connector; the optional preview gate mounts its operator-password hash only while enabled. Pods have no Kubernetes API token, run without root or added capabilities, and use read-only root filesystems. Default-deny policies permit only the required service edges, DNS and public TLS egress. Stateful volumes are retained when StatefulSets are removed or scaled down. Namespace deletion remains destructive; it is not part of the deployment script.

Kubernetes Secret base64 is not encryption. Enable and verify datastore Secret encryption before injection. On an existing shared k3s cluster, that is a separate control-plane change requiring its owner's approval and a consistent encrypted backup. The operator script refuses to send values while encryption is disabled. Public CI has no cluster/vault/Cloudflare credentials and is not allowed to deploy an untrusted pull request.

The operator commands use Nuto's staging vault as configured for this workspace. `resolve --raw` is captured by the consuming process, never printed. Values are streamed to Kubernetes with server-side apply, without a last-applied annotation. Helm state contains references, not credential values. Keep the metadata-only operator state outside Git or in the verified ignored `.stage/` directory. Keep backups encrypted, including the matching datastore, server token and encryption configuration. Never discard encryption keys while encrypted records still exist.

The operator derives the history verification public key in memory and writes only that public key to a namespaced runtime ConfigMap. Cloud Web builds require this same-origin service profile. Foreground tabs and background workers atomically retain the first trusted key in IndexedDB, so offline startup does not need configuration access and a changed key cannot silently replace the pin. API requests include same-origin ingress credentials; cross-origin credentials are omitted. This supplies the private preview's verification configuration without embedding any runtime secret in a build. The consumer identity and advanced service-switching UI remain separate work.

```powershell
python infra/scripts/provision-cloudflare.py --token-reference 'secret:<operator-reference>' --zone example.com --staging-host staging-transfer.example.com --production-host transfer.example.com --state .stage/cloudflare-state.json
# Inspect the preflight, then add --apply to provision.

python infra/scripts/inject-k3s-secrets.py --state .stage/cloudflare-state.json --environment staging --ssh-host <private-ssh-alias>
helm upgrade --install pinelink infra/helm/pinelink --namespace pinelink-staging --set environment=staging --set version=<source-sha> --set images.api=<immutable-api-image> --set images.web=<immutable-web-image>
```

Build from a clean source commit and record its full SHA. Use immutable image digests or distribute the exact, verified image archive to each scheduled node with `imagePullPolicy=Never`; do not give public pull-request jobs a private registry or cluster credential. Inject no runtime credential through Docker build arguments, `VITE_*`, Dart defines, generated JavaScript or release attachments. The Web image currently uses a developer-preview profile, not the finished official consumer identity flow.

## Acceptance

Verify both namespaces and bucket credentials are isolated; the staging key must fail against production storage. Password-gated previews reject absent and cross-environment operator credentials. With the gate disabled, a fresh browser must reach guest mode and account pages without HTTP credentials, while protected API data still requires an authorized account or guest device. Retain no-store responses, CSRF, Secure/HttpOnly cookies, valid public TLS and protection against direct origin access. Inspect actual source versions, image digests and CA validation. Scan proposed Git changes, runtime configuration and operator logs for live credentials without printing matching values.

Run real encrypted direct/relay/fallback/preupload/restore transfers, an interrupted receiver and sender, private-object cleanup, signed receipts, and Web/PWA persistence and reconnection checks. Test pod restarts and certificate renewal. A single-replica PostgreSQL and local-path volume are not a high-availability database; encrypted backup/restore and storage placement remain operational requirements before general production access.

References: [Cloudflare WebSockets](https://developers.cloudflare.com/network/websockets/), [R2 authentication and bucket-scoped credentials](https://developers.cloudflare.com/r2/api/tokens/), [k3s Secret encryption](https://docs.k3s.io/security/secrets-encryption), [k3s configuration](https://docs.k3s.io/installation/configuration).
