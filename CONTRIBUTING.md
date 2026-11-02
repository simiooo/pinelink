# Contributing

Use the pinned dependency files and Flutter SDK. Keep dependent protocol, server and client changes in the same pull request.

Run the checks for affected modules. Concurrency and persistence changes need meaningful lifecycle, retry and transaction coverage. Integration tests require their documented services; report skipped checks explicitly.

UI changes start with editable OpenPencil designs. Keep white/gray surfaces, primary `#0866FF`, semantic spacing, local left alignment in dense sections, accessible icon actions and reduced-motion behavior. Transfer progress reflects confirmed bytes; staging success and delivery receipts are separate facts.

Do not commit identities, secrets, local databases or build caches. Validate deployments in staging. Public production deployment and iOS distribution are separate release actions.
