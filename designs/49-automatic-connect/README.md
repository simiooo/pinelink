# Automatic connection v49

The saved and reopened OpenPencil document covers one show/enter flow, resolver availability, a reply QR only when necessary, mutual verification, and an unreachable device. There is no online/offline or local/server selector.

The default QR is one signed PL2 invitation with both its local SDP and an optional same-service rendezvous token. New Web clients choose a reachable service automatically and fall back to the signed local offer on transport failure. Legacy local clients can still validate the complete signed map and use its SDP. A short eight-character code remains available when the resolver is reachable; without a resolver the interface asks for the complete QR instead. A signature failure, service mismatch, authorization denial, revocation or rate limit is never treated as a transport failure.

Local confirmation remains mutual. Pairing does not approve incoming files or text; each new transfer retains its receive/decline decision. Camera access is requested only for scanning. The two-minute label follows the existing pending WebRTC session lifetime. No client claims to discover arbitrary nearby devices, connect without a reachable path, or keep browser sessions alive in the background.
