# Device conversation v40

OpenPencil draft following product feedback on v39. The message rows use the
shadcn/ui Message and Bubble pattern: alignment follows the sender, content is
in a compact muted bubble, and icon actions sit underneath it. Files, Markdown
and masked sensitive text share the conversation. Sensitive text remains
hidden until explicitly revealed, and copying remains an explicit action.

The `.fig` file was saved, reopened and rendered with OpenPencil core; the PNGs
are previews of the reopened document. The editor GUI did not respond to its
RPC request. This is a design draft, not evidence that the complete conversation
feature has been implemented or deployed.

Reference: https://ui.shadcn.com/docs/ai-elements/radix/message
