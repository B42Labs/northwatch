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

## The apply token binds a plan to a snapshot

Preview returns an **apply token** alongside the plan, and apply requires it. The
token is an HMAC-SHA256 over the plan id and the id of the snapshot taken at
preview time, keyed by a secret generated fresh each time the server starts. Apply
recomputes the expected token and rejects a mismatch, so a plan cannot be applied
with a token that was not issued for that exact `(plan, snapshot)` pair — and no
token survives a server restart.

It is important to be clear about what this token is *not*: it is **not
authentication**. It binds a plan to the snapshot it was previewed against; it
does not identify or authorize the caller. Anyone who can read the preview
response holds the token. Deciding *who* may apply is still the reverse proxy's
job (see below).

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

## Authentication gates the write API

Write safety is about *containing* and *recording* mutations. Deciding *who* may
make them is the job of the bearer tokens configured with `--api-tokens`: every
**mutating** write call — preview, dry-run, apply, cancel, the operational
actions — must present one, or it is rejected with 401 before the engine sees
it. The `GET` write routes (schema, plan inspection, the audit log) are reads
and stay open like the rest of the read surface.

The `apply_token` returned by `POST /api/v1/write/preview` is **not**
authentication. It proves the caller previewed the exact plan it is about to
apply — a guard against applying a stale or different plan — and it is handed out
to whoever asked for the preview. The bearer token is what decides whether they
were allowed to ask.

The audit `actor` is derived from the bearer token's name, not from the request
body, so a caller cannot forge who made a change. (A deployment running with
`--insecure-no-auth` has no credential to attribute a change to, and records
`anonymous` instead.) See [The capability
model](/explanation/capability-model).

## Related

- [Enable write operations](/how-to/enable-write-operations)
- [History & snapshots](/explanation/history-and-snapshots)
