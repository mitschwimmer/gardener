# Architecture and design boundaries

## Contents

- [Purpose and technology](#purpose-and-technology)
- [Resource ownership](#resource-ownership)
- [Declarative model](#declarative-model)
- [Apply behavior](#apply-behavior)
- [Safety and lifecycle](#safety-and-lifecycle)
- [Images and secrets](#images-and-secrets)
- [Non-goals and priorities](#non-goals-and-priorities)
- [Agreed validation approach](#agreed-validation-approach)
- [Follow-on decisions](#follow-on-decisions)

Use [First iteration](first-iteration.md) for the implemented scope. This
reference describes the broader design, including later lifecycle capabilities.

## Purpose and technology

Build a small, purpose-specific Go CLI for a personal digital garden on IncusOS.
Replace OpenTofu for Incus resource management without becoming a general-purpose
infrastructure framework. Prefer simplicity, explicit behavior, and native Incus.

- Go standard library and official Incus Go client, plus its transitive dependencies.
- IncusOS infrastructure host; Incus is the sole infrastructure state source.
- Existing Incus CLI remote configuration and authentication; remote name fixed in Go.
- NixOS/Nix guest configuration and services, independently deployed.
- SOPS + age secrets, independently managed.

## Resource ownership

Use Incus `user.*` metadata where supported. Garden only modifies resources it
owns. Adoption explicitly validates and adopts existing resources. Unmanaged
name collisions are errors. Managed resources absent from definitions are
reported as orphans, never automatically deleted. Deletion requires explicit
invocation, ownership verification, and confirmation. No separate state database.

The implemented project/VM subset and its conservative admission/deletion rules
are documented in [Resource lifecycle](lifecycle.md). Independent custom volumes and
their current safe-update boundary are documented in [Persistent volumes](volumes.md).
[Managed bridge networks](networks.md) covers the shared network resource boundary.

## Declarative model

Define infrastructure in typed Go, organized by workload with shared infrastructure
separate. Always reconcile the entire garden. Explicitly define projects and
resource assignments; support shared and project-specific infrastructure.
Prefer direct instance configuration over custom profiles. Manage only declared
properties, preserve unspecified properties, and support explicit property removal.
Power state is optional: running, stopped, or unmanaged.

## Apply behavior

1. Discover actual resources.
2. Check ownership, references, and basic compatibility before mutation.
3. Display a change summary.
4. Create missing resources and reconcile safe in-place changes.
5. Automatically stop/update/start VMs for CPU/memory changes, preserving power
   intent with the [reconfiguration recovery contract](reconfiguration.md).
6. Report completed operations, orphans, and required manual actions.

Use fixed phases by resource type, not a generic graph. Idempotency is mandatory.
Stop on the first failure, retain successful operations, and resume on the next
apply. Wait for asynchronous Incus operations to complete; then trust their results.
No additional runtime or application health checks. Assume one operator; no
custom locking, daemon, scheduler, or background reconciliation.

## Safety and lifecycle

Never automatically delete removed definitions or replace instances. If replacement
is required, report it and stop for an explicit operator procedure. Preserve
persistent volumes across instance replacement. Apply supported safe in-place
storage changes; destructive storage changes require operator intervention.
No transactions, rollback, or replacement orchestration.

## Images and secrets

Prefer immutable image references. NixOS seed images are separate from ongoing
NixOS updates. No automatic OCI image upgrades, version tracking, or rollback.
Replacement follows explicit operator procedures. Incus retrieves and caches images.

Garden does not decrypt or deliver secrets. SOPS + age remains authoritative;
NixOS guests use existing Nix/sops-nix mechanisms. Determine a simple secure,
reproducible secret-delivery mechanism for direct OCI workloads independently of
Garden. Consider persistence and visibility before putting plaintext in Incus config.

## Non-goals and priorities

No Terraform/OpenTofu reimplementation, general-purpose providers or graphs,
separate state, continuous reconciliation, automatic replacement/rollback,
application deployment/monitoring, NixOS deployment orchestration, secret delivery
or rotation, automated backups, or arbitrary shell-script orchestration.

Priorities, in order: small implementation and maintenance burden; predictable
idempotent behavior; protection of unmanaged resources and persistent data; clear
responsibility boundaries; reproducibility; explicit intervention for destruction.

## Agreed validation approach

Keep the fast checks daemon-free. Use the standard Go test framework and small
handwritten recording fakes at Garden's operation boundary for decisions and
failure injection. Avoid HTTP emulation, generated mocking frameworks, or a
simulated Incus server.

A separate CI job runs opt-in integration tests against an actual disposable
Incus daemon on its GitHub-hosted runner. It tests Garden's connection boundary
and CLI project/VM plan/apply, including bootable VM restarts. Use that isolated
daemon for live acceptance tests as capabilities are implemented; never use the
operator's infrastructure or credentials. Ubuntu's packaged Incus is a baseline,
not proof of IncusOS, NixOS guest boot, other storage drivers, or production-version compatibility.

Cover definition/reference validation, ownership refusal, property preservation
and removal, project/resource identity, orphan reporting, safe-update/restart
decisions, replacement refusal, fixed phases, first-failure stopping, resume,
idempotency, summaries, confirmation, and exit status as those features are added.

Verify assumptions about metadata, restart requirements, and storage capabilities
from upstream documentation/source and focused operator experiments during design.
Compilation checks compatibility with the pinned official client API.

## Follow-on decisions

- Complete first live acceptance for the agreed project/VM MVP, including the
  operator's remote name and actual workload/image/network/pool references.
- Review official client capabilities and metadata support before adding resource types.
- Extend the VM reconfiguration/recovery contract deliberately when adding
  further managed properties or instance types.
- Define explicit replacement procedures that preserve persistent volumes.
- Resolve OCI secret delivery outside Garden.

Agree scope extensions before expanding resource or lifecycle commands.
