# Live Incus integration tests

## CI isolation and coverage

The **Incus integration** job in `.github/workflows/ci.yml` runs on a fresh
GitHub-hosted Ubuntu 24.04 runner for PRs, pushes to `main`, and manual dispatch.
It installs the distribution's Incus/agent, dnsmasq, QEMU, OVMF, and virtiofsd packages, starts the
daemon, waits for readiness, and initializes a minimal local setup. The runner
and all its Incus resources are discarded after the job. No credentials or
connection to the operator's garden are used. Repository access stays read-only;
checkout does not persist credentials. There is no Incus network API listener.
`dnsmasq-base` is required because minimal initialization creates a local bridge;
omitting recommended packages without installing it makes initialization fail.
VM starts also require `qemu-system-modules-spice` with Ubuntu's split QEMU
packages; without it Incus's SPICE launch option is rejected even headlessly.

Tests are compiled as the runner user, then only the test binary runs with sudo
to access the local Unix socket. They log the actual server version. The daemon
version follows Ubuntu's package updates; the official Go client remains pinned
in `go.mod`. This is not an IncusOS or production-version compatibility matrix.

The Linux-only `integration` build tag excludes live tests from `make test` and
`make ci`. Tests run sequentially so whole-garden discovery does not see another
suite's temporary resources. Each test uses a temporary client configuration
and an intentionally unusable default remote. It requires an explicit Unix
socket and never loads operator settings.

Coverage includes:

- Garden's connection boundary and project CRUD with ETags.
- Public CLI entry points with the empty compiled garden; fixture definitions
  enter through the same internal command dispatcher, with the real backend.
- Read-only plan and reference validation before any mutation.
- Owned project and stopped-VM creation, direct disk/NIC placement, and no-op apply.
- Description, CPU/memory, and metadata updates; preservation and explicit removal.
- Unmanaged project/VM refusal and whole-apply blocking on replacement requirements.
- A real concurrent VM edit after discovery: apply retains the completed project
  update, refuses the stale VM update, and a fresh invocation resumes safely.
- Reporting removed definitions as orphans while preserving their resources.
- Bootable Alpine VM restart, graceful stop/update, and failed-start recovery
  with matching config and unmanaged power. KVM is required; the test fails
  instead of silently skipping when the fixture or guest is unavailable.
- Adoption of existing project/VM resources without changing other properties.
- Confirmed orphan deletion, cancellation, and a concurrent edit during the
  confirmation prompt. A separately attached custom volume remains after the
  VM and its empty project are deleted.

CI builds a tiny blank qcow2 disk and imports it with metadata as a local VM
image. Its full fingerprint is the SHA-256 of the unified image tarball, following
[Incus image format](https://linuxcontainers.org/incus/docs/main/reference/image_format/).
This fixture tests stopped-VM operations without an image download. The restart
suite additionally downloads an Alpine 3.22 VM image from the official image
server, logs its resolved full fingerprint, and uses it only on the disposable
daemon. It does not validate NixOS, IncusOS, persistent-volume replacement, or
other storage drivers. Recording fakes cover all interruption phases and lost
success responses; the live suite also induces an actual QEMU start failure.

## Run against a disposable local daemon

Use a dedicated disposable Incus daemon with no valuable resources. Initialize
it separately with the `default` directory storage pool and `incusbr0` managed
bridge used by the fixture. Install QEMU and OVMF, import the blank image using
the workflow's fixture step, fetch its bootable Alpine image, enable KVM/vhost-vsock,
and give the test user access to the Unix socket.
Then run from the checkout root with the Go toolchain and C compiler required
for `make ci`:

```sh
GARDEN_TEST_INCUS_SOCKET=/var/lib/incus/unix.socket \
GARDEN_TEST_INCUS_IMAGE="$(sha256sum /path/to/garden-image.tar | cut -d ' ' -f 1)" \
GARDEN_TEST_INCUS_BOOT_IMAGE="$(incus image list garden-ci-boot --format=json | jq -er '.[0].fingerprint')" \
make test-integration
```

This command creates, starts, restarts, stops, and deletes disposable VMs/projects. The socket path must be explicit,
absolute, and an actual Unix socket; an absent or unusable socket fails the test
rather than skipping it. The test creates its own `INCUS_CONF` and
`INCUS_GLOBAL_CONF` directories and never loads the operator's client settings.
The path check does not prove the daemon is disposable: the caller must ensure
that. Do not point it at production.

In CI a failed job prints the daemon status and recent journal entries. Diagnose
installation/readiness failures before test failures. Rerun on a fresh runner
after a correction. A hard timeout or runner failure can prevent test cleanup;
the CI runner is disposable. After an interrupted local run, inspect and remove
only the `garden-ci-*` project created by that run on the disposable daemon.

Add future resource acceptance tests here as Garden capabilities are implemented.
Keep fast decision/failure tests with recording fakes; reserve the live suite
for behavior that needs the actual daemon.

Persistent-volume coverage also tests detached block adoption/growth and actual
filesystem bytes surviving a running-VM disk addition and explicit VM recreation.
See [Persistent volumes](volumes.md) for supported storage boundaries.
