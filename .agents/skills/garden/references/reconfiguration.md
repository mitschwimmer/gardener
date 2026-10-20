# VM reconfiguration and interrupted restart recovery

## Behavior

`garden plan` reports a `reconfigure VM` operation when CPU/memory changes or added volume disks need
a running VM stopped, or when Incus records unfinished reconfiguration work.
`garden apply` performs the operation in these phases:

1. Record final power intent in `user.garden.pending-power` on the owned VM.
2. Gracefully stop if CPU/memory or declared disks must change or final power is explicitly stopped.
3. Update declared properties, preserving all unspecified configuration/devices.
4. Start if final power is running and the VM is stopped.
5. Clear the pending-power marker only after the preceding phases succeed.

Stops use a 30-second timeout, no force, and no saved state. A guest that cannot
honor graceful shutdown causes apply to stop; Garden does not escalate to a kill.
Description/metadata changes do not trigger a restart. A stopped VM with no
pending marker remains stopped when power is unmanaged. Creation and ordinary
explicit power changes retain their existing semantics.

Explicit running/stopped intent in the current definition wins. Otherwise the
operation preserves the original state of a running VM by recording `running`
before stopping. Existing pending intent also applies when power is unmanaged.
Definitions cannot write reserved `user.garden.*` keys.

## Recover after a failure

Read the [operator diagnostics](diagnostics.md): the error distinguishes confirmed
substeps and last observed state from the unverified outcome after failure.
Inspect the failing Incus operation and fix its cause, then rerun plan/apply:

```sh
garden plan
garden apply
```

Incus is the sole state source. A fresh apply combines the current definitions,
current configuration/power, and pending final power intent. There is no local
state file, rollback, daemon, scheduler, or background retry.

If stopping fails, the marker remains and the next apply retries remaining work.
If updating fails after a stop, the VM remains stopped with its marker. If the
update succeeds but start fails, the next apply still starts it even though the
desired config already matches and declared power is unmanaged. If start succeeds
but marker cleanup fails, the next apply clears the marker without another
restart. Lost success responses are handled by rediscovering what actually
completed. A valid new explicit power declaration overrides older pending intent.

Unknown marker values block the whole apply with a manual action. Do not clear a
marker merely to silence a failed restart: doing so discards recovery intent.
Review the desired end state first. Removed definitions remain preserved orphans;
Garden does not complete their pending work implicitly. Explicit confirmed
orphan deletion remains available under the lifecycle rules.

## Guards and limits

The first write requires unchanged discovered writable state and ownership.
Between phases Garden rereads ownership, project ownership, state, and writable
properties with fresh ETags. Concurrent user changes stop the operation and
leave recovery intent available. Incus may change volatile runtime bookkeeping
during power transitions; those changes are permitted, while the seed fingerprint
and an existing instance UUID remain protected. Resources are not replaced.

Every asynchronous operation is awaited. Garden trusts Incus operation completion;
it does not wait for the guest agent, application readiness, or service health.
The plan lists one reconfiguration operation; errors identify the failing phase,
and the pending marker tracks unfinished internal work. These are single-operator
safeguards rather than transactions or distributed locks.

Adoption refuses unowned resources carrying a nonempty pending-power marker.
Resolve the old intent before taking ownership to avoid inheriting an unintended
start. Already owned resources can resume through ordinary apply.

## Validation

Recording fakes cover phase order, all interruption points, lost success
responses, unmanaged power recovery, explicit overrides, metadata/device
preservation, invalid markers, ETags, graceful power requests, and concurrent
changes. The live CI uses a bootable Alpine VM to exercise restart after CPU
changes, stop/update with explicit stopped intent, and a genuine failed QEMU
start followed by recovery without a config diff. The test alone waits for its
guest agent before requesting graceful shutdown; that polling is not app behavior.

The [pinned QEMU driver](https://github.com/lxc/incus/blob/v7.5.1/internal/server/instance/drivers/driver_qemu.go)
establishes runtime volatile bookkeeping and power behavior. The
[official client](https://github.com/lxc/incus/blob/v7.5.1/client/incus_instances.go)
provides ETag-aware updates and asynchronous state operations. Production
IncusOS/NixOS and additional storage drivers remain separate acceptance work.
