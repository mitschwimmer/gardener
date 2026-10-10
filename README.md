# Garden

Garden is a small Go CLI for managing a personal digital garden on IncusOS.
Incus is the sole source of infrastructure state. Guest configuration and
services remain independently deployed through NixOS/Nix; SOPS + age remains
responsible for secrets.

## Status

Project setup only. The CLI exposes help and rejects infrastructure commands.
The resource MVP must be agreed before reconciliation is implemented.

The only direct application dependency is the official Incus Go module,
currently pinned to `github.com/lxc/incus/v7 v7.5.1`. Its exported
`shared/cliconfig` package provides existing CLI configuration and authentication.
The connection boundary compiles against that API but is not called by the CLI.
The fixed remote name is provisionally `garden`; confirm it before enabling
infrastructure commands. No connection is made during tests or checks.

## Development

Install Go **1.27.2**, Make, and a C compiler (for Go's race detector).
No Incus daemon, Nix installation, infrastructure credentials, or secrets are
needed. Initial dependency downloads and vulnerability scans need network access.

```sh
make test                 # Fast inner loop; Go caches unchanged tests
go test ./internal/cli -run TestHelp -count=1
make fmt
make check                # Formatting, modules, vet, race tests, build
make ci                   # Same complete checks as GitHub Actions
./bin/garden -h
```

`make ci` installs the pinned development tool `govulncheck v1.8.0` into
`.tools/bin`. It does not add an application dependency. Build output and local
tools are ignored by Git. Change the Go version in `go.mod` to update the CI
toolchain; update the local installation to match.

## Repository layout

| Path | Responsibility |
| --- | --- |
| `cmd/garden` | Process entry point and exit status |
| `internal/cli` | Argument handling and output |
| `internal/incusconn` | Thin official-client connection boundary |
| `docs/architecture.md` | Architecture handoff and agreed testing boundaries |
| `.github/workflows/ci.yml` | Daemon-free GitHub checks |

Add definition and reconciliation packages when the MVP is agreed. Avoid empty
package trees, generic providers, and dependency graphs.

## Testing

Trust upstream Incus client/server contracts. Test Garden's definition validation,
ownership decisions, partial property management, fixed phases, failure handling,
and idempotency with ordinary Go tests and small recording fakes. Do not reproduce
Incus or test upstream HTTP/client interactions.

GitHub runs `make ci` for PRs, pushes to `main`, and manual dispatch. Actions are
pinned by commit, repository permissions are read-only, and the workflow needs no
repository secrets. Stable check name: **Go checks**. Branch protection is not
configured by this setup.
