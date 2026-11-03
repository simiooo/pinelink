# Account service

The official preview supports Google and independent email/password sign-in.
The server resolves an account before enrolling the device's permanent keys.
Google is keyed by issuer and subject; a matching email does not merge accounts.
Registration and password recovery use a 15-minute email proof, an eight-digit
code, or a 22-character opaque `/a/…` link. Opening a link never consumes it.

## Runtime configuration

Keep values in a private secret manager and an external Kubernetes Secret.
Set `existingSecrets.accounts` to its name when deploying the public Helm chart.
No OAuth secret, mail key, session token or encryption key belongs in a build,
client bundle, public values file or GitHub workflow.

| Variable | Meaning |
| --- | --- |
| `PINELINK_ACCOUNTS_REQUIRED` | Require account sessions as well as signed devices; enable on a public service. |
| `PINELINK_ACCOUNT_ORIGIN` | Trusted public HTTPS origin; never derived from Host. |
| `PINELINK_ACCOUNT_ENCRYPTION_KEY` | Stable, random 32-byte key in standard base64; protect and back it up. |
| `PINELINK_GOOGLE_CLIENT_ID` / `PINELINK_GOOGLE_CLIENT_SECRET` | Server-side OAuth web client. |
| `PINELINK_GOOGLE_REDIRECT_URI` | Exact registered callback. The provided proxy supports the public origin root. |
| `PINELINK_RESEND_API_KEY` | Environment-specific Resend key for a verified sender domain. |
| `PINELINK_MAIL_FROM` / `PINELINK_MAIL_REPLY_TO` | Verified sender and support reply address. |
| `PINELINK_AUTH_TRUSTED_PROXIES` | Comma-separated peer CIDRs allowed to supply the edge's overwritten client IP header. |
| `PINELINK_ACCOUNT_QUOTA_BYTES` | Default account storage allocation. |
| `PINELINK_PASSWORD_BREACH_CHECK` | Check new passwords through the padded Pwned Passwords hash-range API. |

The proxy preserves Origin for CSRF checks, discards preview Basic credentials
before forwarding, and does not log requests or cache authentication responses.
Cloudflare must forward to this trusted proxy; direct API access stays private.
Once the account service is enforced, deploy with `previewAccess.enabled=false`
to let visitors reach guest mode, registration and sign-in directly. This removes
the browser's old operator-password prompt and its Secret mount. It does not
disable application sessions, device signatures, CSRF or HttpOnly cookies.
The chart keeps the gate enabled by default for older previews. Disabling it
requires `existingSecrets.accounts`; the operator must also verify
`PINELINK_ACCOUNTS_REQUIRED=true` before opening access.
The app serves the shell offline, while proof, callback, short-link and session
responses always use the network. Passwords never enter a sync journal.

## Identity and recovery

Web uses Secure, HttpOnly, host-only cookies and a per-session CSRF token.
React and Flutter Web use separate cookie slots and account storage namespaces.
Native clients keep their session token in the platform vault and use
`X-Pinelink-Session`, leaving Authorization available to an outer preview gate.
Native Google sign-in uses an external browser and matching-code approval;
browser return URLs never contain a session credential. Flutter Web reserves a
popup during the button gesture and claims its bound authorization in the app.

Reset increments the credential epoch and revokes old cloud sessions. Account
and session locks fence durable writes against reset/logout; active relays also
check revocation during I/O. Sign-out stops local cloud work immediately; an
unreachable revocation remains pending and is retried before a new sign-in.
Device revocation is permanent for that enrollment and normal registration
cannot revive it. Normal sign-out preserves the device identity for later fresh
authentication. Existing development owners are never assigned to an account.

Local keys, history and files survive sign-out. A password reset cannot recover
private file keys lost with every trusted device. The separate file/outbox,
service-switching and cross-user handshake work remains in its own scope.

## Verification

`backend/internal/accounts` tests use the isolated PostgreSQL URL
`PINELINK_SERVICE_TEST_DATABASE_URL`; each case creates and removes its own schema.
`backend/cmd/accountfixture` is an isolated, loopback-only browser fixture, with
an in-memory mail adapter. It is absent from the production image. Run
`web/e2e/accounts.mjs` alongside it to check real browser cookies, CSRF, signup,
recovery, cross-device confirmation and local-only access. Synthetic fixture mail
does not establish Resend delivery or real Google-account consent; verify those
separately in the deployed environment with a controlled mailbox and account.
