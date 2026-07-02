# Write safety

Most of Northwatch is read-only. The `write` capability is the exception, and
because a mistaken write to the Northbound database can ripple out to every
chassis, writes are built around safeguards rather than exposed as plain CRUD.
This page explains the model; the mechanics are in
[Enable write operations](/how-to/enable-write-operations).

## Off by default

Writes do nothing until you pass `--write-enabled`. A server that was never told
to allow writes does not register the write routes at all, and the `write`
capability is absent. This makes "is this instance dangerous?" answerable from one
flag and one capability check.

## Plan, then apply

Writes are a two-phase workflow rather than a single mutating call:

1. **Preview / dry-run.** You submit the intended change and get back its predicted
   effect — a `terraform plan`-style description of what would change — *without*
   touching OVN. This is where you confirm the change does what you meant before
   anything happens.
2. **Apply a plan.** A prepared plan is applied by id. Plans expire after
   `--write-plan-ttl` (default `10m`), so a stale plan reviewed an hour ago cannot
   be applied against a deployment that has since moved on.

Separating the decision ("this is the change") from the action ("apply it now")
is what makes a preview meaningful: the thing you reviewed is the thing that runs.

Because a plan can be applied up to `--write-plan-ttl` after it was previewed,
apply re-validates the plan against the live database and asserts — with OVSDB
`wait` operations — that each target row still holds the values captured at
preview time. If a targeted row changed or was deleted in the meantime, apply
aborts with a `409 Conflict` (`plan preconditions no longer hold`) rather than
silently applying against a row that has moved on.

## Rollback restores changed fields only

Rollback compares a stored snapshot to the live NB state and produces a preview
plan that restores the fields that changed — on rows that **still exist**. It is
deliberately narrow:

- Rows **deleted** since the snapshot are **not** recreated. Recreating a row
  gives it a new server-assigned UUID, which would dangle every reference to the
  old UUID (e.g. `Logical_Switch.ports`). Such rows are listed in the plan's
  `warnings` instead.
- Rows **created** after the snapshot are left untouched.

Rollback is therefore a "restore what changed" operation, not a full
point-in-time revert.

## Audit log

Every applied mutation is recorded with a timestamp and the before/after state,
queryable at `/api/v1/write/audit`. Combined with the history store's automatic
snapshots, this gives you both "what was changed, by which operation" and "what
the whole deployment looked like around then." A failure to persist an audit
entry is logged (the mutation itself has already been applied), so a lost audit
trail is at least diagnosable rather than silent.

## Rate limiting

Applied writes are bounded by `--write-rate-limit` (default `30` per minute). This
is a blast-radius limit: it caps how fast an automated client — or a runaway loop —
can change OVN, buying time to notice and stop a bad actor before it has rewritten
the deployment. Preview, dry-run, apply, and rollback are all subject to the
limit; a rejected request returns `429 Too Many Requests`.

## Operational actions

On top of raw changes, the write engine exposes higher-level operations —
gateway failover, chassis evacuation, rollback and restore — so common procedures
are first-class and benefit from the same audit and safeguards instead of being
hand-assembled from individual writes.

## Authentication is still external

Write safety is about *containing* and *recording* mutations, not about deciding
*who* may make them. As with everything else, Northwatch has no built-in auth — an
open API with `--write-enabled` means open OVN mutation. Put it behind an
authenticating reverse proxy. See [The capability
model](/explanation/capability-model).

## Related

- [Enable write operations](/how-to/enable-write-operations)
- [History & snapshots](/explanation/history-and-snapshots)
