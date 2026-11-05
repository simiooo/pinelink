# Connection status and retry v51

Saved and reopened with OpenPencil core. Device presence stays attached to the corresponding node: confirmed remote absence is Offline; a closed local data channel is Connection interrupted; an unavailable/stale service presence check is Status unknown. A service-wide failure names Pinelink service and retries the service check, independently of any active transfer. It does not assert that every device disconnected. Node controls still open device management.

An icon-only button swaps its visible icon for one centred spinner during its pending action. The original label remains mounted but hidden to retain geometry. The button retains its accessible name, fixed target and pending click gate, then restores its original icon after success or failure. Text buttons keep their existing behaviour.

The service notice has a bounded readable width and reserves vertical room between the graph and bottom actions while visible; its retry target cannot shrink. Narrow screens retain the local node and label without overlap.
