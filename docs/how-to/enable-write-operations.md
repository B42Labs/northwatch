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
  --write-enabled \
  --api-tokens "ops=$(openssl rand -hex 32)"
```

The server logs `write operations enabled` and advertises the `write`
capability. Two flags tune the workflow:

| Flag | Default | Purpose |
|---|---|---|
| `--write-enabled` | `false` | Enable writes against OVN NB |
| `--write-plan-ttl` | `10m` | How long a prepared plan stays valid |
| `--write-rate-limit` | `30` | Max write operations per minute (0 = unlimited) |

## Authenticate the calls

Every mutating write call, meaning preview, dry-run, apply, cancel and the
operational actions, needs a bearer token from `--api-tokens`. Without one, the
request is rejected with `401` before the engine sees it, so with
`--write-enabled` but no tokens, the write API is enabled and unusable. The
`GET` write routes (schema, plan inspection, the audit log) are part of the open
read surface and need no token.

```bash
export NW_TOKEN=<token>
curl -s -X POST http://localhost:8080/api/v1/write/preview \
  -H "Authorization: Bearer $NW_TOKEN" --data @change.json
```

The token's name is recorded as the `actor` on every audit entry, so give
each operator or automation its own token. A client-supplied `actor` field in the
request body is ignored.

::: warning The read surface is still open
Tokens gate mutation, not reading. Anyone who can reach the API can still read
your entire OVN deployment. Keep Northwatch behind a reverse proxy that
authenticates users and restrict network access. See
[The capability model](/explanation/capability-model).
:::

## The workflow

Writes are a two-phase, plan-based flow rather than direct CRUD:

1. Preview / dry-run. Submit the intended change and get back the predicted
   effect (a `terraform plan`-style preview) without applying it. The body is
   `{"operations": [...], "reason": "..."}` with at least one operation:

   ```bash
   AUTH="Authorization: Bearer $NW_TOKEN"
   curl -s -X POST http://localhost:8080/api/v1/write/preview -H "$AUTH" --data @change.json
   curl -s -X POST http://localhost:8080/api/v1/write/dry-run -H "$AUTH" --data @change.json
   ```

2. Apply a plan. Apply the prepared plan by id (valid until `--write-plan-ttl`
   expires):

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/write/plans/<id>/apply \
     -H "$AUTH" --data '{"apply_token": "<token from preview>"}'
   ```

   Inspect or cancel a pending plan:

   ```bash
   curl -s http://localhost:8080/api/v1/write/plans/<id>
   curl -s -X DELETE http://localhost:8080/api/v1/write/plans/<id> -H "$AUTH"
   ```

The request/response bodies are documented in the live OpenAPI spec at
<http://localhost:8080/api/v1/docs>.

## Operational actions

With writes enabled, Northwatch also exposes higher-level operations. Each takes
a small JSON body naming its target:

```bash
# Move a gateway HA group to a specific chassis
curl -s -X POST http://localhost:8080/api/v1/write/failover -H "$AUTH" \
  --data '{"group_name": "gw-group-1", "target_chassis": "chassis-2"}'

# Drain all gateway priorities off a chassis (e.g. before maintenance)
curl -s -X POST http://localhost:8080/api/v1/write/evacuate -H "$AUTH" \
  --data '{"chassis_name": "chassis-1"}'

# Return an evacuated chassis to service — the inverse of evacuate
curl -s -X POST http://localhost:8080/api/v1/write/restore -H "$AUTH" \
  --data '{"chassis_name": "chassis-1"}'

# Roll changed fields back to a history snapshot
curl -s -X POST http://localhost:8080/api/v1/write/rollback -H "$AUTH" \
  --data '{"snapshot_id": 42, "reason": "revert bad ACL change"}'
```

A missing required field returns `400` naming the field. `rollback` restores
changed fields on rows that still exist; rows deleted since the snapshot are
reported in the plan's `warnings` (they are not recreated). See
[Write safety](/explanation/write-safety#rollback-restores-changed-fields-only).

## Status codes

The write endpoints use precise status codes so a client can react correctly:

| Status | Meaning |
| ------ | ------- |
| `200`  | Success (preview/dry-run/apply/rollback returned a result) |
| `400`  | Invalid request: malformed body, unknown field, missing/nonexistent reference, or applying a plan that is unknown or expired |
| `401`  | No valid API token presented (see [Authenticate the calls](#authenticate-the-calls)) |
| `404`  | Plan (on inspect/cancel) or audit entry not found or expired |
| `409`  | Plan preconditions no longer hold; a target row changed since preview |
| `413`  | Request body over the 1 MiB limit |
| `429`  | Rate limit exceeded (`--write-rate-limit`); applies to dry-run too. Also returned (with `Retry-After`) by the auth throttle after 5 failed token attempts from one IP within a minute |
| `500`  | Server-side/infrastructure failure. The body is always the generic `{"error": "internal server error"}`; the cause is in the server log. |

## Audit log

Every applied mutation is recorded with timestamp and before/after state:

```bash
curl -s "http://localhost:8080/api/v1/write/audit?limit=100"
curl -s http://localhost:8080/api/v1/write/audit/<id>
```

The writable schema (what can be changed) is available at:

```bash
curl -s http://localhost:8080/api/v1/write/schema
```
