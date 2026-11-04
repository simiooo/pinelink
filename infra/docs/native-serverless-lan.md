# Offline local transfer

Pinelink Local uses the same signed connection and file protocol in the Android
app and Web/PWA. Internet, an account, centre signalling, DNS, STUN, TURN and cloud
storage are not required. Both devices must be reachable on their local network.

## Connect

- Android: open **Add device → Nearby**. Select the other device and compare the
  six digits on both screens. Confirm on both devices. OS NSD/Bonjour discovers
  `_pinelink-rtc._tcp`; discovery alone never establishes trust.
- Web/PWA: use **Add device → Local**, exchange the QR or connection packages,
  compare both screens and confirm on both. The browser does not expose a general
  LAN service browser. A connection package is the usable discovery alternative.
- If multicast or the camera is unavailable, use **Code** and import/paste the
  offer, then return the reply. A package expires after two minutes and cannot
  authorize another session. Changing a trusted peer's signing or encryption key
  requires removing that peer and pairing again.

Wi-Fi client isolation and guest-network isolation can prevent devices from
reaching each other. Use a network or hotspot that allows local peers. Local
mode reports a lost connection; it never silently sends the file through cloud.

## Start without internet

The installed Android APK contains its fonts, cryptography and camera barcode
model. Its device graph opens without waiting for an account or centre request.
The local device identity is stored separately from account identities.

An installed/cached PWA can reopen offline. A browser that has never downloaded
the application cannot fetch it from an unreachable website. For a first offline
desktop start, unpack the matching `pinelink-offline-desktop` release and run
`Start.cmd` or `Start.command`; this serves the app at a stable localhost origin.
It starts no cloud API. Keep its port and browser profile stable to retain keys
and history. See [Web offline guide](web-serverless-lan.md).

## Transfer and recovery

Files require explicit acceptance. No file manifest or data is sent before that
acceptance. This preview supports files up to 64 MiB, two active transfers and a
256 MiB local storage budget. Text transfer and the native online cloud path are
separate deliveries; this implementation does not claim them as complete.

The blue portion of the line represents the receiver's signed, durable byte
prefix. A running transfer shows a subtle sheen; paused or disconnected
transfers stop it. Completion requires the full ciphertext hash, authenticated
decryption, plaintext hash and the receiver's signed receipt. Details remain in
the secondary sheet. Received files can be saved through the system file picker.

Keep the app open during transfer. Mobile operating systems may suspend a
background app. SQLite WAL/FULL transactions preserve accepted bytes and history
across closure. Reconnect to the known device and resume; the ciphertext and
verified prefix are reused. Up to four unacknowledged 64 KiB chunks may be resent.
Expired offers, rejection and cancellation remove temporary data.

Renaming a device changes only its local alias. Removing it closes matching
sessions and revokes the local pin; removal is blocked while a transfer still
needs a terminal acknowledgement. A late pairing callback cannot restore the pin.

## Implementation

- Reliable, ordered WebRTC DataChannel, no ICE servers, data-only SDP. The actual
  selected candidate pair must be host/prflx before pairing becomes available.
- Ed25519-signed packages bind both public keys, the SDP fingerprint and fresh
  nonces. The comparison binds both complete transcripts. Controls carry a
  session transcript and strictly increasing sequence number.
- AGE X25519 envelope and DARE 2.0 file encryption interoperate with the existing
  Web implementation. Native Ed25519 uses bundled libsodium through `sodium`;
  no runtime library download or crypto algorithm fork is used.
- Drift/SQLite stores the local identity profile, full peer pins, aliases, jobs,
  chunks and incremental journal. Chunk batches, acknowledgement, receipt,
  plaintext and corresponding journal records commit atomically as appropriate.
  Leases, control versions and terminal-state checks fence stale writers.
- Local journal records remain local with `uploaded=0`. This unit does not enable
  their centre backfill or merge local trust into an account. Existing account
  history replication remains separate.
- The CLI's older TLS LAN transport has a different discovery service and file
  protocol. CLI ↔ Web/Flutter transfer is not advertised as interoperable.

Android uses `top.sqmem.pinelink`; debug builds use a `.debug` suffix. Release
signing comes from private environment configuration. No signing credential is
required in this public repository. The Android release certificate SHA-256 is
`7a14d69aba4c33fdf996f9b4096052f26640a7a56c358f288563598c03d147f8`.
Release key updates must preserve that certificate. The iOS shared implementation
includes local-network, Bonjour and camera declarations, but this delivery does
not distribute an IPA or claim a native iOS runtime check.

Design source: [OpenPencil v35](../../designs/35-pinelink-native-local.fig).
Protocol and persistence tests are in `mobile/test/local_*`; the real Android and
Chromium runner is `web/e2e/native-local.mjs`. Its `test_support` loopback bridge
is compiled only for the private test entry point and is absent from the release
application entry point.
