# Serverless LAN · v34

SIM-955 closes the Web gap left after SIM-848 native offline delivery and SIM-947 online-signalled WebRTC. Keep the approved graph, Nunito, #0866FF and semantic 4px spacing. Titles use the device name once known. Local connections stay visibly reachable even when the internet is down.

The connect sheet adds Online / Local. Local offers a QR connection package and paste/scan fallback; the reply travels back the same way. Both devices compare and confirm a short code before persisting trust. The short code is a comparison, never an authentication secret. Expiration and camera denial have one concise recovery action. QR patterns in these native previews are inert examples, never real invitations.

Local transfer uses two endpoint nodes and one acknowledged-delivery curve. No cloud node or cloud-restoration affordance appears. Connection loss keeps confirmed bytes and opens reconnection from the node/details; a disconnected browser does not pretend to discover or resume a peer automatically. Verified local history survives closing both browsers. Detail text is left aligned; file icons carry small saved-state badges.

Reuse v29 surface entrance/exit and the persistent graph. The 1.8s sheen indicates live transfer activity; pause, verification, disconnection and reduced-motion stop it. Live negotiation reuses the 1.2s spinner; expiration/camera errors settle once. No page-wide transition on modal close or network change.

Browser-only local pairing exchanges signed, expiring SDP outside any server, then uses normal host ICE and DataChannel. Installed/cached PWA resources work offline; first use needs locally available resources, supplied by a localhost launch bundle. Native discovery uses the existing Bonjour/DNS-SD core; browser manual pairing does not masquerade as raw mDNS discovery.

Research: [WebRTC signalling is application-defined](https://webrtc.org/getting-started/peer-connections), [Chrome Local Network Access](https://developer.chrome.com/docs/modern-web-guidance/guides/security/local-network-access), [local authority contract](../../infra/docs/offline-local-delivery.md).

When the complete signed SDP cannot fit a QR, retain the same connection package and tools, show a download glyph with “Save the connection package”, and allow file import on the other device. Never leave a permanent loading spinner or silently omit candidates. This recovery is included in the 18th editable frame and native save/reopen/export verification.
