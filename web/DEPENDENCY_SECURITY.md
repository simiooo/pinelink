# Web build and test dependencies

Use Node 24, as CI and the isolated build environment do. Vitest 5 requires Node 22.12+ on its supported release lines and Vite 6.4+; the repository retains Vite 6.4.5 and upgrades Vitest to 5.1.0. The tests use its maintained worker implementation instead of the old Tinypool chain. `defineConfig` comes from `vitest/config` so test options use the matching types.

The PWA remains on `vite-plugin-pwa` 2 and Workbox 7.4.1. Two overrides are scoped to `workbox-build`:

- `glob` 14.0.3 fixes cancellation and matching denial-of-service issues. Workbox uses the retained `globSync(pattern, { cwd, follow, ignore })` API.
- `@trickfilm400/rollup-plugin-off-main-thread` 4.0.0 uses the upstream EJS 7 line and removes the old EJS/Jake/Filelist/Minimatch dependency chain. This is an upstream plugin upgrade, not a forced replacement of EJS alone.

Validate the existing test suite, production build, generated precache assets, and real installed-PWA offline cold start/reconnect after changing these overrides. The deployment serves static production files; do not expose Vite/Vitest development servers as the application service.

## Residual advisory

As of 2026-11-03 the complete dependency audit reports four **low** entries, all propagation of one `deepmerge` advisory through `@rollup/plugin-node-resolve` → `workbox-build` → `vite-plugin-pwa`. The upstream advisory has no patched release, and the latest resolver still depends on the affected version. They are build-only dependencies; application inputs do not become build configuration. Run builds in an isolated environment with trusted source/configuration. Do not claim the complete dependency graph is vulnerability-free or downgrade the PWA plugin to an obsolete release solely to hide the warning.

`npm run audit:ci` reports the full graph and fails for moderate or higher findings; low entries remain visible. `npm run audit:production` fails on any production-dependency finding. CI executes both. Recheck the upstream resolver/deepmerge fix and remove the residual advisory once a compatible patched version is verified. Dependency audit results are time-specific and do not prove absence of all vulnerabilities.

References: [Vitest 5 migration](https://vitest.dev/guide/migration/), [Workbox glob call](https://github.com/GoogleChrome/workbox/blob/v7.4.1/packages/workbox-build/src/lib/get-file-details.ts), [Glob 14 API](https://github.com/isaacs/node-glob/tree/v14.0.3), [deepmerge advisory](https://github.com/advisories/GHSA-gcqx-w48m-w2h4).
