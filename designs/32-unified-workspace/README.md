# Unified node workspace · v32

SIM-949 follows the user's navigation and device-management feedback. The fourteen frames were saved, reopened and exported in native OpenPencil with faithful Nunito faces. The initial twelve frames preceded implementation; final spacing corrections and empty/offline states were then verified in the same document.

- The node graph is the single home for signed-in and signed-out sessions. Logo and closing secondary surfaces return here. Existing task/history/local-file links enter the same Files surface with a corresponding filter.
- Files combines active transfers, local saved files and historical outcomes. All / Active / Saved / Issues filters change the collection; status badges have accessible names and tooltips. Metadata stays locally left aligned. Mobile Files fills the screen; desktop uses a floating surface.
- A persistent header bell opens actionable notifications. Account and visitor scopes remain distinct internally, with original authorization, encryption and storage boundaries.
- Add device remains available after pairing and while transfers run. All devices appear in a responsive grid with deliberate axes; larger collections can scroll without covering the local device.
- Selecting a device reveals four 48 px controls: send, files, local display name, remove connection. Names are private local aliases, not edits to another user's device. Aliases and the last device roster persist offline per service.
- Removing a connection requires confirmation and server acknowledgement. It revokes only that pair, keeps local files and history, and rejects active transfers. Pair-row locking serializes removal with transfer creation. Offline removal never pretends to have revoked access.
- Enter/exit, active-node reveal and badge changes reuse v29 recipes and v31 retained backdrop exits. Reduced motion and keyboard dismissal remain supported.

WebRTC direct transport is independently tracked by SIM-947. This navigation unit does not claim that browser direct transfer is implemented.
