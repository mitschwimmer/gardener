# Application use and troubleshooting

## Build and run the current scaffold

Use a checkout of `mitschwimmer/gardener`, Go matching `go.mod` (currently 1.27.2),
and Make. Allow network access for the first dependency download. Build locally:

```sh
make build
./bin/garden -h
./bin/garden
```

Expect `make build` to produce `bin/garden`. Expect `-h` or `--help` to print usage
and exit successfully. Expect invocation without arguments to report that Garden
is being designed and exit successfully. Run the binary directly after building;
Make and Go are build requirements, not requirements for each invocation.

Do not configure infrastructure or supply credentials to try these commands.
The scaffold makes no Incus connection and performs no infrastructure operations.
There are no release artifacts or implemented resource commands yet.

## Diagnose current command behavior

| Symptom | Meaning | Next action |
| --- | --- | --- |
| `apply`, `adopt`, or `delete` returns an unknown-command error | The resource MVP is not implemented | Consult the architecture reference; do not expect infrastructure changes |
| An option such as `--remote` is rejected | The CLI supports help only | Run `garden -h`; the planned remote is fixed in Go |
| `bin/garden` is missing | No local binary has been built, or the working directory differs | Run `make build` from the repository root |
| Go reports an unsupported version | The installed toolchain does not satisfy `go.mod` | Install the declared Go version and retry the build |
| Initial dependency retrieval fails | The build needs access to the Go module proxy and checksum service | Inspect the actual network/proxy error; retry once access is restored |

For an unexplained failure, record the exact invocation, output, exit status, OS,
and checkout commit (`git rev-parse HEAD`). Remove credentials or secrets from any
shared output. Trace argument handling in `internal/cli` and exit behavior in
`cmd/garden`; use the development reference when fixing code.

## Prepare for future infrastructure use

Distinguish the target design from current functionality. Garden is intended to
reuse existing Incus CLI configuration and authentication. The unused connection
scaffold currently requires the fixed remote `garden` and does not fall back to
the CLI default. Confirm that name before enabling resource commands.

Keep NixOS/Nix deployment and SOPS + age independent. Do not put secrets into
Garden definitions or invent secret-delivery steps as part of using this scaffold.
Add verified configuration, apply, adoption, deletion, and recovery procedures to
this reference as those capabilities become available.
