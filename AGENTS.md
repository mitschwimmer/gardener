# Working on Garden

Read README.md and docs/architecture.md before changing behavior.

The repository currently contains project setup only. Agree the resource MVP
before implementing apply, adopt, delete, or resource reconciliation.

Use the Go standard library and the official Incus module. Additional direct
application dependencies require a concrete justification and scope agreement.

Trust the official Incus client and server contracts. CI is daemon-free. Test
Garden's decisions and API usage with small handwritten recording fakes; do not
emulate Incus, mock HTTP, or add upstream client/server integration tests.

Keep Incus as the sole infrastructure state source. Protect unmanaged resources,
preserve unspecified properties and persistent data, and never automatically
delete or replace resources. Use fixed provisioning phases, not a dependency graph.

Run make check for changes. Run make ci before publishing. Keep the inner loop
and GitHub CI on the same checks. Do not commit binaries, caches, credentials,
or local toolchain installations.
