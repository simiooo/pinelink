# Floating actions v52

Five native OpenPencil frames were saved, reopened and visually reviewed before implementation: 390px mobile, 320px compact mobile, desktop offline with a 34px safe area, and a compact update notice with reserved graph space.

The three actions share a single horizontal capsule: Files, Connect a device, Choose a file. The centre action remains blue; all icons are 24px within equal 48px touch targets. The container is 168×64px with 8px padding and 4px gaps, 16px above the bottom safe area. It hugs its contents at every viewport width. Tooltips and accessible names remain available; it does not imply selected navigation destinations.

The page reserves space for the toolbar. Error and update notices sit above it. Offline status appears just above the capsule; duplicate offline text stays hidden while an update notice occupies that area. Modals retain their higher layer and focus handling.

References checked 2026-11-05: [Google FloatingToolbarDefaults](https://developer.android.com/reference/kotlin/androidx/compose/material3/FloatingToolbarDefaults) (24dp icons and 48dp touch targets), [AndroidX floating toolbar tokens](https://github.com/androidx/androidx/blob/androidx-main/compose/material3/material3/src/commonMain/kotlin/androidx/compose/material3/tokens/FloatingToolbarTokens.kt) (64dp height, full corner, 8dp insets, 4dp spacing and 16dp screen offset). Dimensions are adapted to CSS pixels, with Pinelink's existing white/grey and #0866FF palette.
