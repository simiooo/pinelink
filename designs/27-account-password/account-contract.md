# Account contract: Google and password

Status: v27 design proposal after the user's correction. Product acceptance and runtime implementation remain pending. This is not a deployed API specification. The v26 passwordless-only proposal is superseded.

## User paths

The device graph stays the starting point. The More menu and authenticated cloud actions lead to account entry. Local history and supported nearby offline transfers do not require a cloud account; browser local-network limitations still apply.

Google sign-in and password sign-in are independent methods. The Google button is outside the password form and uses `type="button"`. It never reads, validates or submits Pinelink email/password fields. Standard Google authorization resolves the provider identity, creates an account on the first successful sign-in and enrolls the correct device profile, then returns directly to the graph or original action. It does not pass through Pinelink email entry, mailbox proof, password setup or profile setup.

Password login uses an email and password. Registration uses the same fields on a Create account page, then proves mailbox possession with a code or short link. Require only one password input; provide Show/Hide, paste and password-manager support. Mailbox codes are for signup, recovery and explicit method verification, not the default returning-user login mechanism.

Forgot password keeps the email, proves the reset purpose, asks for one new password, then returns to ordinary sign-in with the email retained. Do not automatically sign in an unknown waiting requester. Successful normal login/signup navigates only after server-confirmed account and device enrollment; no mandatory nickname, configuration, tutorial, payment or success page.

Cancellation, resend, expiry, delivery failure and restart retain safe input and intent. Never persist plaintext passwords. Store a bounded internal continuation intent, not recursive `next` URLs. Auth routes cannot be their own continuation, and a changed account must re-authorize the target resource before resuming it.

## Passwords and unverified signup

New or reset passwords have at least 15 Unicode code points and accept at least 64; use a bounded maximum of 128 code points/512 UTF-8 bytes. Do not trim, silently truncate, impose uppercase/symbol mixtures, disable paste, or require periodic changes. Use an explicit, consistent Unicode normalization policy for creation and verification. Block commonly used, compromised and service-specific passwords; return a short actionable error. A password-strength service must never receive the full password.

Use Argon2id with random per-credential salts and a versioned PHC encoding. The starting minimum is OWASP's 19 MiB, two iterations, one lane; benchmark on the actual server before selecting final costs. Bound the number of concurrent KDF operations, payload sizes and queue wait, and check authentication/rate limits before expensive work. Run hashing outside database transactions and locks. Database-supplied PHC parameters have strict upper bounds before verification. Use a comparable dummy-hash path for unknown users and generic failed-login responses.

An anonymous signup request creates only a pending challenge and a pending password hash. It does not create an active account, cloud business access or a usable password credential. Bind the pending hash to the signup generation and initiating proof key. Successful proof in that initiating client may atomically promote its own pending credential. A new signup for an already active identity never changes its password; after mailbox proof show Sign in / Reset password.

A signup link opened on another device must not promote an unknown password chosen by the original requester. Ask for a new password on the current device, invalidate the old pending credential and bind any new grant to that device's own proof. The original requester stays signed out. A later Google claim must likewise not preserve an attacker's unverified password or activate unrelated pending credentials.

Password reset/change is compare-and-swap against the account credential version. A successful reset invalidates the reset generation, outstanding login grants and prior cloud sessions/device credentials for that account. Existing streaming/object authorization must observe this revocation; changing only a cookie row is insufficient. Never use signup as an implicit password reset. Recovery of account access does not reconstruct missing end-to-end file keys.

## Google identities and optional linking

Google identity is validated issuer plus `sub`, not its contact email. Validate authorization state, nonce, audience, issuer, expiry, code exchange and PKCE using the maintained OIDC library. Request only necessary `openid email profile` scopes. Server secrets stay in private deployment configuration. Match the exact callback registered for each environment; production callback configuration is not production-release approval.

Do not silently link, transfer or merge accounts because email strings match, including a provider's verified email claim. Standard Google sign-in resolves its own identity directly without a second Pinelink email form. Optional linking in account settings is a separate operation with fresh proof of the existing account and the new method. If a method already belongs to another account, return a recoverable conflict; do not move files or identities. The Connect Google/password page belongs only to this explicit operation and is not part of standard Google login.

Adding a password to a Google account is optional and uses its known address with appropriate proof. It cannot overwrite an identity attached to another account. Normalize email conservatively and consistently; never strip provider-specific dots/plus tags to infer account equivalence.

## Short email links and proof lifecycle

HTML mail has one purpose-specific Verify email / Reset password button and an eight-digit code. Plain text includes the same code and a short HTTPS URL. No click tracker or third-party shortener is involved.

The URL is `https://pinelink.sqmem.top/a/<token>`; staging uses its own trusted origin. The token is 16 cryptographically random bytes encoded as 22 unpadded base64url characters. The official production shape is 51 characters. It contains no email, JWT, session, device key, return URL or user record. Configure a trusted origin rather than deriving it from an untrusted Host header. Persist the token's SHA-256 digest and purpose. Protect the low-entropy code's digest with a separately stored versioned HMAC key.

Challenges expire in 15 minutes, allow at most five wrong code attempts and have a server-enforced 60-second resend cooldown. Apply shared budgets by challenge, address digest and trusted source address across replicas. A code is not a global short URL or standalone enrollment credential. Equivalent start responses/timing must not disclose account existence; any specific existing-account state requires appropriate proof.

GET, HEAD, prefetch, email scanners and service-worker fetches never consume proof or create a session. The landing page has an explicit confirmation action; its POST performs consumption with CSRF/origin protection. Remove the raw token from browser history promptly, use no-store/no-referrer, disable indexing/analytics on sensitive routes and redact token paths and callback queries in ingress/application logs. Invalid/used links do not disclose an address.

Code/link consumption, identity resolution and creation of one short-lived claimable login authorization serialize in one database transaction. Actual account-session/device enrollment commits in a later atomic claim transaction. Same-client idempotent retries require the original initiating secret, PKCE and key proof. Duplicate submits, tabs, link/code races, resends, callbacks and replicas cannot issue an authorization to a different key or create duplicate identities. Tokens are purpose-bound: a signup proof cannot reset an existing password.

Opening mail on another browser never signs in the waiting requester. Bind any current-device continuation to a fresh proof key; enter the code in the original app to continue there. The UI includes same-device confirmation, current-device signup, expired and used states. Reset links lead to the reset-purpose new-password flow rather than consuming a signup credential.

## Honest email delivery

Show Sending while server/provider outcome is pending, Check your email only after provider acceptance, and a retry state after failure. A queue acknowledgement is not successful delivery. Cooldowns/expiry use server timestamps and survive reload/another tab. Provider acceptance also does not prove mailbox delivery; controlled staging receipt remains a separate smoke condition.

Persist a bounded outbox operation with encrypted mail payload, generation, lease and provider idempotency key. Mail I/O never holds a database row lock. The worker checks current generation/consumption/expiry before sending. Late success/failure uses generation compare-and-swap and cannot undo a later resend or verification. Never log codes, links, plaintext mail or passwords. Unconfigured mail fails closed without pretending a code was sent. Pinelink needs its own verified sender/provider configuration; learning from another product does not authorize copying its private sender credentials.

## Account, session and device boundaries

Separate stable accounts, provider identities, password credentials, pending proofs, login authorizations, sessions and signed device enrollments. Public clients cannot choose an arbitrary owner ID. An unverified email request has no business authorization.

Start auth with an ephemeral proof key and PKCE, not a permanent file-signing/encryption key selected before the account is known. After the account resolves, select/create that service-origin/account device profile, then claim enrollment with its real key proof and the fresh authorization. Reusing a guest or another account's permanent key as an automatic identity migration is forbidden. Never revive a revoked enrollment merely because a later login succeeded.

Web account sessions use Secure, HttpOnly, host-only cookies with CSRF protection. Native account credentials use secure platform storage; ephemeral native browser returns contain no session secret in their URLs. Sessions have bounded lifetime, rotation and revocation. Signed device credentials have bounded lifetime and are checked at HTTP, object-grant, control and long-running relay authorization fences, not only at initial WebSocket connection.

Namespace cloud state by service origin and account. Switching profiles advances a client epoch and cancels old queries, subscriptions, sync workers and file work. Late responses and outbox operations cannot enter another namespace. Keep nearby guest identity/local history separate. Sign-out stops local cloud work immediately and revokes the matching reachable server session/device grant. Offline sign-out cannot claim remote revocation has completed. Session expiry does not delete local files or nearby-pair trust.

Preserve the existing development database. Do not assign old owner rows to accounts by a matching label/email or clear data for auth cutover. Any legacy mapping requires an explicit verified migration. Official service UI has no technical configuration page; self-hosted advanced settings remain an explicit separate choice, isolated by origin.

## Proposed HTTP surface

Names below are implementation contracts, not available endpoints.

| Route | Purpose |
| --- | --- |
| `GET /v1/auth/capabilities` | Public enabled methods; no secrets |
| `POST /v1/auth/requests` | Ephemeral proof key, PKCE and safe continuation |
| `POST /v1/auth/password/login` | Verify existing password into one claimable authorization |
| `POST /v1/auth/password/signup` | Pending password hash plus signup-purpose mail challenge |
| `POST /v1/auth/password/reset/start` | Generic reset-purpose mail request |
| `POST /v1/auth/password/reset/complete` | Consume reset grant, replace hash and revoke old cloud credentials |
| `POST /v1/auth/password/change` | Fresh proof, bounded KDF and version-checked change |
| `POST /v1/auth/email/{id}/resend` | Replace one purpose/generation under shared budgets |
| `POST /v1/auth/email/{id}/verify` | Purpose-bound code proof for the initiating client |
| `GET /a/{token}` | Non-consuming short-link landing |
| `POST /v1/auth/email/confirm-link` | Explicit current-client proof consumption |
| `GET /v1/auth/google/start` | OIDC state/nonce/PKCE authorization |
| Registered Google callback | Validate provider response, resolve identity once |
| `POST /v1/auth/requests/{id}/claim` | Fresh authorization plus actual profile-key proof; atomic session/enrollment |
| `GET /v1/auth/session` | Current account/session and enrollment state |
| `POST /v1/auth/identities/link` | Explicit fresh-proof linking, never email-only merging |
| `POST /v1/auth/logout` | Idempotent current-session/enrollment revocation |

Auth responses, callback routes, short links and request bodies bypass navigation caches, ordinary request logging and offline outboxes. A previously cached auth success cannot authorize cloud work after offline restart. Cloudflare, ingress, Web routing and service-worker rules must agree.

## Layout, keyboard and motion

Use standard labeled email/password fields with correct autocomplete purposes (`username`, `current-password`, `new-password`) and one numeric eight-digit input for whole-code paste/autofill. Visual code spaces are not wire characters. Enter submits only the active method. Google never invokes password-form constraints. A password-visibility toggle is a button with an accessible name and does not submit the form.

Labels/inputs use 8px, related fields/actions 16px and separate functions 24–32px. Dense forms are left aligned. All targets are at least 48px, focus is visible and errors are announced. Scroll/reflow at 320px and with the keyboard open; preserve the field/action and avoid footer overlap. Terms/Privacy links require actual routes/content before public registration is enabled.

Entry uses opacity 0→1 and Y 8→0 over 180ms ease-out; exit 120ms ease-in; a status region changes over 160ms without stealing focus. Reduced motion changes immediately with a static busy indicator. Activity reflects an actual request. No send, resend, consume, login or link mutation runs from a React mount effect; server idempotency is required in addition to disabled controls.

## Delivery and unresolved work

OpenPencil product acceptance precedes UI implementation. The old local Google WIP is partial: it lacks the complete HTTP/client/session flow and embeds permanent device keys before account resolution. Its identity/schema and enrollment design must be revised instead of exposing that draft unchanged. Keep the latest relay expiry/revocation fences when integrating it.

Fedora checks must cover real PostgreSQL concurrent first sign-in, duplicate identities, wrong/expired codes, code/link replay, resend/verify/mail races, scanner GET safety, stolen pending-signup passwords, same-address Google/password conflicts, bounded KDF load, reset/logout/expiry versus live transfers, React StrictMode and late account-switch responses. Verify PWA offline cold start, mail-link caching exclusions, nearby offline access and reconnect sync without replaying auth effects. Password recovery cannot fake file-key recovery.

Push source, deploy the actual version to staging and record real Google callback and controlled-mail receipt, account switch, auth-gated cloud transfer and offline/reconnect smoke evidence. Mock-provider tests do not replace those paths. Do not mark engineering delivered before test/push/deploy/smoke. Production and iOS are outside routine staging authorization.

References: [Google OIDC](https://developers.google.com/identity/openid-connect/openid-connect), [Google brand assets](https://developers.google.com/identity/branding-guidelines), [NIST password requirements](https://pages.nist.gov/800-63-4/sp800-63b.html), [OWASP authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html), [OWASP password storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html), [OWASP reset lifecycle](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html), [OWASP sessions](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
