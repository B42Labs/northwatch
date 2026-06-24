# The capability model

Northwatch describes what it can do as a set of **capabilities** rather than as
exclusive operating modes. This page explains that choice and its security
implications.

## Additive, not modal

A "mode" forces a single choice: you are in *read mode* or *write mode* or *debug
mode*. Capabilities instead stack: a live server always has `read`, `debug`,
`correlate`, `realtime`, `topology`, `flows`, `telemetry`, `alerts`, `history`
and `openapi`, and it gains `enrich`, `write`, `multi-cluster` or `snapshot` as
those features are configured. There is no state where browsing is unavailable
because you switched to tracing.

The server advertises the active set at `/api/v1/capabilities`, and the UI uses it
to decide which views and actions to render. This keeps the frontend honest: a
build pointed at a read-only server simply never shows write controls, because the
capability is absent. See [Capabilities](/reference/capabilities) for the full
list.

## Read and debug are the baseline

`read` and `debug` are always on. Debugging — tracing, connectivity checks, port
diagnostics, ACL audits, stale-entry detection — is read-only: it inspects cached
state and never mutates OVN. There is no reason to gate it, so it ships enabled.

## Write is opt-in and gated

`write` is the one capability that changes OVN, so it is off until you pass
`--write-enabled`. Even then it does not become raw CRUD — it is a plan-based
workflow with previews, an audit log and a rate limit. That design is covered in
[Write safety](/explanation/write-safety).

## There is no authentication

This is the most important thing to understand: **Northwatch has no user
management, login, or authorization.** Capabilities describe what the *server* can
do, not what a particular *caller* is allowed to do. Anyone who can reach the API
gets the full advertised capability set.

That is a deliberate scoping decision, not an oversight. Authentication,
authorization and TLS termination belong at the network and reverse-proxy layer,
where an operator already runs them for everything else. The practical rules:

- Never expose Northwatch directly to untrusted networks.
- Put it behind a reverse proxy that authenticates users — especially with
  `--write-enabled`, where an open API means open OVN mutation.
- Use `--ws-allowed-origins` to constrain which web origins may open the
  WebSocket stream when a browser is involved.

## Offline mode is a capability too

Serving a snapshot adds the `snapshot` capability and sets `mode: snapshot`. The
live-tracking capabilities still appear, but their subsystems are inactive for a
frozen copy — there is nothing to track. This lets the same UI render an offline
deployment without a separate build.

## Related

- [Capabilities](/reference/capabilities)
- [Write safety](/explanation/write-safety)
