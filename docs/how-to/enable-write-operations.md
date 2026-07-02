# Enable write operations

By default Northwatch is read-only. Write operations against the OVN Northbound
database are off until you explicitly enable them, and even then they go through a
plan/preview/apply workflow with an audit log and a rate limit. For the reasoning
behind that workflow, see [Write safety](/explanation/write-safety).

## Turn writes on

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --write-enabled
```

The server prints `Write operations enabled` and advertises the `write`
capability. Two flags tune the workflow:

| Flag | Default | Purpose |
|---|---|---|
| `--write-enabled` | `false` | Enable writes against OVN NB |
| `--write-plan-ttl` | `10m` | How long a prepared plan stays valid |
| `--write-rate-limit` | `30` | Max write operations per minute (0 = unlimited) |

::: warning No built-in authentication
Northwatch has no user management. With `--write-enabled` set, anyone who can
reach the API can mutate OVN. Put it behind a reverse proxy that handles
authentication and authorization, and restrict network access. See
[The capability model](/explanation/capability-model).
:::

## The workflow

Writes are a two-phase, plan-based flow rather than direct CRUD:

1. **Preview / dry-run** — submit the intended change and get back the predicted
   effect (a `terraform plan`-style preview) without applying it.

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/write/preview  --data @change.json
   curl -s -X POST http://localhost:8080/api/v1/write/dry-run  --data @change.json
   ```

2. **Apply a plan** — apply the prepared plan by id (valid until `--write-plan-ttl`
   expires):

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/write/plans/<id>/apply
   ```

   Inspect or cancel a pending plan:

   ```bash
   curl -s http://localhost:8080/api/v1/write/plans/<id>
   curl -s -X DELETE http://localhost:8080/api/v1/write/plans/<id>
   ```

The request/response bodies are documented in the live OpenAPI spec — open
<http://localhost:8080/api/v1/docs>.

## Operational actions

With writes enabled, Northwatch also exposes higher-level operations:

```bash
curl -s -X POST http://localhost:8080/api/v1/write/failover    # gateway failover
curl -s -X POST http://localhost:8080/api/v1/write/evacuate    # evacuate a chassis
curl -s -X POST http://localhost:8080/api/v1/write/rollback    # roll back a change
curl -s -X POST http://localhost:8080/api/v1/write/restore     # restore prior state
```

`rollback` restores changed fields on rows that still exist; rows deleted since
the snapshot are reported in the plan's `warnings` (they are not recreated). See
[Write safety](/explanation/write-safety#rollback-restores-changed-fields-only).

## Status codes

The write endpoints use precise status codes so a client can react correctly:

| Status | Meaning |
| ------ | ------- |
| `200`  | Success (preview/dry-run/apply/rollback returned a result) |
| `400`  | Invalid request — malformed body, unknown field, missing/nonexistent reference |
| `404`  | Plan or audit entry not found or expired |
| `409`  | Plan preconditions no longer hold — a target row changed since preview |
| `429`  | Rate limit exceeded (`--write-rate-limit`); applies to dry-run too |
| `500`  | Server-side/infrastructure failure |

## Audit log

Every applied mutation is recorded with timestamp and before/after state:

```bash
curl -s http://localhost:8080/api/v1/write/audit
curl -s http://localhost:8080/api/v1/write/audit/<id>
```

The writable schema (what can be changed) is available at:

```bash
curl -s http://localhost:8080/api/v1/write/schema
```
