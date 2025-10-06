BINARY := hostapp
PKG := ./...

.PHONY: all build run test lint tidy clean dev-run talos-fresh talos-upgrade

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) ./cmd/hostapp

run: build
	./bin/$(BINARY) serve

# Dev helper: builds, generates certs, sets CORS origin, runs with DEV_NO_TSNET=1
# Usage:
#   make dev-run                    # default origin https://localhost:5173
#   make dev-run ORIGIN=https://app.example.com NO_CERTS=1
dev-run:
	@set -e; \
	ARGS=""; \
	if [ "$(NO_CERTS)" = "1" ]; then ARGS="$$ARGS --no-certs"; fi; \
	if [ -n "$(ORIGIN)" ]; then ARGS="$$ARGS --origin $(ORIGIN)"; fi; \
	sh ./scripts/dev-host-run.sh $$ARGS

# Talos helpers: pass arguments via FRESH_ARGS / UPGRADE_ARGS
# Examples:
#   make talos-fresh FRESH_ARGS="--cluster myc --endpoint https://10.0.0.10:6443 --cp 10.0.0.10 --workers 10.0.0.20"
#   make talos-upgrade UPGRADE_ARGS="--image ghcr.io/siderolabs/installer:v1.7.0 --nodes 10.0.0.10,10.0.0.20 --k8s v1.30.2"
talos-fresh:
	sh ./scripts/talos-fresh-deploy.sh $(FRESH_ARGS)

talos-upgrade:
	sh ./scripts/talos-upgrade-inplace.sh $(UPGRADE_ARGS)

lint:
	golangci-lint run || true

test:
	go test -race $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf bin
