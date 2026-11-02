# Staging deployment

Staging uses its own database, object store, NATS data and receiver state. Database/S3/NATS ports stay on loopback, and API requests require device signatures. Existing persistent data must be preserved during deploys and rollbacks.

PowerShell 7 examples for a Linux Docker host:

```powershell
./infra/scripts/deploy-staging.ps1 -Version (git rev-parse HEAD) -Context staging -ApiUrl http://staging-host:18680
./infra/scripts/deploy-local-peer.ps1 -Version (git rev-parse HEAD) -Context staging -PeerHost STAGING_IP
./infra/scripts/deploy-clients.ps1 -Version (git rev-parse HEAD) -Context staging -HostAddress STAGING_IP
./infra/scripts/smoke-staging.ps1 -Version (git rev-parse HEAD) -SshHost staging-host -ApiUrl http://staging-host:18680
```

Replace host/context values with your own environment. Client self-signed test certificates use an IP SAN; `HostAddress` is an IP address. The scripts create `.stage/staging.env` for private persistent credentials and record source versions and hashes in `.stage/<revision>`; never commit these directories or print credential files.

Browser smoke helpers accept `PINELINK_STAGING_HOST` and `PINELINK_DOCKER_CONTEXT`; set `CHROMIUM_EXECUTABLE` if Chrome is not at the Windows default path. `preview-pwa-staging.mjs` verifies the test server's certificate with a per-process CA and binds only loopback; it does not alter system trust or provide production HTTPS.

The local receiver advertises its own HTTPS endpoint and pins complete device keys during pairing. Connection codes are short-lived secrets. Offline smoke tests use isolated, temporary Docker networks and receiver state; they do not change the host's network or existing application volumes.

```powershell
python ./infra/scripts/smoke-local-offline.py --context staging --version (git rev-parse HEAD) --output .artifacts/offline.json
```

Use only the deployment's named containers and images. Do not run `down -v` or broad prune commands. This is a developer staging environment; public production and iOS distribution are separate releases.
