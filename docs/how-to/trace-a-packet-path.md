# Trace a packet path

Northwatch can simulate a packet through the Southbound logical-flow pipeline —
an `ovn-trace`-style walk evaluated against its cached flow state — and render the
table-by-table match/action sequence. Traces can be saved and exported for
incident reports.

This needs the `Logical_Flow` table in the cache, so do **not** skip it with
`--monitor-skip-tables` if you want to trace.

## Run a trace

The trace endpoint lives under the debug routes:

```bash
curl -s 'http://localhost:8080/api/v1/debug/trace?<parameters>'
```

The exact query parameters (the datapath/port to start from and the simulated
packet fields) are documented in the live OpenAPI spec — open the Swagger UI at
<http://localhost:8080/api/v1/docs> to see the parameter names and try the
endpoint interactively. In the web UI, the tracer is a form: pick the starting
datapath/port, fill in the packet fields, and submit.

## Review past traces

Northwatch keeps recent traces in an in-memory store (retained for one hour):

```bash
curl -s http://localhost:8080/api/v1/debug/traces        # list recent traces
```

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
