# Browser direct/fallback verification

Build and run on a Linux Docker host. The ordinary Web test/build commands remain unchanged. Deep network checks use two independent browser processes and kernel network namespaces; the driver records selected ICE candidate types, DataChannel byte counters, file SHA-256 and delivery receipts. Host ICE uses the browser's normal mDNS/privacy policy. No fake DataChannel or disabled mDNS flag is used for a successful-direct assertion.

Build `web/dist` with the runtime service profile, and install the pinned project dependencies. Build a test image using the Playwright version from `web/package-lock.json`:

```sh
PW_VERSION=$(node -p "require('./web/package-lock.json').packages['node_modules/playwright-core'].version")
docker build -f infra/containers/Dockerfile.browser-network-tests \
  --build-arg PLAYWRIGHT_VERSION="$PW_VERSION" -t pinelink-browser-tests .
```

Run `python3 infra/scripts/test-web-direct.py --help`. Supply the checkout/evidence paths, a compatible browser cache mounted read-only, the test image, and the pinned Linux Go SDK/cache paths for the fixture. An evidence directory must be inside this checkout's `.artifacts`. The browser image's `/ms-playwright` can be copied into a task cache; it is never an application asset. `--browser-b firefox` exercises Chromium-to-Firefox. `--origin https://staging-pinelink.sqmem.top` uses the real service instead of the isolated PostgreSQL/blob fixture.

The runner creates uniquely labelled containers and one dedicated bridge, publishes no host port, and removes only its own resources. `NET_ADMIN` is granted only in the two disposable peer namespaces. The controller requests exact peer-address rules there; it retains the HTTPS/API route and never changes the host firewall. The source, SDK and browser cache are read-only in the browsers. The test-only bridge forwards the fixture's API and a small allowlist of test controls; it never forwards mail.

Cases cover a 16 MiB direct file and an empty frame with no spool/blob I/O, withheld recipient keys and data before consent, closing the recipient before acceptance, a blocked peer network, a network cut after a committed direct checkpoint, pause/reload/cloud resume with sender exit, and cancellation without a receipt. Cloud startup deliberately delays route initialization to catch frames arriving before listeners. Screenshots and per-transfer IDs stay in ignored evidence directories. Real staging additionally needs exact-transfer database checks and actual R2 read/write evidence; a memory blob fixture is not evidence of R2 delivery.

The browser needs reachable pairing, signalling and progress services. These tests do not claim fully disconnected browser discovery or background transfer after both browsers exit. Successful direct files need no R2 object, while a cloud fallback keeps the original encrypted attempt and recipient ledger. Existing guest limits reserve fallback capacity before an offer; a LAN route does not bypass identity, pair, quota, consent or revocation checks.
