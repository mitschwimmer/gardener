# Garden

Garden is a small Go CLI for a personal digital garden on IncusOS. It manages
Incus infrastructure; native NixOS/Nix tools deploy guest systems over direct SSH.
Gardener has not been deployed. The target is a fresh installation, with no
legacy deployment or state migration workstream.

The [Garden skill](.agents/skills/garden/SKILL.md) explains architecture, operation,
development, and recovery. This README owns the plan, experiments, and progress.
Read the [target architecture](.agents/skills/garden/references/architecture.md)
and [NixOS workflow](.agents/skills/garden/references/nixos-workflow.md) before
extending provisioning or deployment behavior.

## Current status

The infrastructure foundation works; the native NixOS provisioning/deployment
workflow is not implemented or live-validated yet. The compiled garden is empty.
See [implemented capabilities](.agents/skills/garden/references/first-iteration.md)
for exact limits.

| Area | Progress and evidence |
| --- | --- |
| Projects and VMs | Implemented; ownership, plan/apply, adoption, and confirmed orphan deletion tested against disposable Incus |
| VM changes | CPU/memory and added disks use graceful stop/update/start with durable recovery; Alpine live tests and interruption tests pass |
| Persistent volumes | Shared filesystem/block volumes, adoption, metadata, detached dir-driver block growth; live bytes survive VM deletion/recreation |
| Native bridges | Shared bridges, adoption, metadata, topology refusal; real guest IPv4 DHCP tested |
| Operator diagnostics | Confirmed progress, last observed state, uncertain outcomes, scoped inspection, command-specific retry |
| Development | Locked Nix shell; both Linux definitions evaluate, x86_64 shell runs `make ci`; ordinary Go and live Incus CI also pass |
| NixOS target workflow | Planned; no NixOS image/bootstrap/SSH deployment experiment has run |
| IncusOS acceptance | Not run; Ubuntu Incus with Alpine fixtures is the current live baseline |

## Next action

Run **E1** on an explicitly disposable Incus daemon before implementing a bootstrap
interface. Establish what the selected NixOS container image actually provides,
then prove the smallest direct-SSH path with native Nix tools. Do not start this
experiment as part of a documentation change.

## Implementation backlog

All entries below are **not started**. Keep procedures in the linked skill;
update this table as implementation and evidence become available.

| ID | Work | Completion criterion |
| --- | --- | --- |
| I1 | NixOS system containers alongside VMs | Typed instance kinds, unprivileged container settings, type-specific updates, lifecycle/ownership tests, and live container acceptance; informed by E1 |
| I2 | Explicit resource namespaces | Project-local images/volumes, shared networks, and managed guests in `default` without claiming the default project; preserve declared namespace choices; validate with E3 |
| I3 | Pinned base-image acquisition and retention | Trusted source/type/architecture/fingerprint, explicit local availability and recovery retention; no moving-alias refresh or automatic guest replacement; informed by E1 |
| I4 | Minimal explicit SSH bootstrap | Public access configuration, persistent host identity, create-only key generation, verified direct SSH, resumable failure output; no deployment/secret engine; E1 and E2 pass |
| I5 | Actual homelab declarations and required network/device support | Map real systems to typed Go infrastructure declarations and per-system NixOS directories; implement only required NIC/static-address/ACL/device capabilities; E3 proves administrative reachability and sandbox isolation |
| I6 | Direct OCI workloads where suitable | Pinned images, explicit runtime/public configuration, protected state and native secret-file procedures; define remaining details through E4 |

Keep whole-garden infrastructure reconciliation. Per-system working directories
scope native NixOS deployment, not a new Garden workload selector or separate
infrastructure state files. Existing pools remain host prerequisites.

## Experiment backlog

Every experiment is **not run**. Use only disposable resources. Record exact
pins, commands, environment, observations, and failures before changing status.

| ID | Question and required observations |
| --- | --- |
| E1 | Can a retained, fingerprint-pinned NixOS container image support minimal bootstrap and direct SSH? Verify image provenance/type/release, available tools, unprivileged boot, operator reachability, local build, native transfer/switch, reboot, and native generation rollback. Use one directory and one explicit configuration/target command. |
| E2 | Do identity and persistence fail safely? Missing data/key mounts prevent writers, key generation, and secret installation; repeat bootstrap preserves keys; changed SSH identity is rejected; interrupted activation can be inspected and recovered with native generations. Use disposable test keys/data only. |
| E3 | Does the real topology retain its boundaries? Verify project-local resources and a default-project guest; direct SSH from approved administrative sources; deny other callers and retain sandbox egress restrictions. Test the required NIC and ACL configuration on the disposable daemon. |
| E4 | What is the smallest native OCI runtime/secret contract? Verify the selected upstream image, pinned digest, supported public configuration, separate protected secret delivery, startup prerequisites, and persistence. Do not introduce a secret orchestrator. |
| E5 | Does the accepted path work on IncusOS? Repeat provisioning, direct SSH deployment, reboot, rollback, and persistence checks on an explicitly authorized disposable IncusOS target. VM-specific NixOS bootstrap/agent and IPv6 behavior require their own evidence if needed. |

E1 is the immediate design gate. E2 and E3 are required before declaring the
NixOS workflow ready for actual homelab use. E4 follows the NixOS foundation;
E5 establishes host-platform acceptance rather than replacing runner-local tests.
The directory convention replaces custom cross-system deployment checks; do not
add a wrong-closure rejection mechanism to these experiments.

## Progress log

| Date | Result | Evidence and limits |
| --- | --- | --- |
| 2026-10-20 | Infrastructure foundation completed through [PR #7](https://github.com/mitschwimmer/gardener/pull/7) | [Live CI](https://github.com/mitschwimmer/gardener/actions/runs/39462911189) includes Alpine VM restart/recovery, persistent bytes, and bridge DHCP; no NixOS/IncusOS acceptance |
| 2026-10-21 | Nix development shell completed in [PR #8](https://github.com/mitschwimmer/gardener/pull/8) | [CI](https://github.com/mitschwimmer/gardener/actions/runs/39622189217): both shell definitions evaluate, x86_64 Nix/Go checks and Incus tests pass; Go minimum raised to 1.27.3 |
| 2026-10-21 | Native NixOS target architecture agreed and documented | Direct SSH; one target directory; native build/transfer/switch/rollback; no custom deployment guard or proxy layer. Source/documentation review only; E1–E5 remain unrun. |

For each future result, distinguish source review, evaluation, build, daemon-free
tests, disposable live acceptance, and operator-host acceptance. A green current
CI does not complete an experiment that it does not exercise. Move reusable
findings into their owning skill reference and link them here; keep temporary
addresses, private keys, plaintext secrets, and host credentials out of the log.
