# Safe application updates v50

Six saved/reopened OpenPencil frames cover the menu's check action and actual build label, a pending update, local work/drafts, another busy page, up-to-date and offline states. A compact nonmodal notice uses a short update action; no technical cache configuration appears in the product flow.

The notice stays out of the way while a conversation/modal or node management orbit is open. Its pending update remains available after returning to the overview and through the menu.

The client checks at startup, on foreground/online transitions (one-minute minimum interval) and every five active minutes. Explicit checks are immediate. Installed updates remain waiting until the user chooses Update and every open app page answers a readiness check. A second prepare check freezes interaction before activation. Active guest/account/local jobs, pairing, attached files, text/auth drafts and account operations block preparation. An unknown old page blocks activation and is named as the next action. The preparation lease expires if activation fails.

Only pages prepared by this handshake reload on controllerchange. Application asset precaching and obsolete-resource cleanup remain in Workbox. IndexedDB, identities, peer records, encrypted history, pending operations and files are not cleared. HTTP uses no-store for documents, worker, version, configuration and APIs; only content-hashed assets receive immutable caching. The menu displays the compiled client build, rather than the server's newest version.
