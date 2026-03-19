.PHONY: build test lint generate schema-download clean vet unquarantine

OVN_VERSION := v24.09.0
OVN_SCHEMA_BASE := https://raw.githubusercontent.com/ovn-org/ovn/$(OVN_VERSION)

build:
	go build -o bin/northwatch ./cmd/northwatch/

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

clean:
	rm -rf bin/
