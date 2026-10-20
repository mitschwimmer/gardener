# Live Incus integration tests

## CI isolation and coverage

The **Incus integration** job in `.github/workflows/ci.yml` runs on a fresh
GitHub-hosted Ubuntu 24.04 runner for PRs, pushes to `main`, and manual dispatch.
It installs the distribution's `incus` and `incus-client` packages, starts the
daemon, waits for readiness, and initializes a minimal local setup. The runner
and all its Incus resources are discarded after the job. No credentials or
connection to the operator's garden are used. Repository access stays read-only;
checkout does not persist credentials. There is no network API listener.

Tests are compiled as the runner user, then only the test binary runs with sudo
to access the local Unix socket. They log the actual server version. The daemon
version follows Ubuntu's package updates; the official Go client remains pinned
in `go.mod`. This is not an IncusOS or production-version compatibility matrix.

The Linux-only `integration` build tag excludes live tests from `make test` and
`make ci`. The current test exercises Garden's `Connect` function using a
temporary CLI configuration and an intentionally unusable default remote. It
reads server information, creates a uniquely named project, verifies description
and user metadata, updates with an ETag, verifies the result, deletes the project,
and checks that it is absent. Cleanup is also registered for assertion failures.
No image download, instance boot, VM, or reconciliation is tested yet.

## Run against a disposable local daemon

Use a dedicated disposable Incus daemon with no valuable resources. Initialize
it separately and give the test user access to its Unix socket. Then run from
the checkout root with the Go toolchain and C compiler required for `make ci`:

```sh
GARDEN_TEST_INCUS_SOCKET=/var/lib/incus/unix.socket make test-integration
```

This command creates and deletes a project. The socket path must be explicit,
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
