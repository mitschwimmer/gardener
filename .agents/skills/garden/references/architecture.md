# Target architecture and boundaries

## Contents

- [Purpose and responsibility](#purpose-and-responsibility)
- [Infrastructure model](#infrastructure-model)
- [Images and bootstrap](#images-and-bootstrap)
- [Native NixOS deployment](#native-nixos-deployment)
- [Identity, secrets, and persistence](#identity-secrets-and-persistence)
- [Execution and recovery](#execution-and-recovery)
- [Deliberate exclusions](#deliberate-exclusions)
- [Evidence and documentation](#evidence-and-documentation)

This is the agreed target, not a claim that every capability exists. Use
[implemented capabilities](first-iteration.md) for today's behavior and the
[root README](../../../../README.md) for the backlog, experiments, and progress.
Garden has not been used for a deployment; design for fresh installations.

## Purpose and responsibility

Build a small, purpose-specific Go CLI for the digital garden on IncusOS.
Prefer native Incus, NixOS, Nix, OpenSSH, and systemd mechanisms to custom tooling.
Use the Go standard library and the pinned official Incus Go client.

| Concern | Owner |
| --- | --- |
| Instances, projects, networks, devices, persistent volumes | Garden through the native Incus API |
| Base-image availability and retention | Garden and the Incus image store |
| Initial direct SSH access | Minimal explicit bootstrap, once implemented |
| NixOS build, closure transfer, activation, generations, rollback | Native Nix and `nixos-rebuild` |
| Guest services, SSH settings, mount guards | Declarative NixOS modules and systemd |
| Managed-secret encryption and runtime delivery | SOPS/age and sops-nix |
| Mutable data, backup consistency, external acceptance | Applications and service procedures |

NixOS configuration remains with the homelab source in `digital-garden`.
Garden owns infrastructure declarations, not a second service configuration.
Provisioning establishes a guest; successful deployment is not application health.

## Infrastructure model

Define the complete garden in typed Go, organized by workload with shared
infrastructure separate. Reconcile the entire garden using fixed resource phases,
not a generic dependency graph. Native NixOS deployment is independent per system;
working directories do not create Garden selectors or separate state databases.

Support NixOS system containers as the ordinary lightweight NixOS placement and
VMs where a separate kernel or device/isolation requirement warrants them.
Direct OCI is appropriate where its native image/configuration/secret contract
is simpler. Containers and OCI are target capabilities, not current commands.
Keep unprivileged containers unprivileged; any nesting/device access is explicit.

Declare namespace choices rather than flattening resources into `default`:
project-local images and volumes, shared networks where intended, and explicitly
owned guests in `default` without claiming ownership of that built-in project.
Existing pools are host prerequisites. Prefer direct devices over custom profiles.
Manage declared properties, preserve omitted properties, and allow explicit
removal where supported. Power intent is running, stopped, or unmanaged.

Use Incus ownership metadata. Unmanaged collisions fail; adoption is explicit.
Removed definitions become preserved orphans. Deletion is explicit, ownership
checked, and confirmed. Persistent volumes survive guest replacement. Never
replace guests or destroy storage automatically to make definitions converge.

## Images and bootstrap

Use suitable upstream NixOS images published for Incus, with the publisher and
trust source named accurately. Record immutable fingerprint, instance type,
architecture, and release. An alias may help select an image for review; runtime
creation uses the reviewed fingerprint in the intended local image namespace.
Retain the image or an export for recovery. Do not depend on an upstream server
retaining old builds or remaining available during normal operation.

Image acquisition and a changed creation-image pin do not update a running guest.
No system-specific seed archives, `NAME-image` outputs, `image_directory` pipeline,
or seed identity files are part of the target. Image retention is distinct from
ordinary NixOS configuration updates and recovery of application data.

Bootstrap is explicit and minimal: verify the selected Incus resource and its
persistent devices, establish public SSH access configuration and persistent host
identity, and verify direct SSH. Inspect the selected image's actual capabilities
before choosing bootstrap commands. Do not assume cloud-init or a running SSH
server. Do not define a bootstrap CLI interface until the disposable experiment
establishes the necessary operations. Ordinary infrastructure apply does not
implicitly deploy NixOS services or transfer private machine keys.

## Native NixOS deployment

Use direct OpenSSH. Do not introduce an Incus SSH proxy, deployment daemon,
profile-switch implementation, custom closure transport, or rollback database.
Incus execution/file access remains a bounded bootstrap and recovery mechanism.

Work from one `systems/NAME` directory per target. Keep a shared flake and lock
file, with deliberate package-pin exceptions where a system needs them. Spell out
the matching flake attribute and SSH destination, and use one `nixos-rebuild`
command to build locally, transfer, and activate. Do not select an arbitrary
`result` link or store closure for ordinary deployment.

This convention removes the artifact handoff that caused the earlier wrong-target
incident. It is an operator convention, not technical enforcement: the operator
can still type mismatched names. Accept that responsibility instead of preserving
custom cross-system closure checks, candidate/seed identity markers, duplicate
`target.json` deployment records, or a wrapper merely to bind these selections.
It does not weaken Garden's independent resource-ownership or storage guards.
See [native NixOS handling](nixos-workflow.md) for the operating procedure.

## Identity, secrets, and persistence

Allow SSH only from the intended administrative sources, using public-key
authentication and strict host verification. Preserve a distinct host identity
for each logical guest on protected storage. Bootstrap generates keys only for
an absent new identity after checking the real mount; it never overwrites an
existing identity. Enroll public host keys through the authenticated Incus channel
or another trusted out-of-band observation, not unchecked network key scanning.

Direct SSH must not bypass sandbox isolation. Any route or ACL permitting
administration is explicit and host-enforced; preserve restricted egress and do
not attach an unrestricted second NIC for convenience. Guest SSH authentication
and the network exposure boundary are separate requirements.

Keep machine age keys and their intended recipients stable. Garden does not
read, decrypt, deliver, rotate, or log private keys or application secrets.
SOPS/sops-nix and separate operator procedures own their installation and use.
Public SSH keys/access configuration are not secret delivery.

NixOS/systemd must prevent all relevant writers from starting when required
persistent mounts are absent: services, database initialization, secret setup,
host-key generation, and relevant tmpfiles work. Directory existence alone is
insufficient. Use mount dependencies and actual-mount assertions/conditions;
verify the chosen mechanism on the real guest type. Preserve `system.stateVersion`
for a system's retained data; a new base image is not a reason to bump it.

## Execution and recovery

Preflight the whole garden before mutation. Display changes, execute each resource
phase in deterministic order, await Incus operations, and stop at the first failure.
Manual actions block the whole apply. Preserve completed work and rediscover on
retry; do not roll back infrastructure automatically. Incus is the sole
infrastructure state source. Assume one operator, without custom distributed locks.

Supported running-VM changes use [durable reconfiguration](reconfiguration.md).
Errors distinguish acknowledged progress and last observed state from uncertain
outcomes after lost responses; see [operator diagnostics](diagnostics.md).
No continuous reconciliation, background retry, or application-health polling.
Bounded bootstrap/readiness checks do not become service monitoring.

NixOS generations provide system rollback. Retain a known-good generation and
avoid garbage-collecting the recovery point before acceptance. Generation rollback
does not undo database migrations or restore data; service recovery stays explicit.
A replaced disposable root does not automatically retain its Nix store/generations.

## Deliberate exclusions

No OpenTofu lifecycle/state layer, generic providers/graphs, deployment framework
such as deploy-rs or Colmena, custom cross-system deployment guards, SSH proxy
layer, automatic guest replacement/image upgrades, secret orchestration,
automated backups, or arbitrary script runner. Add own logic only for a concrete
unmet requirement confirmed by experiment; never recreate native deployment in Go.

## Evidence and documentation

Keep fast checks daemon-free and use handwritten recording fakes at Garden's
operation boundary. Verify version-sensitive contracts against pinned upstream
source. Use actual disposable Incus guests for bootstrap, SSH, networking,
persistence, failure, and native deployment/rollback acceptance. Test containers
and VMs according to their different kernel/boot/agent requirements.

Ubuntu Incus/Alpine CI proves only its recorded resource behavior. It does not
prove NixOS bootstrap, direct SSH deployment, IPv6, or IncusOS acceptance. The
README owns experiment status and evidence; this skill owns reusable decisions
and procedures. Never turn an unrun experiment into documented working behavior.
