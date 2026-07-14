# Trace a packet path

Northwatch can simulate a packet through the Southbound logical-flow pipeline —
an `ovn-trace`-style walk evaluated against its cached flow state — and render the
table-by-table match/action sequence. Traces can be saved and exported for
incident reports.

This needs the `Logical_Flow` table in the cache, so do **not** skip it with
`--monitor-skip-tables` if you want to trace.

## Run a trace

The trace endpoint lives under the debug routes. `port` — the Southbound
`Port_Binding` UUID to start from — is required; `dst_ip` and `protocol`
describe the simulated packet:

```bash
curl -s 'http://localhost:8080/api/v1/debug/trace?port=<uuid>&dst_ip=10.0.0.42&protocol=tcp'
```

An unknown `port` returns `404`. In the web UI, the tracer is a form: pick the
starting datapath/port, fill in the packet fields, and submit. The full
parameter list is in the interactive API reference at
<http://localhost:8080/api/v1/docs>.

## Store and review traces

A trace is retained **only** when you ask for it with `store=true` — the
response then carries an `id`:

```bash
curl -s 'http://localhost:8080/api/v1/debug/trace?port=<uuid>&store=true'
curl -s http://localhost:8080/api/v1/debug/traces        # list stored traces
```

Stored traces live in an in-memory store for one hour, capped at 200 entries;
without `store=true` nothing is listed by `/debug/traces` or exportable.

## Export a trace

Export a saved trace by id for sharing or attaching to an incident report:

```bash
curl -s http://localhost:8080/api/v1/export/trace/<id>
```

## Notes

- Tracing evaluates the cached Southbound flows; it does not send real packets.
- Tracing is part of the `debug` capability, which is on by default — see
  [Capabilities](/reference/capabilities).
- It also works in offline snapshot mode, since the flows are in the captured
  copy. See [Explore a deployment offline](/tutorials/explore-a-deployment-offline).
