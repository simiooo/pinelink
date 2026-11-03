# Account entry and activation contract

Superseded, not adopted. The user requires Google AND password login, with no extra email entry in the standard Google flow. Use the [v27 contract](../27-account-password/account-contract.md); the passwordless-only proposal below is historical.

Status: design proposal, pending product acceptance and implementation. This document does not assert that account authentication or email delivery is deployed.

## Product flow

The device graph remains the starting point. Account entry lives in its More menu and appears when a cloud action needs authentication. Nearby offline transfers and cached local history do not require a cloud account.

Login and registration share one page. Google authorization creates an account on first successful sign-in and reuses it thereafter. Email uses a passwordless code or link; a new account is created only after proof of mailbox possession. Returning email users use the same flow to recover access. There is no password form or password-reset branch in this proposal.

After account and device enrollment commit, return directly to the device graph or the action the user started. Do not insert profile, nickname, server configuration, tutorial, payment, or success pages. An actual first transfer is the product outcome; visiting a verification page is not an activation success metric.

Keep the email and original destination after cancellation, resend, expiry, failure, or app restart. Destinations are bounded, validated internal intent records, not nested `next` URLs. Auth routes cannot become their own continuation. A changed account must re-authorize the original resource before any action resumes.

## Email and short links

An HTML email has one “Continue to Pinelink” button and an eight-digit code. The text alternative contains the same code and one short HTTPS URL. No public third-party URL shortener or click tracking is involved.

The URL shape is `https://pinelink.sqmem.top/a/<token>`; staging uses its own host. `<token>` is 16 cryptographically random bytes encoded as 22 unpadded base64url characters. It contains no email address, JWT, session, device key, return URL, or user record. The fixed trusted service origin supplies the host; never derive it from an untrusted Host header. The official production shape is 51 characters long. Tokens are looked up by a SHA-256 digest and purpose.

The code and link expire after 15 minutes. A challenge allows at most five wrong code attempts; a resend has a server-issued 60-second cooldown. Apply additional shared budgets by challenge, normalized address digest, and trusted source address across replicas. An eight-digit code is never a globally valid short URL or a standalone device-enrollment token. Protect its database digest with a separately stored, versioned server HMAC key.

GET, HEAD, prefetch, email security scanners, and service-worker requests never consume a challenge or create a session. The link opens an explicit Continue action; its POST performs confirmation with CSRF/origin protection. Strip the raw link from browser history promptly, set no-store/no-referrer, disable indexing and analytics on sensitive routes, and redact the token path in ingress/application logs. Loading an invalid or used link does not disclose the address or account status.

Code and link confirmation serialize against the same challenge. Successful consumption, account resolution, enrollment authorization and session creation are atomic. A resend, duplicate submit, other tab, callback, timeout, or server replica cannot create a second account or authorize a different key. A bounded idempotent retry is available only to the same initiating client with its original challenge secret and key proof; arbitrary replay is rejected.

Email confirmation on another browser must not silently sign in the browser that requested the email. Show “Continue on this device” and bind any new grant to that device's own key proof. To continue in the original app, enter the email code there. Never authorize a waiting requester merely because somebody opened its email link.

## Delivery truth and recovery

The client shows Sending while the server/provider outcome is pending, Code sent after provider acceptance, and a clear retry action after failure. A queue acknowledgement alone is not a sent-email result. The countdown comes from server `resend_at`, `expires_at`, and `retry_after`, survives reload, and cannot be bypassed by a second tab.

Mail I/O does not hold a database row lock. Persist a bounded delivery operation with an encrypted payload, a generation and a provider idempotency key. A failed send may restore a prior generation only with compare-and-swap; it cannot undo a later resend or completed verification. A worker checks consumption/expiry before sending. Do not log codes, links or plaintext mail payloads. Missing provider configuration fails closed and provides a recoverable UI state.

Opening the activation link is one optional confirmation page, followed by the graph. Code entry stays in the original app. Neither method adds a separate “Account activated” page.

## Identity, session and tenant boundaries

- `accounts` holds the stable owner identity; `account_identities` holds provider identities. Google identity is the verified issuer plus `sub`, not the contact email. Email identities are created only with a successful mailbox challenge.
- Do not merge accounts solely because their email strings match. Linking a new method requires a fresh proof of the existing account and the new method. If the identity belongs to another account, return a recoverable conflict; never move an identity or its files silently. The design includes both extra-proof variants.
- Do not create an active account or business access when an anonymous requester merely enters an email. This avoids preserving an attacker's pre-registered credentials when the mailbox owner arrives.
- Device enrollment requires the active account, a client PKCE challenge and signing-key proof; the server derives the owner. A client-supplied owner ID and a self-signed registration alone cannot create access to cloud data.
- Web account sessions use secure, HttpOnly, host-only cookies and CSRF protection. Native enrollment credentials stay in secure platform storage. Google client secrets and mail/HMAC secrets exist only in server-side private deployment configuration.
- Signed devices have bounded credential lifetimes. Cloud authorization, object grants, control requests and long-running relay operations enforce expiry/revocation, including fresh per-frame fences. Signing in successfully cannot revive a revoked old credential.
- Account and service origin identify a cloud storage/sync namespace. Guest LAN identity remains local. Switching accounts advances a client epoch, cancels prior queries/subscriptions/work, and prevents late responses or old outbox operations from entering the new namespace.
- Sign-out stops cloud work on this device and revokes the corresponding online session/device grant. An offline sign-out stops local workers immediately; the UI does not claim that unreachable server sessions have been revoked. Offline files and nearby-pair trust are not deleted as a side effect of session expiry.
- Preserve existing development data. Do not assign legacy device-owner rows to a new account by matching a label/email, and do not clear the database to enable authentication. Cutover and migration of test identities require an explicit, verified path.

## Proposed API surface

These routes are a contract for implementation, not currently available endpoints.

| Route | Purpose |
| --- | --- |
| `GET /v1/auth/capabilities` | Enabled methods and public service capability; no secrets |
| `POST /v1/auth/requests` | Bind a short-lived request to client mode, key proof, PKCE and safe continuation |
| `POST /v1/auth/email/start` | Start a mailbox challenge and bounded delivery operation |
| `POST /v1/auth/email/{id}/resend` | Atomically replace the challenge generation under shared budgets |
| `POST /v1/auth/email/{id}/verify` | Consume code and resolve identity for the initiating client |
| `GET /a/{token}` | Non-consuming link landing; no business authorization |
| `POST /v1/auth/email/confirm-link` | Explicit, CSRF-protected link consumption and current-device binding |
| `GET /v1/auth/google/start` | Start OIDC authorization-code flow with state, nonce and PKCE |
| Configured Google callback | Validate issuer, audience, nonce, expiry and subject; resolve once |
| `POST /v1/auth/requests/{id}/claim` | Claim enrollment using the original verifier and device signature |
| `GET /v1/auth/session` | Current account and verified enrollment state |
| `POST /v1/auth/identities/link` | Fresh-proof linking; no email-only merging |
| `POST /v1/auth/logout` | Revoke the current session/device grant idempotently |

Unauthorized or non-existent email addresses get equivalent start responses and timing. Display specific recovery only after the requester has appropriate proof. Rate-limit results include server retry times; provider/network errors do not reveal account existence.

Google callback locations remain exactly those registered for each environment. The callback and short-link routes bypass PWA navigation caching and ordinary body/query logging. Their responses are not put into an offline outbox. Cloudflare proxy, k3s ingress and Web routing must preserve these rules.

## Client interaction and motion

Use a single eight-digit input with numeric keyboard, whole-code paste, autofill and a real field label; visual spaces do not become part of the wire value. Submit on Continue or Enter, once per user action. No sending, resending, consuming or linking side effects run from a React mount effect. Server idempotency remains necessary even with disabled controls.

Related labels/inputs use 8px, fields/actions 16–24px, and functional groups 24–32px. Text-heavy form areas are left aligned. All actions have at least 48px targets; focus is visible and error text is announced. At 320px and with the keyboard open, the field and Continue action remain reachable by scrolling without losing input.

Page entry: opacity 0→1 and Y 8→0 over 180ms ease-out. Exit: 120ms ease-in. Replacing a status/error region uses a 160ms height/opacity transition without moving keyboard focus. Activity indicators run only while a real request is in flight; they do not imply completion. With reduced motion, switch states immediately and retain a static busy indicator with accessible text. Successful navigation waits for server-confirmed account and device enrollment.

## Delivery conditions

Product approval of the native OpenPencil account draft precedes UI implementation. Then complete API/database/mail adapters, Web and Flutter, credential/tenant isolation, and environment configuration as one deliverable.

Fedora checks must include simultaneous first sign-in, link/code replay, wrong/expired/rate-limited codes, resend-versus-verify and delayed-mail races, scanner GET safety, cross-device login CSRF, identity collision/linking, logout/expiry versus a live transfer, React StrictMode and account-switch late responses. Verify offline cold start and reconnect sync without cached auth successes. Send a controlled staging email and complete a real Google callback; fake-provider tests do not replace those paths.

Push source, deploy the actual version to staging and record the smoke results before engineering review. Confirm real Terms/Privacy routes and provider/sender configuration before enabling public registration. Production and iOS remain outside routine staging authorization.

References: [Google OIDC](https://developers.google.com/identity/openid-connect/openid-connect), [Google button branding](https://developers.google.com/identity/branding-guidelines), [OWASP token lifecycle](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html), [OWASP session handling](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
