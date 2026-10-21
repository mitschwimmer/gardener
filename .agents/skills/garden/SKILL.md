---
name: garden
description: Use, contribute to, extend, troubleshoot, and document Garden, the purpose-specific Go CLI in mitschwimmer/gardener for a personal digital garden on IncusOS. Use for infrastructure, native NixOS provisioning and deployment boundaries, Go development, CI failures, and resource lifecycle questions. Keep architecture and procedures in this skill; maintain the plan, experiments, evidence, and progress in the root README.
---

# Garden

Use this skill as the common entry point for application users and contributors.
Keep Garden small: manage Incus infrastructure and, once implemented, establish
direct SSH access through a minimal explicit bootstrap. Native Nix tools own
NixOS build, transfer, activation, and rollback; SOPS/age and sops-nix own secrets.
Use [architecture](references/architecture.md) for the target and
[native NixOS handling](references/nixos-workflow.md) for operating conventions.
The [root README](../../../README.md) owns plans, experiments, and progress.
Gardener is not yet in use; design for fresh installations, without a legacy
migration workstream. Planned behavior is not an available command.

## Establish current capabilities

Use [implemented capabilities](references/first-iteration.md) as the implemented
resource boundary and [resource lifecycle](references/lifecycle.md) for explicit
adoption and confirmed orphan deletion. The CLI supports `plan`, `apply`, `adopt`,
and `delete`. The compiled garden starts empty. CPU/memory reconfiguration can
gracefully stop and restart VMs with [durable recovery](references/reconfiguration.md).
Independent [persistent volumes](references/volumes.md) support explicit VM
attachments, adoption, and detached block growth. Automatic replacement remains
unimplemented. [Managed bridge networks](references/networks.md) support
creation, adoption, metadata updates, and direct VM references.

Inspect the checked-out code when diagnosing behavior. Use `go.mod`, `Makefile`,
and `.github/workflows/ci.yml` as the authoritative version and check definitions.
Infrastructure commands connect using the retained fixed remote
`garden`. Confirm that name and actual workload references before first live use.
Do not confuse daemon-free checks with live acceptance.

CI tests the connection boundary, resource lifecycle, and CLI plan/apply against
a disposable runner-local Incus daemon, including bootable Alpine VM restarts.
This does not validate NixOS guest boot or production IncusOS behavior. See the
integration reference for isolation and use.
Containers, project-local images/volumes, image acquisition, and SSH bootstrap
are not implemented yet. Do not describe existing Alpine VM tests as acceptance
of the native NixOS target workflow.

## Choose the relevant workflow

| Goal | Read | Result to establish |
| --- | --- | --- |
| Build, run, or use the application | [Application use](references/use.md) | Supported command, prerequisites, expected output, and limitations |
| Contribute or extend code | [Development](references/development.md) and [Architecture](references/architecture.md) | Small scoped change, relevant checks, and updated skill |
| Recover after an execution failure | [Operator diagnostics](references/diagnostics.md) | Confirmed progress, uncertain state, scoped inspection, and command-specific retry |
| Diagnose a CLI or configuration problem | [Application use](references/use.md) | Reproducible failure and explanation grounded in current behavior |
| Diagnose code, tests, or CI | [Development](references/development.md) | Failing check, cause, and verified fix |
| Run live Incus integration tests | [Integration tests](references/integration.md) | Disposable daemon, explicit socket, real API checks |
| Adopt or delete a resource | [Resource lifecycle](references/lifecycle.md) | Compatible declared adoption or confirmed deletion of an owned orphan |
| Change CPU/memory or recover a failed restart | [VM reconfiguration](references/reconfiguration.md) | Graceful stop/update/start and pending power intent in Incus |
| Manage VM networks | [Managed bridge networks](references/networks.md) | Shared native bridges, topology guards, and explicit adoption |
| Manage persistent disks | [Persistent volumes](references/volumes.md) | Independent owned storage, safe changes, and preserved data |
| Decide resource or lifecycle behavior | [Architecture](references/architecture.md) | Ownership, data safety, scope, and recovery implications |
| Provision or deploy a NixOS system | [Native NixOS handling](references/nixos-workflow.md) | Direct SSH, one directory per target, native deployment, persistent identity, and recovery |
| Choose the next task or record experiment results | [Root README](../../../README.md) | Ordered backlog, acceptance evidence, and honest progress |
| Check current implementation limits | [Implemented capabilities](references/first-iteration.md) | Supported properties, execution/recovery contract, and acceptance boundary |

Load only the references relevant to the task. Resolve their paths relative to
this skill; execute repository commands from the checkout root. Ask for missing
failure output or target details only when they change the diagnosis or decision.
Do not require infrastructure credentials for a daemon-free development task.

## Maintain the two documentation owners

- Keep architecture, behavior, usage, setup, recovery, and contributor procedures
  in this skill and its references. Do not create a parallel manual or `docs/` tree.
- Keep the plan, experiment backlog, progress, and validation evidence in the
  root README. Link to owning procedures instead of copying them. AGENTS remains
  a discovery and documentation-routing pointer.
- Serve both perspectives: explain the task, prerequisites, exact commands,
  expected outcome, and recovery for users; explain code ownership, extension
  points, safety constraints, and verification for contributors.
- State what works today separately from intended behavior and open decisions.
  Verify instructions against code; update the skill in the same change as the
  behavior it explains.
- Update README status when work is implemented or an experiment is run. Record
  what actually ran, its environment/revision, results, and remaining limits;
  source inspection, a successful build, and live acceptance are different evidence.
- Keep each fact in one place. Add a direct link from this entry point when adding
  a reference, and add a contents list when a reference exceeds 100 lines.
- Use clear language, concrete examples, and concise task-oriented guidance.
  Capture verified troubleshooting findings so the next reader can act on them.
- Check frontmatter, relative links, and examples before publishing. Commit and
  sync the skill with the project so documentation travels with its code version.
