# Application use and troubleshooting

## Contents

- [Build and preview](#build-and-preview)
- [Define the garden in Go](#define-the-garden-in-go)
- [Adopt and delete resources](#adopt-and-delete-resources)
- [Diagnose and recover](#diagnose-and-recover)

## Build and preview

Use a checkout, Go matching `go.mod`, and Make. Allow network access for initial
dependency retrieval. Build and inspect help without infrastructure credentials:

```sh
make build
./bin/garden -h
```

Run the built binary directly; Go and Make are needed only for building. There
are no release artifacts yet. Help and no-argument guidance exit successfully
without connecting to Incus.

For infrastructure commands, configure the authenticated Incus CLI remote named
`garden`. Garden never falls back to the default remote and explicitly selects
projects. Discovery needs read access to all projects, their instances, pools, and default-project custom volumes; apply
also needs write access for declared resources. Confirm the remote's target with
the operator before the first live apply.

```sh
./bin/garden plan
./bin/garden apply
```

Expect a summary of operations, preserved orphans, and manual actions. Plan makes
no mutations. Apply performs fresh discovery and prints its own summary; it does
not consume a saved plan. Errors/manual actions exit nonzero. A manual action
blocks all writes; an operation failure retains completed work and stops later
operations. Fix the cause and rerun apply to resume.

## Define the garden in Go

The initial `internal/garden/definitions.go` returns an empty garden. This reports
existing Garden-owned orphans and executes no operations. Add real definitions
there and rebuild. Use workload files for organization; always assemble the
entire garden. No runtime configuration file or workload selector exists.

For example, replace `Definitions` with this function after substituting the
actual VM seed fingerprint and existing pool/bridge names. The placeholder
fingerprint is deliberately invalid and prevents any connection until replaced.

```go
func Definitions() definition.Garden {
    return definition.Garden{
        Projects: []definition.Project{{Name: "apps"}},
        VMs: []definition.VM{{
            Project: "apps", Name: "wiki",
            Image: "REPLACE_WITH_FULL_FINGERPRINT",
            Pool: "existing-pool", Network: "existing-bridge",
            Description: definition.Value("NixOS wiki"),
            Config: definition.Properties{
                "limits.cpu": definition.Value("2"),
                "limits.memory": definition.Value("2GiB"),
                "user.note": definition.Value("personal garden"),
            },
            Power: definition.Running,
        }},
    }
}
```

Import `github.com/mitschwimmer/gardener/internal/definition` as the starter file
does. The seed must already exist as a VM image in the remote's `default` project.
Prepare images, pools, and networks independently; Garden does not import or
manage them in this iteration. Keep NixOS/Nix deployment and SOPS + age separate.

Omit config keys to preserve them; set a key's value to nil for explicit removal.
For descriptions, nil preserves and `definition.Value("")` clears. Omit power
to leave existing power state unmanaged. New VMs start stopped unless running
is declared. See the first-iteration reference for supported property constraints.

See [Persistent volumes](volumes.md) for independent data volumes and VM disks.

## Adopt and delete resources

Use [Resource lifecycle](lifecycle.md) for `garden adopt project NAME`,
`garden adopt vm PROJECT NAME`, and confirmed `garden delete` procedures,
including prerequisites, volume preservation, refusal conditions, and recovery.

## Diagnose and recover

Use [Operator diagnostics](diagnostics.md) to interpret execution progress,
partial reconfiguration, diagnostic commands, and command-specific retries.

| Symptom | Next action |
| --- | --- |
| Missing binary or unsupported Go version | Build from the checkout root with the version declared in `go.mod` |
| Missing `garden` remote or authentication failure | Check the existing Incus CLI configuration; fix access to the intended remote |
| Invalid definition or missing reference | Fix the typed definitions or prepare the referenced image/pool/managed bridge |
| Unmanaged name collision | Review the lifecycle adoption procedure or choose a different name; do not bypass validation by manually adding ownership metadata |
| CPU/memory change on a running VM | Plan shows a reconfiguration; apply gracefully stops, updates, and restores intended power; see [VM reconfiguration](reconfiguration.md) |
| Image/device placement/project isolation differs | Review the manual action and use an explicit operator procedure; Garden never replaces resources |
| Resource changed since discovery | Review external changes and rerun plan/apply |
| Asynchronous operation or graceful stop fails | Fix the Incus operation failure and rerun apply; see pending-power recovery in [VM reconfiguration](reconfiguration.md) |
| Owned resource is an orphan | Restore its definition or follow the explicit deletion procedure; apply preserves it |
| Lifecycle command is rejected | Review declaration, ownership, compatibility, confirmation, and dependent-disk rules in the lifecycle reference |
| `--remote` or extra arguments are rejected | These interfaces are unsupported; consult `garden -h` |

For unexplained failures, record invocation, output, exit status, OS, and checkout
commit. Remove credentials/secrets from shared output. Use the development
reference for code and check failures. Complete the first-iteration live
acceptance procedure before relying on Garden for the operator's infrastructure.
