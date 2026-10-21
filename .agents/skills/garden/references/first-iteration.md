# First iteration: owned projects and NixOS VMs

## Outcome and scope

Provision a NixOS VM in an explicitly declared project using typed Go definitions.
Prove ownership refusal, property preservation, idempotency, orphan reporting,
and recovery from partial failure before expanding the resource model.

The implementation supports multiple declared projects and VMs; the first live
acceptance exercise should use one of each. Assemble the complete garden in
`internal/garden/definitions.go`, with workloads in separate Go files as needed.
The checked-in garden is empty: no example workload is active.

[Resource lifecycle](lifecycle.md) extends this declaration/reconciliation scope
with explicit adoption and confirmed orphan deletion.
[Persistent volumes](volumes.md) extends it with independent storage and VM disks.
[Managed bridge networks](networks.md) adds declared shared networks.

| Area | Implemented boundary |
| --- | --- |
| Commands | `garden plan` is read-only; `garden apply` rediscovers, validates, summarizes, and executes the entire garden |
| Projects | Create owned projects; reconcile an explicitly declared description |
| Project infrastructure | Share images, networks, profiles, and custom-volume namespaces with `default`; feature choices are creation-only |
| VMs | Create non-ephemeral, non-stateful VMs with no applied profiles |
| Seed image | Full lowercase 64-character fingerprint of an existing VM image in the remote's `default` project |
| Devices | Direct `root` disk on an existing ready pool; direct `eth0` NIC on an existing ready managed bridge in `default` |
| Config | Positive CPU counts, positive memory values in MiB/GiB, and `user.*` metadata outside reserved `user.garden.*` |
| Updates | Description/metadata updates and automatic graceful stop/update/start for CPU/memory changes |
| Power | Optional running/stopped intent; unset preserves power, including durable restart recovery |
| Ownership | `user.garden.owner=garden` on projects and instances; unmanaged collisions fail |
| Removal | An omitted config key is preserved; a nil config value removes it; a nil description is preserved and an empty description clears it |
| Orphans | Report owned projects and instances absent from definitions; never delete |

Image fingerprint, project feature settings, root placement, and NIC placement are
creation-only. Differences produce manual actions, including when an instance
would need rebuilding or replacement. Unsupported profiles, ephemeral/saved-state
instances, and transient/error power states also require manual intervention.
Unspecified config, devices, and writable fields survive supported updates.

## Execution and recovery

Use fixed phases: all project operations in name order, networks in name order,
custom volumes in pool/name order,
then each VM's create or
update and optional power operation in project/name order. Complete definition,
ownership, reference, and compatibility validation before mutation. A manual
action blocks the whole apply, including otherwise safe operations.

CPU/memory changes use [VM reconfiguration](reconfiguration.md). Stops are graceful,
with a 30-second timeout and no force or saved state. Incus stores pending final
power before stopping, allowing later applies to resume even when config matches
and declared power is unmanaged.

Wait for each asynchronous Incus operation. Stop on the first error, retain
successful work, and rediscover on the next invocation. A failed start remains
recoverable because the desired running state still differs from actual state.
Existing VMs do not need their cached seed image to remain available; compare
their original `volatile.base_image` fingerprint instead.

Before updates, reread the resource, verify ownership and unchanged writable
state, and pass the current ETag to Incus. Before VM creation, recheck the project
ownership and feature settings. Before power changes, verify instance ownership,
non-ephemeral/non-stateful VM lifecycle, and expected power state. Assume one
operator; these checks are not a distributed locking protocol.

## Deferred capabilities

Do not add managed pools, custom
profiles, OCI workloads, replacement, or image upgrades to
this iteration. Keep NixOS deployment, SOPS + age, backups, secrets, and health
checks independent. Broader architecture goals remain in `architecture.md`.

## Validation and first live acceptance

Use daemon-free recording fakes for Garden decisions and exact official-client
requests. Cover invalid references, ownership, project/resource identity, fixed
phases, preservation/removal, manual-action blocking, operation/output failures,
resume, orphan reporting, idempotency, ETags, and operation waits. Run `make ci`.

The pinned Incus source establishes these assumptions:

- [Project configuration](https://github.com/lxc/incus/blob/v7.5.1/cmd/incusd/api_project.go): `user.*` metadata and feature validation.
- [Project namespaces](https://github.com/lxc/incus/blob/v7.5.1/internal/server/project/project.go): shared images/networks/profiles/custom volumes.
- [Instance creation](https://github.com/lxc/incus/blob/v7.5.1/internal/server/instance/instance_utils.go): original image in `volatile.base_image`.
- [Official client](https://github.com/lxc/incus/blob/v7.5.1/client/interfaces.go): project selection, reads, updates, ETags, and operation contracts.

The automated [live integration suite](integration.md) exercises CLI plan/apply,
project and stopped-VM creation, a no-op second apply, stopped CPU/memory updates,
metadata preservation/removal, ownership refusal, replacement blocking, recovery
after a stale-state partial failure, and orphan preservation against Ubuntu Incus.
It uses a local blank disk for stopped-VM tests and a bootable Alpine image for
restart, graceful-stop, and failed-start recovery tests. Neither is a NixOS seed.

Before first operator use, confirm the retained remote name `garden`, the first
workload, and actual seed/pool/bridge references. This checkout has not been
validated against the operator's IncusOS daemon. In a disposable project, verify
NixOS seed creation and requested power, including graceful stop and failed-start
recovery. Record confirmed findings here; do not treat stopped-VM coverage as
guest boot acceptance.
