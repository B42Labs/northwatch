.PHONY: build test lint generate schema-download clean vet unquarantine build-ui dev-ui build-all ensure-ui-dist openapi-export \
	ovnsim lab-images lab-up lab-down lab lab-seed lab-reseed lab-sim lab-bind lab-unbind lab-clean lab-nbctl lab-sbctl lab-install-tools lab-multi-up lab-multi-down \
	lab-compose-up lab-compose-build lab-compose-down lab-compose testbed

OVN_VERSION := v24.09.0
OVN_SCHEMA_BASE := https://raw.githubusercontent.com/ovn-org/ovn/$(OVN_VERSION)

# --- Local OVN lab (containerlab) -------------------------------------------
# A throwaway OVN deployment for developing and demoing Northwatch. See lab/.
# Requires a Linux Docker host (on macOS run via Colima/OrbStack/Lima).
LAB_NAME    ?= nw-lab
LAB_TOPO    ?= lab/topology.clab.yml
LAB_MULTI   ?= lab/topology-multi.clab.yml
LAB_COMPOSE ?= lab/docker-compose.yml
NB          ?= tcp:127.0.0.1:6641
SB          ?= tcp:127.0.0.1:6642
CENTRAL     := clab-$(LAB_NAME)-central

# Compose file set. `make lab-compose-* KERNEL=1` layers in the kernel-datapath
# override (needs a Linux host with the openvswitch module — see
# lab/docker-compose.kernel.yml).
LAB_COMPOSE_FILES := -f $(LAB_COMPOSE)
ifeq ($(KERNEL),1)
LAB_COMPOSE_FILES += -f lab/docker-compose.kernel.yml
endif

build: ensure-ui-dist
	go build -o bin/northwatch ./cmd/northwatch/

build-ui:
	cd ui/frontend && npm ci && npm run build

dev-ui:
	cd ui/frontend && npm run dev

build-all: build-ui build

ensure-ui-dist:
	@mkdir -p ui/frontend/dist
	@test -f ui/frontend/dist/index.html || echo '<!doctype html><html><body>Run make build-ui</body></html>' > ui/frontend/dist/index.html

test:
	go test -race ./...

lint:
	golangci-lint run

vet:
	go vet ./...

generate:
	go generate ./internal/ovsdb/...

schema-download:
	curl -sL $(OVN_SCHEMA_BASE)/ovn-nb.ovsschema -o internal/ovsdb/nb/ovn-nb.ovsschema
	curl -sL $(OVN_SCHEMA_BASE)/ovn-sb.ovsschema -o internal/ovsdb/sb/ovn-sb.ovsschema

unquarantine:
	xattr -d com.apple.quarantine bin/northwatch*

openapi-export:
	go run ./cmd/openapi-export > openapi.json

ovnsim:
	go build -o bin/ovnsim ./cmd/ovnsim/

# Build the central (ovn-northd + NB/SB) and chassis (OVS + ovn-controller) images.
lab-images:
	docker build -f lab/Dockerfile.central -t northwatch/lab-central:latest lab
	docker build -f lab/Dockerfile.chassis -t northwatch/lab-chassis:latest lab

# Bring up the single-cluster lab (1 central + 3 chassis) and publish NB/SB on the host.
lab-up: lab-images
	containerlab deploy -t $(LAB_TOPO)

# Tear the lab down.
lab-down:
	containerlab destroy -t $(LAB_TOPO)

# One-shot: bring the lab up and seed a baseline topology, then print next steps.
lab: lab-up lab-seed
	@echo ""
	@echo "Lab is up and seeded. Start Northwatch against it with:"
	@echo "  make build && ./bin/northwatch --ovn-nb-addr $(NB) --ovn-sb-addr $(SB)"
	@echo "Then open http://localhost:8080 and run 'make lab-sim' for continuous change."

# Create the baseline OVN objects (the dashboard "Grundlast").
lab-seed:
	go run ./cmd/ovnsim seed --nb $(NB)

# Force a clean baseline: remove all simulator objects, re-seed, bind VIFs.
# Use this when re-seeding does not seem to apply new topology (seed is
# idempotent by name, so it skips objects that already exist).
lab-reseed: lab-clean lab-seed lab-bind

# Continuously mutate the topology; --bind-ports also binds ports onto chassis.
# Runs in the foreground — Ctrl-C to stop.
lab-sim:
	go run ./cmd/ovnsim run --nb $(NB) --bind-ports --lab-name $(LAB_NAME)

# Bind every seeded VIF onto a chassis (creates real OVS interfaces), so ports
# bind in SB and the "VIF not bound to any chassis" health alerts clear.
lab-bind:
	go run ./cmd/ovnsim bind --nb $(NB) --lab-name $(LAB_NAME)

# Reverse of lab-bind: remove the chassis binding from every seeded VIF.
lab-unbind:
	go run ./cmd/ovnsim unbind --nb $(NB) --lab-name $(LAB_NAME)

# Remove everything ovnsim created (leaves the lab containers running).
lab-clean:
	go run ./cmd/ovnsim clean --nb $(NB)

# Convenience wrappers: `make lab-nbctl ARGS=show`, `make lab-sbctl ARGS="list Chassis"`.
lab-nbctl:
	docker exec $(CENTRAL) ovn-nbctl $(ARGS)

lab-sbctl:
	docker exec $(CENTRAL) ovn-sbctl $(ARGS)

# Optional second, independent cluster (NB 6643 / SB 6644) for the multi-cluster view.
lab-multi-up: lab-images
	containerlab deploy -t $(LAB_MULTI)

lab-multi-down:
	containerlab destroy -t $(LAB_MULTI)

# Docker Compose variant — no containerlab needed (works on macOS / Docker Desktop).
# Reuses existing images (building only missing ones); does NOT force a rebuild,
# so it works without Docker Hub access once the images exist. Only the chassis/
# central images contain OVN/OVS + entrypoints — ovnsim/seed changes are
# host-side and need no rebuild. Rebuild explicitly after changing a Dockerfile
# or entrypoint with `make lab-compose-build` (needs registry access).
lab-compose-up:
	docker compose $(LAB_COMPOSE_FILES) up -d

lab-compose-build:
	docker compose $(LAB_COMPOSE_FILES) build

lab-compose-down:
	docker compose $(LAB_COMPOSE_FILES) down

# One-shot: Compose lab up, wait for the chassis to register, then seed.
lab-compose: lab-compose-up
	@echo "Waiting for OVN to come up..."
	@until docker exec $(CENTRAL) ovn-nbctl --timeout=2 show >/dev/null 2>&1; do sleep 1; done
	$(MAKE) lab-seed
	@echo ""
	@echo "Lab is up and seeded. Start Northwatch against it with:"
	@echo "  make build && ./bin/northwatch --ovn-nb-addr $(NB) --ovn-sb-addr $(SB)"
	@echo "Then open http://localhost:8080 and run 'make lab-sim' for continuous change."

# Install containerlab. On macOS, run the lab inside a Linux VM (Colima/OrbStack/Lima);
# containerlab itself needs a Linux Docker host.
lab-install-tools:
	@echo "Installing containerlab (Linux)..."
	@echo "  bash -c \"$$(curl -sL https://get.containerlab.dev)\""
	@echo "On macOS: run the lab in a Linux VM (e.g. 'colima start' / OrbStack) — containerlab needs Linux."
	bash -c "$$(curl -sL https://get.containerlab.dev)"

# --- OSISM testbed ----------------------------------------------------------
# Run Northwatch against the OSISM testbed control plane with OpenStack name
# resolution. NB/SB are read from the three control-plane nodes (failover);
# OpenStack credentials mirror clouds.yaml and the Keystone API is verified
# against contrib/testbed.pem (the clouds.yaml `cacert`). Override any value on
# the command line, e.g. `make testbed TESTBED_CP1=10.0.0.1 OS_PASSWORD=secret`,
# or source an openrc beforehand (the OS_* defaults below honour the
# environment via ?=).
TESTBED_CP1 ?= 192.168.16.10
TESTBED_CP2 ?= 192.168.16.11
TESTBED_CP3 ?= 192.168.16.12
TESTBED_NB  ?= tcp:$(TESTBED_CP1):6641,tcp:$(TESTBED_CP2):6641,tcp:$(TESTBED_CP3):6641
TESTBED_SB  ?= tcp:$(TESTBED_CP1):6642,tcp:$(TESTBED_CP2):6642,tcp:$(TESTBED_CP3):6642

# OpenStack credentials — values from clouds.yaml (cloud "admin").
OS_AUTH_URL         ?= https://api.testbed.osism.xyz:5000/v3
OS_USERNAME         ?= admin
OS_PASSWORD         ?= password
OS_PROJECT_NAME     ?= admin
OS_USER_DOMAIN_NAME ?= default
OS_REGION_NAME      ?= RegionOne
OS_CACERT           ?= $(CURDIR)/contrib/testbed.pem

# Write operations are enabled by default in the testbed; override with
# `make testbed NORTHWATCH_WRITE_ENABLED=false`.
NORTHWATCH_WRITE_ENABLED ?= true

# HTTP listen address for the dashboard/API; override e.g.
# `make testbed NORTHWATCH_LISTEN=:9090`.
NORTHWATCH_LISTEN ?= :8080

testbed: build
	@test -f "$(OS_CACERT)" || { echo "error: CA cert $(OS_CACERT) not found (clouds.yaml 'cacert')"; exit 1; }
	@echo "Starting Northwatch against the OSISM testbed control plane:"
	@echo "  NB: $(TESTBED_NB)"
	@echo "  SB: $(TESTBED_SB)"
	@echo "  OpenStack: $(OS_AUTH_URL) (cacert $(OS_CACERT))"
	@echo "  Write operations: $(NORTHWATCH_WRITE_ENABLED)"
	@echo "  Dashboard: http://localhost$(NORTHWATCH_LISTEN)"
	OS_AUTH_URL="$(OS_AUTH_URL)" \
	OS_USERNAME="$(OS_USERNAME)" \
	OS_PASSWORD="$(OS_PASSWORD)" \
	OS_PROJECT_NAME="$(OS_PROJECT_NAME)" \
	OS_USER_DOMAIN_NAME="$(OS_USER_DOMAIN_NAME)" \
	OS_REGION_NAME="$(OS_REGION_NAME)" \
	OS_CACERT="$(OS_CACERT)" \
	NORTHWATCH_WRITE_ENABLED="$(NORTHWATCH_WRITE_ENABLED)" \
	NORTHWATCH_LISTEN="$(NORTHWATCH_LISTEN)" \
	./bin/northwatch --ovn-nb-addr "$(TESTBED_NB)" --ovn-sb-addr "$(TESTBED_SB)"

clean:
	rm -rf bin/ ui/frontend/dist/ ui/frontend/node_modules/
