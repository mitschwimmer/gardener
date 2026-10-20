# Operator diagnostics after a failure

## Read execution output

Before mutation, Garden prints the complete plan. Each operation then prints
`Executing [N/total]: ...` before its first API action and `Completed: ...` only
when the whole operation succeeds. An error exits with status 1, stops later
operations, and keeps completed changes; no rollback is attempted.

Execution failures name the operation/resource, include the original Incus error,
and report the number of whole operations that completed. A reconfiguration may
have changed the VM even when that number is zero: it is one operation containing
multiple steps. Its failure also lists substeps for which Incus acknowledged
success and the last observed power/pending-power values. Those observations
precede the failure and are explicitly not a verified current state.

For example, a failed start during a CPU change can report:

```text
reconfigure VM apps/wiki (...) failed after 0 completed operations (1 planned): start VM: ...
Confirmed substeps within this reconfiguration: record pending power intent, stop VM, update VM.
Last observed VM state (before failure, current state unverified): power=Stopped, pending-power=running.
```

A lost response may mean that the failing action actually succeeded. Garden does
not assert that it failed without side effects, and does not make extra diagnostic
API requests after the error. Diagnose from fresh reads before retrying. Pending
intent and recovery follow the [reconfiguration contract](reconfiguration.md).

## Inspect and recover

The error prints read-only Incus commands for the affected resource on remote
`garden`, with an explicit project. VM commands include current information
and configuration; a separate optional log command may fail when an early QEMU
start failure has not created a log file (observed in live CI). Project and volume commands show the corresponding resource.
When an asynchronous operation was returned, its ID is retained through wrapped
errors and an `incus operation show` command is included. Request failures before
an operation is returned and synchronous project/volume requests have no operation
ID. Incus may already have expired an operation or removed a resource; a not-found
response is possible and does not erase previously completed work.

Run the printed diagnostic commands and review their output locally. Garden does
not automatically dump full configuration or logs into the failure message. Fix
the underlying cause, then follow the command-specific retry instruction:

- Reconciliation: `garden plan`, review the fresh plan, then `garden apply`.
- Adoption: repeat the exact `garden adopt ...` target after checking ownership.
- Deletion: check whether the target still exists; repeat the exact `garden delete
  ...` target only if deletion is still needed. Confirmation is required again.
  An already absent target needs no further deletion; do not recreate it to retry.

Retrying an adoption whose lost response succeeded is a no-op if the resource
still satisfies admission checks. Never clear pending power intent merely to
silence a failed restart. Inspect configuration and desired power first.

## Output failures and verification

If the initial plan or an `Executing` line cannot be written, Garden does not
start that operation. If a `Completed` line cannot be written, its operation has
already succeeded and the error says so. Failure to write the final summary
reports that all operations completed. Later work stops after output failures;
previous successful writes are retained. These distinctions prevent an output
problem being mistaken for an infrastructure failure.

Recording tests exercise every reconfiguration failure phase, lost responses,
error chaining/IDs, scoped and quoted commands, command-specific retries, and
output failures before/after mutation. The live CI deliberately causes a QEMU
start failure, checks the diagnostics and real operation ID, runs the printed VM
inspection command forms against its isolated remote, and verifies recovery.
