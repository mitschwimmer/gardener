# Explicit project and VM lifecycle

## Adopt a compatible existing resource

Use the authenticated `garden` remote and the compiled Go definitions described
in [Application use](use.md). Adoption is explicit and targeted. Declare the
resource first and rebuild the binary. Then adopt the project before its VMs:

```sh
garden adopt project apps
garden adopt vm apps wiki
garden plan
garden apply
```

Adoption validates the target and writes only `user.garden.owner=garden`. It
preserves descriptions, metadata, configuration, devices, and power state. A
following plan/apply reconciles declared differences separately. Repeating
adoption of an already owned, compatible resource succeeds with zero operations.
Other still-unadopted garden resources do not block adopting one target at a time.

The project must have `features.images`, `features.networks`, `features.profiles`,
and `features.storage.volumes` explicitly set to `false`, matching Garden's shared
default-project namespaces. Garden does not change project isolation during
adoption. A VM requires an already owned project and the first-iteration VM
compatibility rules: non-ephemeral VM without saved state or applied profiles,
running or stopped, matching original seed fingerprint, root pool, and managed
bridge placement. Existing pool/bridge references must still be ready. Its seed
image need not remain cached. A different nonempty owner is always refused.

An undeclared or incompatible resource is refused before mutation. Resolve the
reported mismatch independently or choose a different definition; adoption does
not rebuild, move storage, restart, convert, or silently modify the resource.

## Delete an owned orphan

Remove the resource from the compiled definitions and rebuild first. `garden
plan` reports the orphan. Apply continues to preserve it. Deletion requires an
explicit target and exact name confirmation from standard input:

```sh
garden delete vm apps wiki
# Type apps/wiki when prompted.
garden delete project apps
# Type apps when prompted, after all project resources have been removed.
```

VM deletion requires both the VM and its project to be Garden-owned. The VM must
be stopped, non-ephemeral, and without saved state. Garden does not stop it as a
side effect of deletion. The prompt states that the VM's root disk and snapshots
will be destroyed. Independently attached custom volumes are preserved. A disk
marked `dependent` is refused because Incus can delete that volume along with
the VM. This includes inherited expanded devices. Preserve that volume explicitly
and remove its dependency setting independently before retrying; Garden does not
detach devices or weaken dependency rules automatically.

Project deletion requires an empty owned project: no instances and no other
`used_by` resources. It never cascades into contained resources. The default
project, unmanaged resources, and resources still declared in Go are refused.
There is no force or confirmation-bypass option. Wrong input, EOF without a
name, input errors, and output errors before execution perform no mutation.

## Recovery and concurrency

Each command discovers current state and prints the intended operation. Writes
recheck ownership and writable state immediately before the API request. Adoption
uses current ETags. Deletion rechecks stopped state, dependent disks, or project
emptiness after confirmation. A concurrent edit requires rerunning the command;
deletion then asks for confirmation again. The official delete methods have no
ETag argument; these are single-operator safeguards, not atomic distributed locks.
Incus remains responsible for rejecting running/nonempty/protected resources.

Garden waits for asynchronous VM adoption/deletion operations. On an error it
stops and reports the failure; it does not roll back. Rediscover with plan and
inspect the resource before retrying. A completed adoption becomes a no-op on
retry. A completed deletion is reported as missing on retry. Restore a deleted
VM only through an explicit provisioning/recovery procedure and existing backups;
its preserved custom volumes remain independent of the deleted root disk.

## Validation and upstream contracts

Fast tests cover declaration and compatibility checks, ownership, preservation,
confirmation/I/O failures, stale-state refusal, dependent disks, project emptiness,
ETags, and operation waits. The live CI suite covers adoption, declined and
confirmed deletion, edits during confirmation, and custom-volume preservation
against the packaged Ubuntu Incus daemon. Dependent-disk rejection is also tested
with recording fakes against the pinned upstream contract; baseline Incus 6.0
does not establish compatibility with every newer Incus feature.

Pinned upstream sources:

- [Instance deletion](https://github.com/lxc/incus/blob/v7.5.1/cmd/incusd/instance_delete.go): rejects running instances and invokes dependent-resource cleanup.
- [VM driver deletion](https://github.com/lxc/incus/blob/v7.5.1/internal/server/instance/drivers/driver_qemu.go): deletes root storage and snapshots; dependent custom disks may also be removed.
- [Dependent disks](https://github.com/lxc/incus/blob/v7.5.1/internal/server/instance/drivers/driver_common.go): dependency cleanup includes expanded devices with a true `dependent` setting.
- [Official client](https://github.com/lxc/incus/blob/v7.5.1/client/incus_instances.go): instance deletion returns an operation and has no ETag parameter.
