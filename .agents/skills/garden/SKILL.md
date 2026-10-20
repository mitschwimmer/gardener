---
name: garden
description: Use, contribute to, extend, troubleshoot, and document Garden, the purpose-specific Go CLI in mitschwimmer/gardener for a personal digital garden on IncusOS. Use for Garden application guidance, repository changes, architecture and MVP decisions, Go development, CI failures, and Incus ownership or reconciliation questions. Maintain this skill as the sole project documentation for humans and agents.
---

# Garden

Use this skill as the common entry point for application users and contributors.
Keep Garden small: manage Incus infrastructure; leave guest deployment to
NixOS/Nix and secrets to SOPS + age.

## Establish current capabilities

Use the [first-iteration scope](references/first-iteration.md) as the implemented
resource boundary and [resource lifecycle](references/lifecycle.md) for explicit
adoption and confirmed orphan deletion. The CLI supports `plan`, `apply`, `adopt`,
and `delete`. The compiled garden starts empty. CPU/memory reconfiguration can
gracefully stop and restart VMs with [durable recovery](references/reconfiguration.md).
Independent [persistent volumes](references/volumes.md) support explicit VM
attachments, adoption, and detached block growth. Automatic replacement remains
unimplemented.

Inspect the checked-out code when diagnosing behavior. Use `go.mod`, `Makefile`,
and `.github/workflows/ci.yml` as the authoritative version and check definitions.
Infrastructure commands connect using the retained fixed remote
`garden`. Confirm that name and actual workload references before first live use.
Do not confuse daemon-free checks with live acceptance.

CI tests the connection boundary, resource lifecycle, and CLI plan/apply against
a disposable runner-local Incus daemon, including bootable Alpine VM restarts.
This does not validate NixOS guest boot or production IncusOS behavior. See the
integration reference for isolation and use.

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
| Manage persistent disks | [Persistent volumes](references/volumes.md) | Independent owned storage, safe changes, and preserved data |
| Decide resource or lifecycle behavior | [Architecture](references/architecture.md) | Ownership, data safety, scope, and recovery implications |
| Implement or review the first iteration | [First iteration](references/first-iteration.md) | Supported properties, execution/recovery contract, and acceptance boundary |

Load only the references relevant to the task. Resolve their paths relative to
this skill; execute repository commands from the checkout root. Ask for missing
failure output or target details only when they change the diagnosis or decision.
Do not require infrastructure credentials for a daemon-free development task.

## Maintain documentation exclusively here

- Extend this skill or its bundled references whenever behavior, usage, design,
  setup, or troubleshooting needs documentation. Do not create a parallel manual,
  `docs/` tree, or separate contributor guide.
- Keep README and AGENTS as discovery pointers. Keep substantive guidance here.
- Serve both perspectives: explain the task, prerequisites, exact commands,
  expected outcome, and recovery for users; explain code ownership, extension
  points, safety constraints, and verification for contributors.
- State what works today separately from intended behavior and open decisions.
  Verify instructions against code; update the skill in the same change as the
  behavior it explains.
- Keep each fact in one place. Add a direct link from this entry point when adding
  a reference, and add a contents list when a reference exceeds 100 lines.
- Use clear language, concrete examples, and concise task-oriented guidance.
  Capture verified troubleshooting findings so the next reader can act on them.
- Check frontmatter, relative links, and examples before publishing. Commit and
  sync the skill with the project so documentation travels with its code version.
