# Contributing, extending, and troubleshooting code

## Contents

- [Prepare the development loop](#prepare-the-development-loop)
- [Locate the change](#locate-the-change)
- [Extend behavior](#extend-behavior)
- [Diagnose local checks and CI](#diagnose-local-checks-and-ci)

## Prepare the development loop

On NixOS with flakes enabled, use the locked development shell:

```fish
nix develop -c fish
make test
make ci
```

Or run a single command: `nix develop -c make check`. The shell supports
`x86_64-linux` and `aarch64-linux` and provides Go 1.27, Gopls, Make, GCC for
CGO/race tests, Git, Fish, SOPS, and age. Launch your installed Helix from this
shell so it finds Gopls; use `hx --health go` to inspect editor integration.
SOPS/age remain separate operator tools; entering the shell decrypts nothing.
Incus services and credentials are not needed for the normal development loop.

`flake.lock` pins nixpkgs; `GOTOOLCHAIN=local` prevents Go from downloading another
compiler. Keep the selected Go version compatible with `go.mod` when updating
the lock using `nix flake update nixpkgs`. Go module dependencies still belong to
`go.mod`/`go.sum`, and normal Go caches remain outside the Nix store. The flake
provides a development shell, not a release package or automatic Go test checks;
`nix flake check` alone does not run the test suite. CI runs `make ci` in the shell
on x86_64 Linux; aarch64 shell execution is not currently tested in CI.

Outside Nix, use the existing toolchain workflow below.

Install Go matching `go.mod`, Make, and a C compiler for the race detector. Keep
the official Incus module as the only direct application dependency; its transitive
dependencies are expected. Use `shared/cliconfig` for existing CLI configuration
and authentication. Add another direct dependency only with a concrete
justification and scope agreement.

Do not provision Incus, infrastructure credentials, or secrets for the fast
local checks. Allow network access for initial modules, development-tool
installation, and the vulnerability scan. In the prepared agent workspace, source the ignored
`.tools/env.sh` if present to select the workspace toolchain and writable caches.
Use a normal Go installation elsewhere; do not commit workspace-specific settings.

```sh
make test
go test ./internal/cli -run TestHelp -count=1
make fmt
make check
make ci
```

Use `make test` for the fast inner loop; Go caches unchanged tests. Use a focused
`go test -run` invocation to investigate a case. Run `make check` for changes and
`make ci` before publishing, as required by the project's shared local/CI workflow.

Expect `make check` to verify formatting and modules, run vet and race tests, and
build the CLI. Expect `make ci` to additionally install the pinned `govulncheck`
development tool into `.tools/bin` and scan code. Treat it as a development tool,
not an application dependency. Keep tools, binaries, caches, and credentials out
of Git. Find the tool version in Makefile and the Go/Incus versions in `go.mod`.

## Locate the change

| Path | Responsibility |
| --- | --- |
| `cmd/garden` | Process entry point and exit status |
| `internal/cli` | Argument handling and output |
| `internal/incusconn` | CLI authentication, project selection, thin official-client adapter, stale-state guards, and operation waits |
| `internal/definition` | Typed garden model and daemon-free validation |
| `internal/garden` | Operator definitions, initially empty; workload organization |
| `internal/reconcile` | Whole-garden discovery decisions, fixed phases, summaries, and failure stopping |
| `flake.nix` / `flake.lock` | Pinned NixOS development tools; Make/Go still own checks and builds |
| `Makefile` | Shared development and CI commands |
| `.github/workflows/ci.yml` | Fast checks and isolated live Incus integration job |
| `.agents/skills/garden` | All substantive project documentation |

Read the first-iteration reference for supported properties and recovery behavior.
Avoid empty package trees, generic providers, and dependency graphs.

## Extend behavior

1. Read the architecture reference and establish the requested scope. Separate
   implemented behavior from proposed capability; agree scope extensions before
   expanding resource commands.
2. Identify the smallest affected package and the ownership, property-preservation,
   persistent-data, and interrupted-operation implications.
3. Implement explicit Go code. Keep the Incus adapter thin and use upstream types
   where practical. Manage only the properties Garden declares.
4. Test Garden's decisions and intended API calls with ordinary `testing` and
   small handwritten recording fakes. Inject operation outcomes and failures;
   avoid HTTP emulation, generated mocks, simulated servers, or tests of upstream
   internals. Keep these checks daemon-free; use the separate
   live job for Garden's actual API boundary and future resource acceptance.
5. Update the skill's application guidance, development guidance, or architecture
   reference for any changed behavior or newly resolved decision. Capture
   prerequisites, failure behavior, verification, and user-visible limits.
6. Run relevant checks and `make ci`. Review the diff for unintended dependencies,
   infrastructure changes, credentials, and duplicated documentation; sync with Git.

Use the architecture reference's validation requirements when adding reconciliation.
Do not infer safe storage changes or metadata support from mocks; establish those
contracts from official source/documentation and focused operator experiments.

## Diagnose local checks and CI

Reproduce the first failing CI check locally with `make ci`, then narrow to its
Make target or test. GitHub runs that command on PRs, pushes to `main`, and manual
dispatch. Expect the check name **Go checks**, read-only repository permissions,
commit-pinned Actions, and no repository secrets. The separate **Incus integration**
check installs Ubuntu's Incus package and runs against its local Unix socket.
Branch protection was not configured by the initial setup; inspect repository
settings before asserting it.

| Failure | Next action |
| --- | --- |
| Formatting check prints files | Run `make fmt` and review the resulting diff |
| `go mod tidy -diff` reports changes | Run `go mod tidy`, review dependency changes, and include the resulting `go.mod`/`go.sum` changes |
| Vet or compilation fails | Inspect the affected code and pinned Incus API; reproduce with `make vet` or `make build` |
| Race tests require CGO or a compiler | Install a C compiler and enable CGO; retain the race check |
| A unit test fails | Run that test with `-run` and `-count=1`; diagnose Garden behavior from its inputs and recorded calls |
| Vulnerability scan fails | Distinguish a scan/network failure from a reported reachable vulnerability; inspect the advisory/call trace and fix the relevant version or code |
| The prepared sandbox reports inaccessible Go roots or temporary paths | Source its local `.tools/env.sh`; check `GOROOT`, `GOCACHE`, `GOTMPDIR`, and `TMPDIR` point to available workspace paths |

Preserve successful operations when diagnosing apply failures. Do not add
automatic rollback, replacement, deletion, or live-garden access to make a test pass.
Record a confirmed diagnosis and recovery procedure in the appropriate skill
reference before considering a recurring problem resolved.
