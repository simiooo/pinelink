# Overlay notices and right corner actions v54

Seven OpenPencil frames were saved, reopened and visually checked before implementation. Matching before/after mobile and desktop frames retain identical graph and device positions. Notices are overlays and never reserve or shrink the home container.

The existing 168×64 action capsule sits at the bottom right, 16px from the screen edges plus applicable safe-area insets. Its 24px icons and 48px touch targets remain unchanged. The compact update notice has a 44px minimum height and close target, directly above the capsule. Service retry uses a 48px row and target. The original conditional graph-height rules are removed; service feedback fades in without a positional offset.

Node, graph and page bounds must stay identical when notices appear, disappear or change text. Browser verification covers both mobile languages, narrow and desktop layouts, actual toolbar entry actions, safe areas and service retry without overlap.

An explicit update check or available update takes priority in the single overlay area. The service error stays in runtime state and keeps retrying; dismissing the update notice reveals it again if it has not recovered. A service error cannot hide the update control.
