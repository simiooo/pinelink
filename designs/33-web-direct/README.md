# Web direct transport · v33

SIM-947 extends the approved v20/v31/v32 graph. Nunito, primary #0866FF, 4px spacing, restrained Meta-style controls, all device labels and ordinary controls remain unchanged.

The graph uses two endpoints and one delivery curve only when an actual browser DataChannel is selected. Blue is durable recipient acknowledgement; the sheen is activity, never invented completion. While negotiating or reconnecting the curve is neutral/dashed. A small status mark exposes a short tooltip and an accessible status. Detailed connection and byte counts stay in the modal/bottom sheet, left aligned.

Fallback keeps acknowledged delivery bytes. The cloud node enters only when the real cloud transport is opened or a spool exists, with separate cloud-storage and delivery curves. Its entrance is a short fade/scale, with no full-page remount. Pause freezes delivery and sheen. A receiver pause may close direct I/O; the sender may then stage an independent cloud copy, as in the existing policy. Refresh discards stale connection claims and safely resumes an attempted transfer through cloud fallback; it never revives a persisted `connected` flag.

Pairing and file consent remain separate. Previously paired devices may exchange bounded connection metadata while the file awaits consent; no file data is sent until Receive commits. Cloud staging remains possible before consent after a direct attempt fails. DTLS/SCTP metadata is bound to the signed transfer/role and the current offer; file encryption, durable chunks, hash verification and final receipt are identical on both routes.

Motion: negotiation mark reuses the existing 1.2s spinner; the delivery sheen uses the approved 1.8s loop. Topology/cloud entrance is 180ms, with no progress tween on pause or a route change; acknowledged blue bytes stay visible during reconnection. Completion settles only after verification/receipt. Hidden pages and `prefers-reduced-motion` show static states. Existing Close/Escape/backdrop/swipe exits keep the persistent graph.

Browser direct transport still requires reachable pairing/signalling/progress services. LAN priority uses host ICE candidates; no TURN data relay is labelled Direct. Native offline discovery and fully disconnected browser bootstrapping are separate capabilities.

Research: [WebRTC Recommendation](https://www.w3.org/TR/webrtc/), [RTCStats](https://www.w3.org/TR/webrtc-stats/), [Chrome local-network access](https://developer.chrome.com/blog/local-network-access). Use bounded SCTP messages, application flow control, actual candidate-pair/data-channel counters and explicit cleanup on every terminal path.
