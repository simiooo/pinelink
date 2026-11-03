# Guest transfers

The Web client can link two devices and transfer a real file before either
person creates an account. Enable the service explicitly; public builds do not
contain a shared guest owner, session credential or private server setting.

## Runtime policy

Add these variables to the external runtime Secret described in [accounts.md](accounts.md).
Keep the existing account encryption key stable. The account origin, trusted
proxy boundary and signed-device checks remain enabled.

| Variable | Default when enabled |
| --- | --- |
| `PINELINK_GUESTS_ENABLED` | `false`; set `true` to enable |
| `PINELINK_GUEST_MAX_FILE_BYTES` | 67,108,864 plaintext bytes |
| `PINELINK_GUEST_DAILY_BYTES` | 268,435,456 reserved bytes per guest and UTC day |
| `PINELINK_GUEST_IP_DAILY_BYTES` | 536,870,912 reserved bytes per trusted edge IP and UTC day |
| `PINELINK_GUEST_GLOBAL_DAILY_BYTES` | 8,589,934,592 reserved bytes per UTC day |

Reservations include encrypted content and metadata, with a minimum 64 KiB
charge per file. Both guests are charged; cancellation does not refund the
daily reservation. At most two live transfers may involve a guest. The IP is
an additional abuse limit, never an identity. The service also bounds challenge,
identity, invite and join creation. Use the edge's overwritten client IP header
only from configured trusted proxies. IPv6 resource buckets use a /64 prefix.

Migration 020 adds separate guest identities, leases, candidates, exact-device
pair grants, resource counters and one-use browser relay tickets. Apply through
the normal migration runner after verifying an encrypted database backup.
Account sessions and their foreign keys are not reused for guest sessions.

## Identity and consent

Opening a new Web client generates local, non-extractable signing and encryption
keys. A network identity is enrolled only after an explicit connection action.
Enrollment uses a one-minute, signed body-bound challenge. Concurrent retries
preserve the current valid lease; an expired lease is replaced rather than
revived. Device revocation remains permanent. A lease lasts 24 hours and can be
renewed using the original local keys.

A 128-bit invitation lives in a scrubbed URL fragment; an eight-symbol code
supports manual entry. Both expire after ten minutes. Reading or previewing a
link does not consume it. Joining proposes a candidate. Only the invitation
owner's explicit confirmation atomically consumes it and grants access to the
two specified devices for 24 hours. Every file still needs its own Receive.
Knowing an owner, device or transfer ID does not grant access.

Guest requests retain device signatures and use a distinct Secure, HttpOnly,
SameSite=Strict cookie with origin and CSRF checks. Native session headers and
the React/Flutter cookie slots remain separate. A relay WebSocket uses a
30-second, single-use subprotocol ticket bound to role, transfer and the same
live cookie session; credentials never appear in the socket URL. Active I/O and
durable transactions retain typed session and device-revocation fences.

## File lifecycle and local continuity

The Web implementation uses the existing AGE/X25519 sealed manifest and DARE
2.0 encrypted chunks. It currently uses the cloud relay; WebRTC direct transfer
is not included. Upload may finish before recipient consent. A cloud node and
its blue boundary reflect durable upload, while the recipient's separate
boundary reflects acknowledged download. Upload completion is not delivery.
The normal pending-transfer deadline, rejection, resource references and expiry
garbage collection remain in force.

Encrypted attempts are persisted before creating offers. Received chunks are
committed locally before acknowledgements. Full ciphertext, authenticated
decryption and plaintext hash are verified before persisting the local file and
committing a delivery receipt. Pause and refresh preserve the acknowledged
boundary; resume may replay stored chunks without rewinding the server ledger.
The decorative sheen stops while paused, hidden or under reduced motion.

Sign-in during a transfer uses a separate window where supported. Account
changes cannot replace or close the guest runtime, keys or in-flight owner.
After sign-in, **On this device** opens the original local guest history. The
PWA can reopen that history and save verified local files fully offline. It
does not continue network work after the browser closes. A browser that cannot
persist storage shows a retry state rather than claiming a durable save.

Automatic migration into an account, longer cloud retention, memory-only guest
fallback, new offline send intentions and the Flutter guest file UI are separate
follow-ups. The native CLI's existing offline LAN service is unchanged. Do not
describe this release as completing every client or every transfer route.

## Verification

`backend/internal/accounts` and `backend/internal/service` use the isolated
PostgreSQL URL `PINELINK_SERVICE_TEST_DATABASE_URL` and create their own schemas.
Guest tests cover concurrent enrollment, expiry, exact pairing competition,
typed relay tickets, atomic budgets and revocation fences.

`backend/cmd/accountfixture` is a loopback-only browser fixture with an isolated
schema and in-memory blob/mail adapters. It is absent from the production
image. Build the Web app with `VITE_PINELINK_API_URL=/api` and
`VITE_PINELINK_SERVICE_PROFILE=runtime`, start the fixture, then run
`web/e2e/guests.mjs` with matching Playwright Chromium and a UTF-8 locale. The
suite checks two independent guest contexts, real encrypted files, zero-byte
files, 32 MiB pause/reload/resume with the sender closed, signup continuity,
post-login local history, access denial and offline PWA reopening. Separately
verify the actual deployed R2 path; the fixture is not evidence of R2 delivery.
