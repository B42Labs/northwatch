# Investigate with Omnisearch

This tutorial walks you through a complete investigation: starting from a single
IP address, you will follow the correlation chain from Northbound intent to
Southbound realization and find the chassis that hosts the port. Along the way
you will learn how Northwatch links the two OVN databases together.

It assumes you already have Northwatch running against the lab from
[Getting started](/tutorials/getting-started). Every command uses the REST API
so you can see exactly what the UI is doing; you can follow the same path by
clicking through the web UI.

## Step 1: Pick something to search for

OVN debugging usually starts with one fact: an IP, a MAC, a UUID, or a name a
user reported. List a few logical switch ports from the lab to grab a real one:

```bash
curl -s http://localhost:8080/api/v1/nb/logical-switch-ports | head
```

Note an IP address or port name from the output, anything that identifies a
workload.

## Step 2: Search across everything at once

Hand that value to Omnisearch. It detects the input type (IPv4/IPv6, MAC, UUID,
or free text) and queries the Northbound tables, the Southbound tables, and any
enrichment caches in parallel:

```bash
curl -s 'http://localhost:8080/api/v1/search?q=10.0.0.42'
```

The results are grouped by entity type: Logical Switch Port, Port Binding,
Chassis, and so on. One query has already crossed both databases. In the UI this
is the single search field at the top; the grouped results are clickable.

## Step 3: Follow the chain into the Northbound database

Take the Logical Switch Port UUID from the search results and fetch the
correlated view. Unlike the raw table endpoint, the correlated endpoint
stitches the Northbound port to its Southbound counterpart and any enrichment
context:

```bash
curl -s http://localhost:8080/api/v1/correlated/logical-switch-ports/<uuid>
```

This is what Northwatch is built around: a Northbound `Logical_Switch_Port`
expresses intent ("this port should exist on this switch"), while the Southbound
`Port_Binding` records its realization ("this port is bound to this chassis").
The correlated view shows both sides together.

## Step 4: Land on the chassis

The correlated port tells you which chassis claims the binding. Pull the
correlated chassis to see the physical side: its hostname, encapsulation, and
the ports it currently hosts:

```bash
curl -s http://localhost:8080/api/v1/correlated/chassis
```

You have now traced one IP address all the way down the stack:

```
IP / name  →  Logical_Switch_Port (NB, intent)
           →  Port_Binding        (SB, realization)
           →  Chassis             (SB, physical host)
```

## Step 5: When the chain breaks

The interesting cases are the ones where the chain does not complete: a port
with no binding, or a binding with no chassis. That is exactly what the debug
tools surface. Ask Northwatch which ports are not bound:

```bash
curl -s http://localhost:8080/api/v1/debug/port-diagnostics
```

If you ran `make lab-unbind`, those ports show up here with no chassis. This is
the natural handoff from "find it" to "diagnose it".

## What you learned

- Omnisearch is the entry point: one value, every data source, grouped results.
- The correlated endpoints join Northbound intent to Southbound realization,
  which is the mental model the whole tool is built around. See
  [Correlation & search](/explanation/correlation-and-search).
- A broken correlation chain is a diagnosis, and the debug endpoints make it
  explicit.

## Next steps

- Reproduce a packet's journey through the pipeline:
  [Trace a packet path](/how-to/trace-a-packet-path).
- Dig into binding problems:
  [Diagnose port bindings](/how-to/diagnose-port-bindings).
- Read the recipe form of this workflow:
  [Search with Omnisearch](/how-to/search-with-omnisearch).
