BINARY := hostapp
PKG := ./...

# Defaults (override as needed)
LISTEN_LOCAL ?= 127.0.0.1:8080

# User-scoped kubeconfig location (used by scripts and docs)
GN_KUBECONFIG ?= $(HOME)/.guildnet/kubeconfig

# Provisioner choice: lan | forward | vm
PROVIDER ?= lan

.PHONY: all help \
	build build-backend build-ui \
	run \
	test lint tidy clean setup ui-setup \
	health tls-check-backend regen-certs stop-all \
	talos-fresh talos-upgrade agent-build \
	crd-apply operator-run operator-build db-health \
	setup-headscale setup-tailscale setup-talos setup-all \
	bootstrap-sudo setup-all-provision talos-provision-vm \
	deploy-k8s-addons deploy-operator deploy-hostapp verify-e2e \
	diag-router diag-talos diag-k8s diag-db headscale-approve-routes


all: build ## Build backend and UI

# ---------- Help ----------
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make [target]\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---------- Setup ----------
setup: ui-setup regen-certs ## One-time setup: install UI deps and generate local TLS certs

ui-setup: ## Install UI dependencies (npm ci)
	cd ui && npm ci

regen-certs: ## Regenerate local server TLS certificate
	./scripts/generate-server-cert.sh -f

setup-headscale: ## Setup Headscale (Docker) and bootstrap preauth
	bash ./scripts/setup-headscale.sh

setup-tailscale: ## Setup Tailscale router (enable forwarding, up, approve routes)
	bash ./scripts/setup-tailscale.sh

setup-talos: ## Fresh deploy Talos and validate
	bash ./scripts/setup-talos.sh

setup-all: ## Run Headscale, Tailscale, and Talos setup in order
	$(MAKE) setup-headscale
	$(MAKE) setup-tailscale
	@if [ "$(PROVIDER)" = "vm" ]; then \
		$(MAKE) talos-provision-vm; \
	else \
		$(MAKE) setup-talos; \
	fi
	$(MAKE) deploy-k8s-addons
	$(MAKE) deploy-operator || true
	$(MAKE) deploy-hostapp || true
	$(MAKE) verify-e2e || true

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
	bash ./scripts/run-hostapp.sh

# ---------- DB / Health ----------
db-health: ## Check database API and report availability
	@echo "Checking backend health and DB API..."; \
	(set -x; curl -sk https://$(LISTEN_LOCAL)/healthz); echo; \
	(set -x; curl -sk https://$(LISTEN_LOCAL)/api/db) || true; echo

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
	curl -k https://$(LISTEN_LOCAL)/healthz || true

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
		KUBECONFIG=$(GN_KUBECONFIG) kubectl apply -f $$f >/dev/null || exit 1; \
	done; echo "CRDs applied"

operator-run: ## Run workspace operator (controller-runtime manager) locally
	go run ./cmd/hostapp --mode operator 2>&1 | sed 's/^/[operator] /'

agent-build: ## Build agent image (see scripts)
	sh ./scripts/agent-build-load.sh

# ---------- Host subnet router (native tailscale) ----------
.PHONY: router-install router-up router-down router-status router-grant-operator router-daemon router-daemon-sudo router-grant-operator-sudo

router-install: ## Install native tailscale client (host subnet router)
	bash ./scripts/tailscale-router.sh install

router-daemon: ## Ensure tailscaled is running (non-interactive, best effort)
	- systemctl --user enable --now tailscaled 2>/dev/null || true
	- systemctl enable --now tailscaled 2>/dev/null || sudo -n systemctl enable --now tailscaled 2>/dev/null || true
	- service tailscaled start 2>/dev/null || sudo -n service tailscaled start 2>/dev/null || true

router-daemon-sudo: ## Ensure tailscaled is running (sudo, prompts if needed)
	sudo systemctl enable --now tailscaled || sudo service tailscaled start || true

router-grant-operator: ## Allow current user to run tailscale commands without sudo prompts
	- sudo -n tailscale set --operator=$$USER 2>/dev/null || true
	@echo "If the above failed due to sudo, run: make router-grant-operator-sudo"

router-grant-operator-sudo: ## Grant operator with sudo (prompts once)
	sudo tailscale set --operator=$$USER || true

router-up: ## Bring up host subnet router (advertise TS_ROUTES)
	bash ./scripts/tailscale-router.sh up

router-down: ## Bring down host subnet router
	bash ./scripts/tailscale-router.sh down

router-status: ## Show tailscale router status
	bash ./scripts/tailscale-router.sh status

# ---------- Local Headscale (LAN bind) ----------
.PHONY: headscale-up headscale-down headscale-status env-sync-lan headscale-bootstrap local-overlay-up headscale-approve-routes

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

headscale-approve-routes: ## Approve tailscale routes for the router in Headscale
	bash ./scripts/headscale-approve-routes.sh

# ---------- Talos helpers ----------
# Examples:
#   make talos-fresh FRESH_ARGS="--cluster myc --endpoint https://10.0.0.10:6443 --cp 10.0.0.10 --workers 10.0.0.20"
#   make talos-upgrade UPGRADE_ARGS="--image ghcr.io/siderolabs/installer:v1.7.0 --nodes 10.0.0.10,10.0.0.20 --k8s v1.30.2"

# Export KUBECONFIG for kubectl invocations that run via Make targets
export KUBECONFIG := $(GN_KUBECONFIG)

# ---------- Provision / Addons / Deploy / Verify ----------
.PHONY: talos-provision-vm deploy-k8s-addons deploy-operator deploy-hostapp verify-e2e diag-router diag-talos diag-k8s diag-db

talos-provision-vm: ## Provision a local Talos dev cluster (VM/provider)
	bash ./scripts/talos-vm-up.sh

deploy-k8s-addons: ## Install MetalLB (pool from .env), CRDs, imagePullSecret, DB
	bash ./scripts/deploy-metallb.sh
	$(MAKE) crd-apply
	bash ./scripts/k8s-setup-registry-secret.sh || true
	bash ./scripts/rethinkdb-setup.sh || true

deploy-operator: ## Deploy operator (placeholder manifests)
	bash ./scripts/deploy-operator.sh

deploy-hostapp: ## Run hostapp locally (or deploy in cluster if configured)
	$(MAKE) run

verify-e2e: ## Verify router, routes, Talos reachability, kube API, DB
	bash ./scripts/verify-e2e.sh

# ---------- Diagnostics ----------

diag-router: ## Show tailscale status and headscale routes
	$(MAKE) router-status || true
	docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$$' && docker exec -i guildnet-headscale headscale routes list || true

diag-talos: ## Show talosctl reachability for CP/WK nodes
	bash -lc 'set -a; [ -f ./.env ] && . ./.env; for n in $${CP_NODES//,/ }; do echo "-- $$n"; done'

diag-k8s: ## Show kube API status and nodes
	kubectl --request-timeout=5s get --raw='/readyz?verbose' || true
	kubectl get nodes -o wide || true

diag-db: ## Print DB service details
	bash ./scripts/rethinkdb-setup.sh || true
