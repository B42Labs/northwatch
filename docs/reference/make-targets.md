# Make targets

The `Makefile` is the entry point for building, testing and running the local
lab. This page lists the targets; the lab targets are covered in detail in
[Run the local lab](/how-to/run-the-local-lab).

## Build & test

| Target | What it does |
|---|---|
| `make build` | Build the binary to `bin/northwatch` (ensures a UI dist exists first). |
| `make ensure-ui-dist` | Ensure a UI dist exists, writing a placeholder if the frontend has not been built. A prerequisite of `make build`. |
| `make build-ui` | Build the Svelte frontend (`npm ci && npm run build`). |
| `make build-all` | `build-ui` then `build`. |
| `make dev-ui` | Run the frontend dev server. |
| `make test` | Run all Go tests with the race detector (`go test -race ./...`). |
| `make lint` | Run `golangci-lint`. |
| `make vet` | Run `go vet ./...`. |
| `make docs-check` | Fail if a CLI flag or make target is undocumented in `docs/reference/`. |
| `make clean` | Remove `bin/` and frontend build artifacts. |

## Code generation

| Target | What it does |
|---|---|
| `make generate` | Regenerate the OVSDB models from the schemas. |
| `make schema-download` | Download the pinned OVN NB/SB schemas (`OVN_VERSION` in the Makefile). |
| `make openapi-export` | Write the OpenAPI spec to `openapi.json`. |
| `make ovnsim` | Build the `ovnsim` load generator to `bin/ovnsim`. |

## macOS note

`make unquarantine` strips the macOS quarantine attribute from `bin/northwatch*`
so a freshly built binary runs without a Gatekeeper prompt.

## Lab targets

| Target | What it does |
|---|---|
| `make lab-compose-up` / `lab-compose-down` | Start / stop the Docker Compose lab. `lab-compose-up` reuses existing images (building only missing ones — no forced rebuild), so it works without registry access once the images exist; add `KERNEL=1` to layer in the kernel-datapath override. |
| `make lab-compose-build` | Force-rebuild the Compose lab images (needs registry access). Run after changing a `Dockerfile` or entrypoint. |
| `make lab-compose` | Compose up, wait for OVN, then seed. |
| `make lab-up` / `lab-down` | Start / stop the containerlab lab (Linux). |
| `make lab` | `lab-up` + `lab-seed`. |
| `make lab-seed` | Create the baseline OVN topology. |
| `make lab-bind` / `lab-unbind` | Bind / unbind seeded VIFs onto chassis. |
| `make lab-reseed` | `lab-clean` + `lab-seed` + `lab-bind`. |
| `make lab-sim` | Continuously mutate the topology (foreground). |
| `make lab-clean` | Remove everything `ovnsim` created. |
| `make lab-nbctl ARGS=...` / `lab-sbctl ARGS=...` | Run `ovn-nbctl` / `ovn-sbctl` in the central container. |
| `make lab-multi-up` / `lab-multi-down` | Start / stop a second independent cluster. |
| `make lab-images` | Build the central and chassis images. |
| `make lab-install-tools` | Install containerlab (Linux). |

## OSISM testbed

`make testbed` builds Northwatch and runs it against the OSISM testbed control
plane, with OpenStack name resolution enabled and the Keystone API verified
against the private testbed CA (`contrib/testbed.pem`, the clouds.yaml
`cacert`). It connects to the three control-plane nodes with NB/SB failover and
enables write operations by default. The target fails fast if the CA bundle at
`OS_CACERT` is missing.

Every value is overridable on the command line — for example
`make testbed TESTBED_CP1=10.0.0.1 OS_PASSWORD=secret` — or by sourcing an
OpenStack RC file first (the `OS_*` defaults honour the environment):

| Variable | Default | Purpose |
|---|---|---|
| `TESTBED_CP1` / `TESTBED_CP2` / `TESTBED_CP3` | `192.168.16.10` / `.11` / `.12` | Control-plane node addresses. |
| `TESTBED_NB` | failover across `CP1/2/3:6641` | Northbound address. |
| `TESTBED_SB` | failover across `CP1/2/3:6642` | Southbound address. |
| `OS_AUTH_URL` … `OS_CACERT` | values from `clouds.yaml` | OpenStack credentials and the CA bundle. |
| `NORTHWATCH_WRITE_ENABLED` | `true` | Enable write operations. |
| `NORTHWATCH_LISTEN` | `:8080` | Dashboard / API listen address. |

## Useful variables

| Variable | Default | Purpose |
|---|---|---|
| `NB` | `tcp:127.0.0.1:6641` | Northbound address used by lab targets. |
| `SB` | `tcp:127.0.0.1:6642` | Southbound address used by lab targets. |
| `LAB_NAME` | `nw-lab` | Lab name (container name prefix). |
| `KERNEL` | *(unset)* | `KERNEL=1` layers in the kernel-datapath Compose override for real HA failover. |

```bash
# Re-seed a second lab cluster:
make lab-seed NB=tcp:127.0.0.1:6643

# Real HA failover on a Linux host:
make lab-compose-up KERNEL=1
```
