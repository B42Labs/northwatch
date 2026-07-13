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

The server prints `Write operations enabled` and advertises the `write`
capability. Two flags tune the workflow:

| Flag | Default | Purpose |
|---|---|---|
| `--write-enabled` | `false` | Enable writes against OVN NB |
| `--write-plan-ttl` | `10m` | How long a prepared plan stays valid |
| `--write-rate-limit` | `30` | Max write operations per minute (0 = unlimited) |

## Authenticate the calls

Every `/api/v1/write/*` call needs a bearer token from `--api-tokens`. Without
one, the request is rejected with `401` before the engine sees it — so with
`--write-enabled` but no tokens, the write API is enabled and unusable.

```bash
export NW_TOKEN=<token>
curl -s -X POST http://localhost:8080/api/v1/write/preview \
  -H "Authorization: Bearer $NW_TOKEN" --data @change.json
```

The token's **name** is recorded as the `actor` on every audit entry, so give
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

1. **Preview / dry-run** — submit the intended change and get back the predicted
   effect (a `terraform plan`-style preview) without applying it.

   ```bash
   AUTH="Authorization: Bearer $NW_TOKEN"
   curl -s -X POST http://localhost:8080/api/v1/write/preview -H "$AUTH" --data @change.json
   curl -s -X POST http://localhost:8080/api/v1/write/dry-run -H "$AUTH" --data @change.json
   ```

2. **Apply a plan** — apply the prepared plan by id (valid until `--write-plan-ttl`
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

The request/response bodies are documented in the live OpenAPI spec — open
<http://localhost:8080/api/v1/docs>.

## Operational actions

With writes enabled, Northwatch also exposes higher-level operations:

```bash
curl -s -X POST http://localhost:8080/api/v1/write/failover -H "$AUTH"  # gateway failover
curl -s -X POST http://localhost:8080/api/v1/write/evacuate -H "$AUTH"  # evacuate a chassis
curl -s -X POST http://localhost:8080/api/v1/write/rollback -H "$AUTH"  # roll back a change
curl -s -X POST http://localhost:8080/api/v1/write/restore  -H "$AUTH"  # restore prior state
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
| `401`  | No valid API token presented (see [Authenticate the calls](#authenticate-the-calls)) |
| `404`  | Plan or audit entry not found or expired |
| `409`  | Plan preconditions no longer hold — a target row changed since preview |
| `413`  | Request body over the 1 MiB limit |
| `429`  | Rate limit exceeded (`--write-rate-limit`); applies to dry-run too |
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
