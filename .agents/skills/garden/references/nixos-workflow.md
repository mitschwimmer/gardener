# Native NixOS provisioning and deployment

## Contents

- [Status and responsibility](#status-and-responsibility)
- [Provision and bootstrap](#provision-and-bootstrap)
- [One directory per target](#one-directory-per-target)
- [Persistent state and secrets](#persistent-state-and-secrets)
- [Restart, diagnose, and roll back](#restart-diagnose-and-roll-back)

## Status and responsibility

This is the target operating procedure. NixOS container provisioning, image
acquisition, and explicit SSH bootstrap are not implemented yet. Follow the
[root README](../../../../README.md) for experiment prerequisites and evidence;
do not interpret this guide as completed acceptance testing.

Garden manages Incus infrastructure. Native Nix tools build, transfer, activate,
and roll back NixOS. The NixOS configuration remains in the homelab repository,
with one shared flake and lock. See [architecture](architecture.md) for boundaries
and [current CLI use](use.md) for the supported VM subset.

## Provision and bootstrap

Review `garden plan`, then use `garden apply` for supported declarations. These
commands reconcile the entire garden; the target-directory convention below
applies to NixOS deployment, not to selecting infrastructure resources.

For the target workflow, select a trusted upstream NixOS image by full immutable
fingerprint, guest type, architecture, and release. Keep the image locally
available and retain a recoverable copy; do not depend on an upstream server
retaining historical images. Never refresh a moving alias into an existing guest.
Containers and VMs require separate image and boot validation.

The future explicit bootstrap must establish only direct SSH management from
public configuration. Verify the intended Incus guest and its required mounts;
configure key authentication and generate unique host keys only when absent.
Never overwrite existing host keys. Read the host public key through the
authenticated Incus connection or another trusted channel and enroll it in the
operator's known-hosts file. Unchecked `ssh-keyscan` is not authentication.

The bootstrap interface will be selected after the disposable-image experiment.
There is no `garden bootstrap` command today. Incus exec is acceptable for this
bounded bootstrap or recovery; routine deployment uses direct SSH.

Expose SSH only to the intended administration sources, enforce network policy
at the host boundary, and disable password/keyboard-interactive authentication.
Preserve workload ingress and egress restrictions. Do not add an unrestricted
second NIC just to obtain management access. Use strict SSH host verification.

## One directory per target

Use `systems/NAME` in the homelab repository for each target. Work from that
directory and keep its name, flake attribute, and SSH destination aligned. Do not
introduce per-target flakes, duplicate lock files, selection wrappers, or
`target.json`. Shared NixOS modules remain shared.

For example, after provisioning and verified SSH bootstrap:

```fish
cd systems/webdav
nixos-rebuild switch --flake ../..#webdav --target-host root@webdav
```

This is a planned example, not an executed validation. With no build host
specified, the build runs locally; the native tool transfers and activates the
result on the remote target. Do not add a localhost SSH build hop. Use an
explicit flake attribute and destination in the same command instead of handing
an independently selected `result` symlink or store path to another step.

The directory is an operator convention, not a technical binding: an operator
can still enter mismatched names. We accept that limitation rather than maintain
custom cross-system artifact checks. SSH host verification authenticates the
remote machine; it does not prove that the selected NixOS configuration belongs
there. Review both names before activation. Key-only root SSH is the minimal
example; an unprivileged deployment account additionally needs native privilege
escalation configured and verified.

## Persistent state and secrets

Keep SSH host keys and SOPS/age machine keys unique and persistent. Garden handles
public bootstrap material, never private key delivery, secret decryption,
rotation, or application secrets. Use SOPS/age and sops-nix; keep plaintext secrets
out of the Nix store and shared diagnostic output.

NixOS must order every persistent-data writer after the actual required mounts:
services, database initialization, secret installation, key generation, and
tmpfiles. `RequiresMountsFor` supplies dependencies but does not alone prove that
a dedicated volume is mounted; a path can resolve on the root filesystem.
Combine dependencies with appropriate mount assertions and test missing-volume
behavior. No writer may initialize fallback state on the temporary root.
Retain the chosen `system.stateVersion` when configuration versions change;
updating nixpkgs is not a reason to change persisted-data compatibility settings.

## Restart, diagnose, and roll back

Validate a deployment, then test a restart in the disposable acceptance guest.
Use native SSH, `systemctl`, and `journalctl` for diagnosis. An activation failure
can leave partially changed service state; inspect the active system and profile
before choosing a native recovery action.

For an accessible target with an available previous system generation:

```fish
nixos-rebuild switch --rollback --target-host root@webdav
```

Confirm that the previous generation is the intended recovery point. Retain
known-good generations against garbage collection. Native generations belong to
the guest root; replacing that root does not preserve them automatically. If SSH
is unavailable, use the authenticated Incus recovery channel to inspect the guest
and select an available generation. VM boot-menu recovery and container recovery
need separate validation.

System rollback does not roll back databases or persistent application data.
Restore those using their own backup and recovery procedures. Garden provides no
`deploy` or `rollback` command, deployment database, custom closure transport, or
automatic application rollback.
