# Account browser handoff companion

Five compact, editable frames supplement v27 for the external browser flow:
desktop/mobile matching-code approval, app waiting, return and cancellation.
They reuse Nunito, #0866FF and the 8/16/24/32 semantic spacing groups. The code
`1234 5678` is illustrative. No credential or personal account appears here.

`build.mjs` creates the editable SVG sources. The native file is
`../27-pinelink-account-browser-handoff.fig`. `native-review.json` records an
independent OpenPencil-core save/readback and five rendered PNG checks.
The existing shared app still reports a CanvasKit memory error when opening
documents, including this English-only companion. A fresh native GUI reopen
has **not** passed; other projects' open documents were preserved.

Runtime motion follows v29: 80ms press, 160ms state reveal, 280ms page entry,
and 1200ms activity rotation. Reduced motion uses static activity feedback;
hidden or paused app work does not keep a decorative loop running. A callback
or upload animation never stands in for a server acknowledgement.
