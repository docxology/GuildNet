BINARY := hostapp
PKG := ./...

# Defaults (override as needed)
ORIGIN ?= https://localhost:5173
LISTEN_LOCAL ?= 127.0.0.1:8080
VITE_API_BASE ?= https://localhost:8080

.PHONY: all help \
	build build-backend build-ui \
	run dev-backend dev-run dev-ui \
	test lint tidy clean setup ui-setup \
	health tls-check-backend tls-check-ui regen-certs stop-all \
	talos-fresh talos-upgrade agent-build \
	crd-apply operator-run operator-build db-health
rethink-deploy: ## Deploy RethinkDB (single replica) into current kube-context
	kubectl apply -f k8s/rethinkdb.yaml
	@echo "Waiting for RethinkDB Service external IP (if using LoadBalancer)..."
	@kubectl get svc rethinkdb -o wide || true

rethink-info: ## Show RethinkDB Service address and hints for RETHINKDB_ADDR
	@echo "RethinkDB Service:"; kubectl get svc rethinkdb -o wide || true; echo
	@echo "If hostapp runs outside the cluster, set RETHINKDB_ADDR to <EXTERNAL-IP>:28015 (or use 'kubectl port-forward svc/rethinkdb 28015')."
	@echo "If hostapp runs in-cluster, it can reach 'rethinkdb:28015' directly."


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

# ---------- Run / Dev ----------
run: build-backend ## Run compiled backend (serve)
	./bin/$(BINARY) serve

dev-all: ## Run backend and UI in dev mode (tsnet + Vite), CORS origin=$(ORIGIN)
	$(MAKE) build-backend && $(MAKE) -j2 dev-backend dev-ui

dev-backend: ## Run backend in dev mode (tsnet), CORS origin=$(ORIGIN)
	LISTEN_LOCAL=$(LISTEN_LOCAL) ORIGIN=$(ORIGIN) $(MAKE) dev-run

# Dev helper: builds, generates certs, sets CORS origin, runs with Tailscale (tsnet)
# Usage examples:
#   make dev-run                    # default ORIGIN=$(ORIGIN)
#   make dev-backend ORIGIN=https://app.example.com RENEW_CERTS=1
dev-run: ## Low-level dev runner (invokes scripts/dev-host-run.sh)
	@set -e; \
	ARGS=""; \
	if [ -n "$(RENEW_CERTS)" = "1" ]; then ARGS="$$ARGS --no-certs"; fi; \
	if [ -n "$(ORIGIN)" ]; then ARGS="$$ARGS --origin $(ORIGIN)"; fi; \
	LISTEN_LOCAL=$(LISTEN_LOCAL) sh ./scripts/dev-host-run.sh $$ARGS

# ---------- DB / Health ----------
db-health: ## Check database API and report availability
	@echo "Checking backend health and DB API..."; \
	(set -x; curl -sk https://127.0.0.1:8080/healthz); echo; \
	(set -x; curl -sk https://127.0.0.1:8080/api/db) || true; echo

dev-ui: ## Run UI (Vite) pointing at backend API ($(VITE_API_BASE))
	cd ui && VITE_API_BASE=$(VITE_API_BASE) npm run dev

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

tls-check-ui: ## Show TLS info for Vite :5173
	echo | openssl s_client -connect localhost:5173 -servername localhost -tls1_2 2>/dev/null | head -n 20

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

# ---------- Talos helpers ----------
# Examples:
#   make talos-fresh FRESH_ARGS="--cluster myc --endpoint https://10.0.0.10:6443 --cp 10.0.0.10 --workers 10.0.0.20"
#   make talos-upgrade UPGRADE_ARGS="--image ghcr.io/siderolabs/installer:v1.7.0 --nodes 10.0.0.10,10.0.0.20 --k8s v1.30.2"

talos-fresh: ## Talos cluster fresh deploy
	bash ./scripts/talos-fresh-deploy.sh $(FRESH_ARGS)

talos-upgrade: ## Talos cluster in-place upgrade
	bash ./scripts/talos-upgrade-inplace.sh $(UPGRADE_ARGS)
