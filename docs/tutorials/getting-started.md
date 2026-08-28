# Getting started

This tutorial takes you from a fresh checkout to a running Northwatch looking at
a live OVN deployment. You will bring up the bundled OVN lab, point Northwatch at
it, and open the UI on a topology that has objects, events and alerts flowing
through it. Follow the steps in order.

By the end you will have a real OVN control plane running locally and Northwatch
visualizing it.

## Prerequisites

Before you start, make sure you have:

1. Go 1.26+ installed (`go version`), the version the module declares in
   `go.mod`.
2. Docker running. On macOS, Docker Desktop is enough for the Compose
   lab used below.
3. Node.js 22+ if you want to build the embedded web UI (`make build-ui`).
   The binary builds and runs without it; it just serves a placeholder page
   until the UI is built.
4. The repository checked out, with your shell in its root.

The lab brings up a real `ovn-northd` plus Northbound/Southbound databases and
three chassis running Open vSwitch and `ovn-controller`. It uses the OVS
userspace datapath, so it needs no kernel modules and no real packet
forwarding. It is a control-plane lab for observability.

## Step 1: Bring up the OVN lab

The Compose lab works on plain Docker, including Docker Desktop on macOS:

```bash
make lab-compose-up    # build images + start 1 central + 3 chassis (NB :6641 / SB :6642)
```

The first run builds the `central` and `chassis` images, which takes a few
minutes. Subsequent runs reuse them.

::: tip Linux with containerlab
On a Linux Docker host you can use the containerlab variant instead:
`make lab-up`. The Compose variant above is the simplest path on macOS. See
[Run the local lab](/how-to/run-the-local-lab) for the full set of options.
:::

## Step 2: Seed a baseline topology

An empty OVN database is not interesting to look at. The bundled `ovnsim` load
generator creates a realistic baseline across every major Northbound table:
tenant switches with VIF ports and DHCP, routers with NAT and static routes,
security port groups with ACLs and address sets, load balancers, and more:

```bash
make lab-seed          # create the baseline OVN objects
make lab-bind          # bind the seeded ports onto chassis (clears "unbound VIF" alerts)
```

::: info Why `lab-bind`?
`seed` only creates the logical ports in the Northbound database. Until a
backing OVS interface exists on a chassis, `ovn-controller` never binds them, so
Northwatch correctly flags them as "VIF not bound to any chassis". `make lab-bind`
creates those interfaces so the bindings appear. This is Northwatch already doing
its job. See [Diagnose port bindings](/how-to/diagnose-port-bindings).
:::

## Step 3: Build and start Northwatch

Build the binary (this also embeds the web UI if you have built it):

```bash
make build-ui          # optional: build the Svelte UI assets
make build             # -> bin/northwatch
```

Point it at the lab's Northbound and Southbound databases:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:127.0.0.1:6641 \
  --ovn-sb-addr tcp:127.0.0.1:6642
```

On startup Northwatch connects to both databases, subscribes to every table,
builds its in-memory cache, and logs the listen address:

```
level=INFO msg="connecting to OVN databases" cluster=default
level=INFO msg="connected to OVN databases" cluster=default
level=INFO msg="northwatch listening" addr=127.0.0.1:8080 scheme=http
```

## Step 4: Open the UI

Open <http://localhost:8080> in your browser. You are looking at the live state
of the lab: the logical and physical topology, the chassis, the port bindings,
and the real-time event stream. The API is served alongside it under the
`/api/v1` prefix, with an interactive reference at
<http://localhost:8080/api/v1/docs>.

Confirm the API responds and reports its capabilities:

```bash
curl -s http://localhost:8080/api/v1/capabilities
```

```json
{
  "capabilities": ["read", "debug", "correlate", "realtime", "topology",
                   "flows", "telemetry", "alerts", "history", "openapi"],
  "mode": "live"
}
```

## Step 5: Make something happen

Northwatch is real-time. In a second terminal, start the simulator to
continuously mutate the topology, creating and deleting switches, routers and
ports, flipping NAT and ACL rules, and simulating gateway failover:

```bash
make lab-sim           # continuous change; Ctrl-C to stop
```

Watch the UI update live as objects come and go. The event log, history timeline
and alerts all populate from this activity.

## Next steps

- Follow a real debugging workflow:
  [Investigate with Omnisearch](/tutorials/investigate-with-omnisearch).
- Capture this deployment and browse it offline:
  [Explore a deployment offline](/tutorials/explore-a-deployment-offline).
- Point Northwatch at your own deployment:
  [Connect to a Raft cluster](/how-to/connect-to-a-raft-cluster).
- Tear the lab down when you are done: `make lab-compose-down`.
