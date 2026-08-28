# The capability model

Northwatch describes what it can do as a set of capabilities rather than as
exclusive operating modes. This page explains that choice and its security
implications.

## Additive, not modal

A "mode" forces a single choice: you are in read mode or write mode or debug
mode. Capabilities instead stack: a live server always has `read`, `debug`,
`correlate`, `realtime`, `topology`, `flows`, `telemetry`, `alerts`, `history`
and `openapi`, and it gains `enrich`, `write`, `multi-cluster`, `ovs` or
`snapshot` as those features are configured. There is no state where browsing is
unavailable because you switched to tracing.

The server advertises the active set at `/api/v1/capabilities`, and the UI uses it
to decide which views and actions to render. The frontend therefore cannot offer
what the server does not have: a build pointed at a read-only server never shows
write controls, because the capability is absent. See
[Capabilities](/reference/capabilities) for the full list.

## Read and debug are the baseline

`read` and `debug` are always on. Debugging covers tracing, connectivity checks,
port diagnostics, ACL audits and stale-entry detection, and all of it is
read-only: it inspects cached state and never mutates OVN. There is no reason to
gate it, so it ships enabled.

## Write is opt-in and gated

`write` is the one capability that changes OVN, so it is off until you pass
`--write-enabled`. Even then it does not become raw CRUD. It is a plan-based
workflow with previews, an audit log and a rate limit, a design covered in
[Write safety](/explanation/write-safety).

## Capabilities are not authorization

Capabilities describe what the server can do, not what a particular caller is
allowed to do. Northwatch has no user accounts or roles; there is exactly one
authorization boundary, and it sits between reading and mutating:

- Read endpoints are open. Anyone who can reach the API can browse the full
  advertised capability set.
- Mutating endpoints require a bearer token, configured with `--api-tokens`
  or `--api-tokens-file`. The gate fails closed: with no tokens configured,
  every mutating request is rejected. Token names identify actors in the
  write-audit log; they do not carry per-token permissions.

The server also refuses to bind a non-loopback address without tokens (or an
explicit `--insecure-no-auth`), and can terminate TLS itself with
`--tls-cert` / `--tls-key`. See [CLI flags](/reference/cli#authentication) and
[Write safety](/explanation/write-safety).

The practical rules:

- Never expose Northwatch directly to untrusted networks. The read surface is
  unauthenticated by design.
- When readers are not all trusted, put it behind a reverse proxy that
  authenticates them; see [Deploy to production](/how-to/deploy-production).
- Use `--ws-allowed-origins` to constrain which web origins may open the
  WebSocket stream when a browser is involved.

## Offline mode is a capability too

Serving a snapshot adds the `snapshot` capability and sets `mode: snapshot`. The
live-tracking capabilities still appear, but their subsystems are inactive for a
frozen copy, because there is nothing to track. This lets the same UI render an
offline deployment without a separate build.

## Related

- [Capabilities](/reference/capabilities)
- [Write safety](/explanation/write-safety)
