BINARY := hostapp
PKG := ./...

# Defaults (override as needed)
LISTEN_LOCAL ?= 127.0.0.1:8080

.PHONY: all help \
	build build-backend build-ui \
	run \
	test lint tidy clean setup ui-setup \
	health tls-check-backend regen-certs stop-all \
	talos-fresh talos-upgrade agent-build \
	crd-apply operator-run operator-build db-health



all: build ## Build backend and UI

# ---------- Help ----------
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make [target]\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---------- Setup ----------
setup: ui-setup regen-certs ## One-time setup: install UI deps and generate local TLS certs

ui-setup: ## Install UI dependencies (npm ci)
	cd ui && npm ci

regen-certs: ## Regenerate local server TLS certificate
	./scripts/generate-server-cert.sh -f

# ---------- Build ----------
build: build-backend build-ui ## Build backend and UI

build-backend: ## Build Go backend (bin/hostapp)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) ./cmd/hostapp

operator-build: ## Build operator manager binary (reuses hostapp for now if integrated later)
	@echo "(placeholder) operator shares hostapp binary in prototype"

build-ui: ## Build UI (Vite)
	cd ui && npm ci && npm run build

# ---------- Run ----------
run: build ## Build all (backend+UI) and run backend (serve)
	LISTEN_LOCAL=$(LISTEN_LOCAL) ./bin/$(BINARY) serve

# ---------- DB / Health ----------
db-health: ## Check database API and report availability
	@echo "Checking backend health and DB API..."; \
	(set -x; curl -sk https://127.0.0.1:8080/healthz); echo; \
	(set -x; curl -sk https://127.0.0.1:8080/api/db) || true; echo

# ---------- Quality ----------
test: ## Run Go tests (race)
	go test -race $(PKG)

lint: ## Run golangci-lint (non-fatal if not installed)
	golangci-lint run || true

tidy: ## go mod tidy
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin ui/dist

# ---------- Utilities ----------
health: ## Check backend health endpoint
	curl -k https://127.0.0.1:8080/healthz || true

tls-check-backend: ## Show TLS info for backend :8080
	echo | openssl s_client -connect 127.0.0.1:8080 -servername localhost -tls1_2 2>/dev/null | head -n 20

stop-all: ## Stop all managed workloads via admin API
	@curl -sk -X POST https://127.0.0.1:8080/api/admin/stop-all || curl -sk -X POST https://127.0.0.1:8080/api/stop-all || true

# ---------- CRD / Operator helpers ----------
CRD_DIR ?= config/crd
crd-apply: ## Apply (or update) GuildNet CRDs into current kube-context
	@[ -d $(CRD_DIR) ] || { echo "CRD dir $(CRD_DIR) missing"; exit 1; }
	@for f in $(CRD_DIR)/*.yaml; do \
		echo "kubectl apply -f $$f"; \
		kubectl apply -f $$f >/dev/null || exit 1; \
	done; echo "CRDs applied"

operator-run: ## Run workspace operator (controller-runtime manager) locally
	go run ./cmd/hostapp --mode operator 2>&1 | sed 's/^/[operator] /'

agent-build: ## Build agent image (see scripts)
	sh ./scripts/agent-build-load.sh

# ---------- Host subnet router (Option A) ----------
.PHONY: router-install router-up router-down router-status

router-install: ## Install native tailscale client (host subnet router)
	bash ./scripts/tailscale-router.sh install

router-up: ## Bring up host subnet router (advertise TS_ROUTES)
	bash ./scripts/tailscale-router.sh up

router-down: ## Bring down host subnet router
	bash ./scripts/tailscale-router.sh down

router-status: ## Show tailscale router status
	bash ./scripts/tailscale-router.sh status

# ---------- Local Headscale (LAN bind) ----------
.PHONY: headscale-up headscale-down headscale-status env-sync-lan headscale-bootstrap local-overlay-up

headscale-up: ## Start Headscale bound to LAN IP (auto-detected)
	bash ./scripts/headscale-run.sh up

headscale-down: ## Stop & remove Headscale container
	bash ./scripts/headscale-run.sh down

headscale-status: ## Show Headscale container status
	bash ./scripts/headscale-run.sh status

headscale-bootstrap: ## Create Headscale user+preauth key and sync TS_AUTHKEY in .env
	bash ./scripts/headscale-bootstrap.sh

env-sync-lan: ## Rewrite TS_LOGIN_SERVER in .env to use LAN IP if it is 127.0.0.1
	bash ./scripts/detect-lan-and-sync-env.sh

local-overlay-up: ## Bring up local Headscale on LAN + router; prepares a working local overlay
	$(MAKE) headscale-up
	$(MAKE) env-sync-lan
	$(MAKE) headscale-bootstrap
	$(MAKE) router-install
	$(MAKE) router-up

# ---------- Talos helpers ----------
# Examples:
#   make talos-fresh FRESH_ARGS="--cluster myc --endpoint https://10.0.0.10:6443 --cp 10.0.0.10 --workers 10.0.0.20"
#   make talos-upgrade UPGRADE_ARGS="--image ghcr.io/siderolabs/installer:v1.7.0 --nodes 10.0.0.10,10.0.0.20 --k8s v1.30.2"

talos-fresh: ## Talos cluster fresh deploy
	bash ./scripts/talos-fresh-deploy.sh $(FRESH_ARGS)

talos-upgrade: ## Talos cluster in-place upgrade
	bash ./scripts/talos-upgrade-inplace.sh $(UPGRADE_ARGS)

# ---------- tsnet subnet router ----------
.PHONY: tsnet-subnet-router run-subnet-router tsnet-router-up tsnet-router-down tsnet-router-status tsnet-router-logs

tsnet-subnet-router: ## Build tsnet subnet router binary
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/tsnet-subnet-router ./cmd/tsnet-subnet-router

run-subnet-router: tsnet-subnet-router ## Run tsnet subnet router with .env
	/bin/bash -lc 'set -a; [ -f ./.env ] && . ./.env; TS_HOSTNAME="${TS_HOSTNAME:-host-app}-router" ./bin/tsnet-subnet-router'

tsnet-router-up: tsnet-subnet-router ## Start tsnet subnet router in background
	bash ./scripts/tsnet-router.sh up

tsnet-router-down: ## Stop tsnet subnet router
	bash ./scripts/tsnet-router.sh down

tsnet-router-status: ## Show tsnet subnet router status and recent logs
	bash ./scripts/tsnet-router.sh status

tsnet-router-logs: ## Tail tsnet subnet router logs
	bash ./scripts/tsnet-router.sh logs
