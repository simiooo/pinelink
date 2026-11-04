# Pinelink component source

Generated on the Fedora build machine with the official shadcn CLI 4.23.0, using the `new-york` / `new-york-v4` Radix registry. Twelve editable component sources live in this directory. Installation follows [the official Vite guide](https://ui.shadcn.com/docs/installation/vite) and [component source workflow](https://ui.shadcn.com/docs/cli).

Pinelink customizations retain the accepted v29 design: Nunito, primary #0866FF, white/gray surfaces, semantic spacing, locally left-aligned content, 48px targets and restrained shadows. Button sizes and focus appearance are changed here; Dialog/Sheet expose bounded motion presence to the shared owner. `src/motion` adds interruptible motion and async action gating while the primitives own accessibility, focus, keyboard navigation, portals and dismissal. Do not overwrite these files with an unreviewed CLI `--overwrite`.

Source attribution: shadcn/ui is MIT licensed, copyright shadcn. Retain its license alongside these adapted files and the package notices in releases. Runtime primitives are Radix UI; the CLI and Tailwind are build tooling. This directory does not introduce new backend actions or imply that account/file-data/peer features awaiting their own implementation are complete.

Message, Bubble, Input Group, Textarea and Switch are copied from the Radix registry at shadcn/ui commit `8a3536678a528cc012b9e62484290e79d0be410f`; registry import paths were adjusted to the existing local components. Conversation presentation is styled in the product's guest CSS.

Button, Dialog, Sheet, DropdownMenu, Tooltip, Input, NativeSelect and Progress are bound to existing product paths. Badge, Tabs, Checkbox and Separator are prepared foundations for the upcoming feature pages; their presence in this directory does not claim those pages are shipped. Keep motion in the shared owner rather than adding a second transition to the same property.
