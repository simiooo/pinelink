# v31 · Cloud relay and surface exit

SIM-946 corrects two observed behaviors: closing a sheet briefly restored an
opaque backdrop, and a cloud icon replaced the destination device.

The relay graph retains both devices and places a cloud node between them.
Its upload segment uses durable spool acknowledgements; its download segment
uses recipient acknowledgements. Receiving after the sender exits keeps the
sender visibly offline and the cloud leg available. A stored badge belongs to
the cloud copy and does not mean the recipient has completed verification.
The download boundary remains fixed during a pause. The upload segment may
continue if that independent upload is still active.

Modal and sheet backdrops fade monotonically and retain their transparent final
frame until teardown. The device graph keeps its position and identity during
closing. Reduced motion removes the decoration immediately.

Eight editable frames in [the OpenPencil file](../31-pinelink-cloud-route.fig)
cover desktop/mobile upload, download, pause and sheet exit at 0/80/160 ms.
This file was saved, reopened and rendered in the native OpenPencil application;
Nunito was reported faithful. SVG progress segments use actual split Bezier
geometry so native imports preserve their blue/gray boundary.

The deployed Web anonymous transport currently uses the encrypted cloud relay.
Browser WebRTC direct transfer remains a separate implementation unit; this
design does not label a relay connection as a direct LAN connection.

References: [Radix animation lifecycle](https://www.radix-ui.com/primitives/docs/guides/animation)
documents ownership of forced mounting; [CSS animation fill mode](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/animation-fill-mode)
explains retaining the final frame. [WebRTC connectivity](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API/Connectivity)
describes ICE negotiation for a separate direct path.
