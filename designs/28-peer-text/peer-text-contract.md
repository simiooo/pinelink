# Cross-user devices and text delivery — proposed contract

Status: SIM-895 design proposal, not deployed behavior. The product explicitly requires transfers between different users' devices, a basic handshake, and text transfer. This document proposes the detailed interaction and engineering rules for those requirements. The account design inherited from v27 still requires product adoption. New UI must follow the repository's OpenPencil-first acceptance rule.

## Connection, item consent and identity

A device name, a discovery advertisement, a signed-in account, or knowledge of a recipient's public key is not a connection grant. A sender selects one explicit device. Do not expose a public user/email directory, the other user's full device roster, unrelated files or complete history. The device alias is enough for the normal UI; a small person badge distinguishes another user's device. Accessible names and focusable tooltips explain the badge without adding persistent copy.

The QR/paste invitation pins the intended device keys and an unpredictable, bounded, single-use invitation secret. Scanning or pasting and explicitly continuing is the initiator's consent; the other device allows or declines. A remembered device skips a future connection introduction, not per-item receiving consent. Remember is off for a new peer. Local trust binds the active service/account context and the peer's signing/encryption keys. Every new session still proves possession; a key change never silently overwrites a pin.

For nearby/manual connection without that out-of-band secret, both people compare and confirm a short authentication string. Bind roles, protocol version, service/account/device key identities, fresh nonces and negotiated capabilities to an authenticated transcript. Use an audited handshake with the appropriate commitment/PAKE or out-of-band construction and a security review. A six-digit hash of public metadata alone is not a secure pairing protocol. The illustrated `318 042` is sample display data, not a new cryptographic algorithm or shared secret.

Connection consent and receiving one item are different authorization decisions. If an invitation already binds a particular offer and its authenticated intent, one Receive action may confirm both, atomically. That grant is restricted to those keys, that device and that offer; it must not authorize later content. Declining either decision cannot implicitly accept the other. Closing a pending flow cancels its request; a remembered connection is created only after the authoritative commit succeeds.

Proposed connection states are INVITED, WAITING, CONNECTED, DECLINED, CANCELLED, EXPIRED and REVOKED, with a separate per-item state machine. UI labels can remain short. Expiry is authoritative server/local-protocol time, not a frontend animation timer. Online invitations should use the existing LAN invitation's 120-second lifetime as the initial product default; the secret/grant format remains versioned and capability negotiated. A short online invitation link can resolve a high-entropy, one-use record; do not copy every key and metadata field into a long public URL. Never record the invitation secret in analytics, access logs or notifications.

Remember/revoke operations need a stable operation ID and version checks. Approval versus cancel/expiry/revoke, double clicks, retries, two tabs and a changed account epoch must resolve to one authoritative result. Revocation fences active signaling and data operations as well as subsequent HTTP requests. Forgetting a peer does not erase user-owned local files or claim to remotely erase content already received.

## Routes and existing implementation

The existing P2P API already represents separate sender and recipient owners/devices. `backend/internal/service/p2p.go` validates participant fields and creates a pending transfer; `transfer.go` enforces participant/item state rules. These fields are a lower-level transport model, not the complete consumer connection handshake. `devices.go` exposes recipient-key lookup for a specified active device; the new product path needs a purpose-bound invitation/grant gate for lookup, offer creation, signaling and content authorization, without a public roster query.

Native LAN already has signed, expiring invitations, target key pinning, initiator proofs, human approval, replay checks and per-request revocation. See `cli/internal/lan/auth.go`, `types.go` and `client.go`. Its existing secret-bearing QR path is not being declared broken or replaced with an unauthenticated numeric code. Extend it only where the new manual/online interaction requires an explicit capability. Native fully local transfer must continue when the internet and central service are unavailable. Web may use an actually available bridge/transport; do not claim browser multicast discovery or arbitrary local socket access.

Online/direct, active relay and cloud restoration must apply the same participant, connection and item authorization at their relevant boundaries. A connected peer does not grant arbitrary key envelopes or history. Preserve the current route epochs, leases, receipt authority, bounded spool and per-frame I/O fences. Apply origin validation, message/session authorization and size/rate limits throughout the signaling connection, consistent with the [OWASP WebSocket guidance](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html).

Cloud ciphertext may upload before the recipient chooses Receive, following the adopted pre-upload policy. This does not allow unconnected users to create unlimited unsolicited offers. Accept gates manifest/envelope/downlink access. Decline or expiry atomically revokes this delivery's reference and authority; storage deletion waits for other legitimate references and active leases. Keep/restore references remain distinct. Do not reset the existing retention clocks on acceptance or route migration.

Account login/session support (SIM-884), local pairing (SIM-848), cloud reference lifecycle (SIM-847), replication/persistence (SIM-850/852/854) and client file data planes (SIM-859 and Flutter follow-up) are real dependencies. Switching a service must not transfer account credentials, trust, drafts or old requests into its namespace. This proposal does not silently introduce cross-service cloud federation. Local sessions remain independent of cloud availability.

## File and text payloads

Introduce an authenticated, versioned FILE/TEXT kind in the encrypted manifest. TEXT is `text/plain; charset=utf-8`, with exact bytes and line breaks. A `.txt` file remains FILE unless a person explicitly chooses conversion. Capability negotiation must produce a clear Send as file fallback for an older client; do not silently reinterpret content or show successful text support for an unknown manifest kind.

The proposed initial TEXT limit is 256 KiB of UTF-8 bytes, not JavaScript UTF-16 code units or visible characters. Disable a zero-byte send, preserve intentional whitespace, and never trim, normalize or truncate the body to fit. Show a short over-limit recovery with an explicit Send as file option. Implementation must define strict UTF-8 validation, authenticated byte counts and boundary tests, including multi-byte characters, invalid encodings, newlines and payloads at/over the bound.

Reuse the encrypted payload/data path, chunk resume, byte/hash validation, durable local commit and authenticated receipt. Text should not become an unencrypted signaling/chat message or a second delivery ledger. Direct and active relay continue to report actual receiver-confirmed bytes. Cloud Stored and recipient Delivered remain separate. Completion waits for the receiver's required validation, durable commit and receipt. Copying text is independent of delivery. A tiny message may finish immediately after a real receipt; do not impose a fake progress duration.

Keep the plaintext body and derived first line out of cloud filenames, logs, outbox transport metadata and notifications. Use generic `Text` as transport-facing display metadata. A local history title may derive from the first nonblank line with single-line ellipsis; this never changes the body. Decrypt/render only in the authorized local profile. Drafts and retained text use the same service/account/device isolation and local encryption boundaries as file content; do not replay an old profile's queue after login/logout or a service change. Browser local encryption does not establish protection against code executing in that same origin.

Receiver consent shows the sender/device and generic text type, then a selectable plain-text view after acceptance. No HTML execution, automatic URL navigation, remote link previews or background clipboard reads. Copy text is an explicit user action and Copied appears only when the write promise succeeds. The browser Clipboard API is restricted by secure-context, permission and user-activation rules; failures retain selectable content with Select text/manual-copy recovery. See [W3C Clipboard API](https://w3c.github.io/clipboard-apis/) and [MDN writeText](https://developer.mozilla.org/en-US/docs/Web/API/Clipboard/writeText). A denied clipboard write must not undo a valid transfer receipt.

Offline compose retains an encrypted local draft and a visibly pending local intent. Queue does not mean Sent/Delivered and cannot manufacture a remote connection grant. Reconnect reconciles the invitation/grant, peer capability and current account epoch before automatic retries. Prevent duplicate messages with the same operation ID and exact payload identity. If authorizing the recipient now requires a person, resume that decision instead of bypassing it in background sync. Leave the draft available after rejection, expiry or unsupported capabilities.

## History and synchronization

Use the inherited icon grid with left-aligned local text axes. Every item has a type icon, local title and small availability badge; tooltip and accessible name describe the state. Files and text share All/Reusable/One-time/Expired filters. Do not add a permanent metadata/status paragraph below every entry.

Expired cloud access does not make an already retained local copy unreadable. Copy/open remains available where local content was committed; only a missing local body plus an expired cloud grant produces Text expired. A one-time cloud retrieval policy cannot prevent the receiving person from copying accepted bytes and must not be sold as view-once or remote erasure.

Replicate only authorized participant projections and encrypted facts, never join two users' full streams. Native `replication_publication.go` already limits local facts/envelopes to the applicable trusted participant; online cross-account projections need the equivalent explicit authorization, durable cursors and atomic commit boundaries. Local pending intent, cloud availability, receive acknowledgment and user copy activity are separate facts. Store typed versioned events with deduplication; do not infer these transitions by parsing display strings.

## Layout, motion and accessibility

Keep Nunito, blue `#0866FF`, white/gray surfaces, simple icon actions and local left alignment. Desktop uses a 480px centered modal; mobile uses a bottom sheet or viewport-sized editor above the OS keyboard. Modal horizontal/vertical insets are 20/24px; mobile sheets use 16/24px. Related label/code content uses 8px; identity uses 12px; related controls use 16px; functional groups use 24px; grid rows use 32px. These are semantic gaps, not one gap repeated everywhere.

Tabs use measured labels with their own 12px horizontal inset inside 48px targets and 8px between targets. They are bare text with a thin active underline. Filename labels have a fixed single line; icon-to-label spacing is 8px, while rows use 32px. Badges overlay icons instead of occupying a second text row. The connection curve starts 8px below the peer's 24px label box, preserving the icon/label group.

| Transition | Proposed motion and authority |
| --- | --- |
| Confirmed peer appears | 180ms opacity 0→1, Y 8→0; only after mutual authorization |
| Desktop modal | 160ms opacity and gentle scale |
| Mobile sheet | 220ms, 24px travel; exit 160ms |
| Error/status replacement | 160ms; no success timer substitutes for a response |
| Transfer activity | inherited 1.8s flow within the genuinely confirmed blue arc; pause freezes flow and boundary |
| Copied | after clipboard success, 160ms icon transition and brief announcement; restore action after about 1.2s |
| Reduced motion | immediate/static state changes and a static busy cue |

48px touch targets remain usable at 320px and above the keyboard. Icon controls need tooltips, accessible names and visible focus. Restore focus to the opener on dismissal; trap focus only while a modal is open. Escape/cancel must preserve the appropriate draft and cancel only the intended pending request. Enter submits only the focused form; multiline editing must keep a normal newline gesture. No invitation, approval, send, receive or clipboard side effect may be triggered merely by a React mount/effect replay. Use explicit events, bounded resource cleanup, server idempotency and stale-response/account-epoch checks.

Motion is specified in editable contract boards; no playable native prototype or runtime animation is delivered here.

## Implementation acceptance on Fedora

After product design adoption, implement and verify the whole engineering unit before marking it in review:

- Two real accounts/devices exchange FILE and TEXT in both directions. An unrelated third account cannot enumerate devices, read body/envelopes/history, signal or use another device's grant. Remembered connection still requires the intended item consent.
- Pairing replay, forged identities, invitation expiry, wrong codes, changed keys, duplicate confirmation, cancel/approve/revoke races, rate limits and interrupted sessions are tested with real authority transitions. Revoke is enforced during active I/O.
- Direct, relay, route switch, deferred cloud receive and native no-internet LAN preserve payload bytes/hash and the single receive receipt. Cloud stored 100% cannot become delivered 100% without that receipt.
- UTF-8 boundaries, empty/whitespace/multiline content, invalid input, HTML-looking text, old-client fallback, cloud expiry with/without retained local content, clipboard denial and explicit manual-copy recovery are covered.
- Web/Flutter persistence, cold offline reopen, reconnect catch-up, account/service changes, two tabs, StrictMode, close/dispose, queued send retries and late results are tested without duplicate delivery or plaintext transport metadata.
- Run the appropriate Go race/database/integration and client checks on the isolated Fedora build machine. Push public source and packages, actually deploy staging, smoke the affected paths, then record its URL/version/time/evidence in SIM-895, set in_review and release the claim. Only complete product acceptance moves the unit to done. Production and iOS remain outside this authorization.
