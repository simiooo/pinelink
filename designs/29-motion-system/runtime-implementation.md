# v29 client implementation

The user adopted v29 on 2026-11-03, then requested editable shadcn components for the Web client. The approved OpenPencil appearance remains the visual contract. The design prototype and its historical evidence are preserved; they are not evidence of a live transfer.

This engineering unit delivers the shared motion foundations and bindings to the existing React and Flutter pages. [Runtime bindings](runtime-bindings.json) account for all 109 design targets. A target marked `feature-dependent` still needs its actual product feature; the foundation or prototype alone does not complete it. Account and cross-user/text designs retain their separate adoption and implementation boundaries.

| Layer | Implementation |
| --- | --- |
| Web primitives | Twelve official shadcn/Radix sources under `web/src/components/ui`, customized in the repository. Nunito, #0866FF, neutral surfaces, 48px targets, line tabs, semantic spacing, quiet focus and light shadows. MIT attribution is retained. |
| Web motion | `web/src/motion`: a shared WAAPI owner, analytic spring retargeting from the current position and velocity, keyed list measurements, async button gates, real progress masks, adaptive Dialog/Sheet presence and native drag targets. |
| Flutter motion | `mobile/lib/src/motion`: shared timing/spring tokens, state-driven buttons, feedback, progress painters, adaptive native routes, lifecycle/TickerMode and accessibility handling. Flutter stays native; it does not embed shadcn. |
| Existing bindings | Navigation/menu/language selection; preparation form and clipboard feedback; existing request/history/settings actions; real task curves and secondary task details. Unused Courier code is retained for historical tests but is no longer mounted by product pages. |
| Transfer authority | Confirmed bytes determine the blue boundary. Flow runs only during fresh running snapshots, stops during pause/offline/reduced motion, and never supplies a completion event. Full bytes wait for verification/receipt; upload completion means cloud storage. |
| Deferred features | Full device discovery graph and node action orbits, node-near drop arbitration, file-manager history grid, consumer account/service flows, full file data plane, peer trust and text sending require their respective feature units. Existing file drops enter preparation only; they do not claim upload or delivery. |

The source fixture `web/test/motion.html` runs the actual React components in StrictMode through Vite. It is excluded from the production entry and PWA cache. Browser checks cover interrupted presence/focus, keyboard menu/tooltip, mobile sheet drag, pending double clicks, current-pose reversal, real-byte masking, pause/late acknowledgement, live reduced-motion changes, offscreen loops, keyed drafts, nested drops, large lists with CPU throttling and complete owner cleanup. This is functional browser evidence, not a claim of native device frame rates.

Run on the build machine after installing the locked dependencies:

```sh
cd web
npm test -- --maxWorkers=2
npm run build
CHROMIUM_PATH=/usr/bin/chromium node test/motion-browser.mjs
npm run audit:ci
npm run audit:production
cd ../mobile
dart run tool/sync_i18n.dart
flutter analyze
flutter test --concurrency=2
cd ..
python3 infra/scripts/prepare-flutter-web.py
cd mobile
flutter build web --release --no-web-resources-cdn --no-wasm-dry-run
```

The CLI's transitive `minimatch` is narrowly overridden to patched 11.0.1. The production audit is clean; six low-severity, unpatched development-tool findings remain in the deepmerge chain. Do not downgrade the CLI with an unreviewed forced audit fix.

Web-only staging releases can set `webVersion` to the exact source commit while preserving the API/tunnel revision. The chart defaults to the existing shared `version` when it is absent. Runtime deployment and PWA smoke evidence are recorded separately after the candidate is built and deployed.

Flutter Web builds must prepare the pinned SQLite WASM and compile the Drift worker before the Flutter build, following [Drift's Web setup](https://drift.simonbinder.eu/platforms/web/). Release assembly rejects missing database assets. Browser validation caught an omission in the temporary packaging path before publication; the shared preparation script and CI build now cover it.

Flutter also bundles its existing Noto Sans SC fallback under OFL 1.1 so CJK menu labels and filenames do not initiate Google font downloads. Nunito remains primary, with Microsoft YaHei preferred where available. The English OpenPencil addendum `29-pinelink-offline-fonts.fig` records the unchanged typography contract; the CJK trial triggered the known OpenPencil memory error and was replaced with English labels as authorized. Native font availability is verified separately from browser offline glyph coverage.
