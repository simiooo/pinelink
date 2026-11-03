# Pinelink mobile

The Flutter client targets Android and iOS. The independent Web/PWA client lives
in `web/` and continues its normal build, deployment and release process.

## Release targets

- Android: deliver an installable, release-signed APK through GitHub Releases,
  with its source revision, version and SHA-256 checksum.
- iOS: retain development support; do not distribute an IPA, TestFlight build or
  App Store release until the user changes this publishing boundary.
- Flutter Web may support compatibility checks and previews. Those results do
  not replace Android installation, native behavior or APK delivery.

This release policy was confirmed by the user on 2026-11-03. Existing published
Flutter Web previews remain historical artifacts.

## Native delivery

Use the verified stable SDK pinned in `.fvmrc`; build and test on the Fedora
builder. Before the first Android release, finish the stable application ID and
release signing setup. The current generated Android configuration still uses a
template application ID and a debug signing configuration.

Keep signing keys and passwords private. Validate the actual Android app against
staging, including account return, persistence, file selection/saving and Web
interoperability. SIM-936 tracks native guest pairing and real file transfer;
the Android APK and its GitHub Release are part of that delivery.
