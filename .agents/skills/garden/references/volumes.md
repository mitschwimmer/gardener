# Persistent custom volumes

## Define and attach

Garden manages independent `custom` volumes in Incus's `default` project. All
managed workload projects share this namespace (`features.storage.volumes=false`).
Pools must already exist and be ready. Declare volumes separately from VMs:

```go
Volumes: []definition.Volume{{
    Pool: "existing-pool", Name: "wiki-data", ContentType: "filesystem",
    Description: definition.Value("Persistent wiki data"),
}},
// Inside the corresponding VM definition:
Disks: []definition.Disk{{
    Name: "data", Pool: "existing-pool", Volume: "wiki-data", Path: "/data",
}},
```

Use a unique absolute non-root path for filesystem disks. A block volume instead
needs `ContentType: "block"`, an explicit positive `Size: "1GiB"` (MiB or GiB),
and an empty disk path. Block creation and managed growth currently require the
`dir` storage driver, whose behavior is covered by live CI. Filesystem quotas,
other block drivers, isolated volume namespaces, snapshots, and volume deletion
are not managed. Garden does not format block devices or resize guest filesystems;
configure guest storage independently through NixOS/Nix.

Each disk must reference a declared volume, and each volume may be declared on
only one VM. References by another instance or profile block attachment. Existing
attachment to the target VM is allowed. Garden checks ownership, content type,
and usage again before creating/updating/starting the VM and between restart
phases. These checks assume one operator; they do not provide atomic cross-resource
locking. Disk names `root` and `eth0` are reserved.

## Plan and apply

Fixed phases are projects by name, volumes by pool/name, then VMs by project/name.
Volume creation writes `user.garden.owner=garden`. Description and nonreserved
`user.*` metadata reconcile with the same omission/nil-removal rules as VM metadata.
Undeclared storage configuration is preserved. Equal byte sizes are a no-op even
when MiB/GiB spelling differs.

Garden supports detached block growth. Shrinkage, content-type changes, and growth
of a referenced volume require manual intervention and block the whole apply.
Detach externally under an explicit operator procedure before growing; removing
a disk definition alone does not detach it. Garden never changes existing disk
bindings, paths, or dependent-volume semantics automatically. Missing disks are
added; a running VM uses the [durable restart procedure](reconfiguration.md).

Failures retain completed operations. Fix the reported cause and rerun plan/apply.
If volume creation succeeds and a subsequent VM operation fails, the next apply
reuses the owned volume. No rollback or replacement is performed.

## Adoption and data preservation

An existing unowned name blocks apply. Declare the reviewed compatible volume,
then run:

```sh
garden adopt volume POOL NAME
garden plan
garden apply
```

Adoption changes only the owner marker; description, metadata, and size converge
on the following apply. Different owners, incompatible content, unsafe sizes,
and unsupported pools are refused. The update rereads the complete writable state
and current references, checks unchanged discovery, and passes the current ETag.

Removing a volume definition reports an orphan and preserves the volume. Remove
its disk definition too for definition validation. Omitting a disk preserves its
existing device. Confirmed deletion of a stopped VM preserves independent custom
volumes; dependent disks are refused by the lifecycle command. To recreate a VM,
retain its volume definition, explicitly delete the stopped VM orphan, then restore
the VM definition and apply. There is no `garden delete volume` command. Review
backups and use Incus explicitly for destructive storage procedures.

## Evidence and limits

Daemon-free tests cover definitions, phases, recovery, metadata preservation,
adoption, ownership and stale-state guards, ETags, exclusive use, manual-action
blocking, and disk-binding preservation. Live CI creates and grows a detached
block volume, adopts without changing its settings, and refuses shrinkage. A
bootable Alpine VM receives a filesystem volume through a running-VM restart,
writes bytes, is explicitly deleted/recreated, and reads the same bytes. Omitting
the declarations also leaves the mounted data readable.

The pinned [storage API](https://github.com/lxc/incus/blob/v7.5.1/shared/api/storage_pool_volume.go),
[official client](https://github.com/lxc/incus/blob/v7.5.1/client/incus_storage_volumes.go),
[dir driver](https://github.com/lxc/incus/blob/v7.5.1/internal/server/storage/drivers/driver_dir_volumes.go),
and [disk device](https://github.com/lxc/incus/blob/v7.5.1/internal/server/device/disk.go)
establish the contracts used here. CI checks Ubuntu's packaged Incus; production
IncusOS/NixOS and other storage drivers still require separate acceptance.
