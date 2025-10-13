BINARY := hostapp
PKG := ./...

# Defaults (override as needed)
LISTEN_LOCAL ?= 127.0.0.1:8090

# User-scoped kubeconfig location (used by scripts and docs)
GN_KUBECONFIG ?= $(HOME)/.guildnet/kubeconfig

# Provisioner choice: lan | forward | vm
PROVIDER ?= lan

.PHONY: all help \
	build build-backend build-ui \
	run \
	test lint tidy clean setup ui-setup \
	health tls-check-backend regen-certs stop-all \
	agent-build \
	crd-apply operator-run operator-build db-health \
	setup-headscale setup-tailscale setup-all \
	kind-up \
	deploy-k8s-addons deploy-operator deploy-hostapp verify-e2e \
	diag-router diag-k8s diag-db headscale-approve-routes


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

## (Talos flow removed) Use existing Kubernetes and per-cluster router.

setup-all: ## One-command: Headscale up -> LAN sync -> ensure Kubernetes (kind) -> Headscale namespace -> router DS -> addons -> operator -> hostapp -> verify
	@CL=$${CLUSTER:-$${GN_CLUSTER_NAME:-default}}; \
	echo "[setup-all] Using cluster: $$CL"; \
	$(MAKE) headscale-up; \
	$(MAKE) env-sync-lan; \
	# Ensure Kubernetes is reachable; if not, bring up a local kind cluster and export kubeconfig
	ok=1; kubectl --request-timeout=3s get --raw=/readyz >/dev/null 2>&1 || ok=0; \
	if [ $$ok -eq 0 ]; then \
		$(MAKE) kind-up; \
	fi; \
	CLUSTER=$$CL $(MAKE) headscale-namespace; \
	CLUSTER=$$CL $(MAKE) router-ensure || true; \
	$(MAKE) deploy-k8s-addons || true; \
	$(MAKE) deploy-operator || true; \
	$(MAKE) deploy-hostapp || true; \
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
db-health: ## Check backend health summary
	@echo "Checking backend health..."; \
	(set -x; curl -sk https://$(LISTEN_LOCAL)/healthz); echo; \
	(set -x; curl -sk https://$(LISTEN_LOCAL)/api/health) || true; echo

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

tls-check-backend: ## Show TLS info for backend :8090
	echo | openssl s_client -connect 127.0.0.1:8090 -servername localhost -tls1_2 2>/dev/null | head -n 20

stop-all: ## Stop all managed workloads via admin API
	@curl -sk -X POST https://127.0.0.1:8090/api/admin/stop-all || curl -sk -X POST https://127.0.0.1:8090/api/stop-all || true

# ---------- CRD / Operator helpers ----------
CRD_DIR ?= config/crd
crd-apply: ## Apply (or update) GuildNet CRDs into current kube-context
	@[ -d $(CRD_DIR) ] || { echo "CRD dir $(CRD_DIR) missing"; exit 1; }
	@ok=1; kubectl --request-timeout=3s get --raw=/readyz >/dev/null 2>&1 || ok=0; \
	if [ $$ok -eq 0 ]; then \
		echo "[crd-apply] Kubernetes API not reachable or kubeconfig invalid; skipping"; \
	else \
		for f in $(CRD_DIR)/*.yaml; do \
			echo "kubectl apply -f $$f"; \
			KUBECONFIG=$(GN_KUBECONFIG) kubectl apply -f $$f >/dev/null || exit 1; \
		done; \
		echo "CRDs applied"; \
	fi

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

## (Talos helpers removed)

# Export KUBECONFIG for kubectl invocations that run via Make targets
export KUBECONFIG := $(GN_KUBECONFIG)

# ---------- Provision / Addons / Deploy / Verify ----------
.PHONY: deploy-k8s-addons deploy-operator deploy-hostapp verify-e2e diag-router diag-k8s diag-db

deploy-k8s-addons: ## Install MetalLB (pool from .env), CRDs, imagePullSecret, DB
	bash ./scripts/deploy-metallb.sh
	$(MAKE) crd-apply
	bash ./scripts/k8s-setup-registry-secret.sh || true
	bash ./scripts/rethinkdb-setup.sh || true

deploy-operator: ## Deploy operator (placeholder manifests)
	bash ./scripts/deploy-operator.sh

deploy-hostapp: ## Run hostapp locally (or deploy in cluster if configured)
	$(MAKE) run

verify-e2e: ## Verify router, routes, kube API, DB
	bash ./scripts/verify-e2e.sh

# ---------- Diagnostics ----------

diag-router: ## Show tailscale status and headscale routes
	$(MAKE) router-status || true
	docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$$' && docker exec -i guildnet-headscale headscale routes list || true

## (Talos diagnostics removed)

diag-k8s: ## Show kube API status and nodes
	kubectl --request-timeout=5s get --raw='/readyz?verbose' || true
	kubectl get nodes -o wide || true

diag-db: ## Print DB service details
	bash ./scripts/rethinkdb-setup.sh || true

# ---------- Network & Proxy ----------
router-ensure-novalidate: ## Deploy Tailscale router without server-side schema validation (bootstrap when API unreachable)
	TS_AUTHKEY=$${TS_AUTHKEY:-$${HEADSCALE_AUTHKEY:-}} kubectl apply --validate=false -f - <<'YAML'
	apiVersion: apps/v1
	kind: DaemonSet
	metadata:
	  name: tailscale-subnet-router
	  namespace: kube-system
	  labels: { app: tailscale-subnet-router }
	spec:
	  selector: { matchLabels: { app: tailscale-subnet-router } }
	  template:
	    metadata: { labels: { app: tailscale-subnet-router } }
	    spec:
	      hostNetwork: true
	      dnsPolicy: ClusterFirstWithHostNet
	      tolerations: [ { operator: Exists } ]
	      containers:
	      - name: tailscale
	        image: tailscale/tailscale:stable
	        securityContext: { capabilities: { add: [NET_ADMIN, NET_RAW] }, privileged: true }
	        env:
	        - { name: TS_AUTHKEY, value: "$${TS_AUTHKEY}" }
	        - { name: TS_LOGIN_SERVER, value: "$${TS_LOGIN_SERVER:-https://login.tailscale.com}" }
	        - { name: TS_ROUTES, value: "$${TS_ROUTES:-10.0.0.0/24,10.96.0.0/12,10.244.0.0/16}" }
		- { name: TS_HOSTNAME, value: "$${TS_HOSTNAME:-subnet-router}" }
	        volumeMounts: [ { name: state, mountPath: /var/lib/tailscale }, { name: tun, mountPath: /dev/net/tun } ]
		args: [ /bin/sh, -c, "set -e; /usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state & sleep 2; tailscale up --authkey=\"$${TS_AUTHKEY}\" --login-server=\"$${TS_LOGIN_SERVER:-https://login.tailscale.com}\" --advertise-routes=\"$${TS_ROUTES:-10.0.0.0/24,10.96.0.0/12,10.244.0.0/16}\" --hostname=\"$${TS_HOSTNAME:-subnet-router}\" --accept-routes; tail -f /dev/null" ]
	      volumes:
	      - { name: state, emptyDir: {} }
	      - { name: tun, hostPath: { path: /dev/net/tun, type: CharDevice } }
	YAML

set-cluster-proxy: ## Set per-cluster API proxy URL and force HTTP (usage: make set-cluster-proxy CLUSTER_ID=... PROXY=http://host:8001)
	@[ -n "$(CLUSTER_ID)" ] || { echo "CLUSTER_ID required"; exit 2; }
	@[ -n "$(PROXY)" ] || { echo "PROXY required (e.g., http://127.0.0.1:8001)"; exit 2; }
	@curl -sk -X PUT https://$(LISTEN_LOCAL)/api/settings/cluster/$(CLUSTER_ID) \
	  -H 'Content-Type: application/json' \
	  -d '{"api_proxy_url":"'"$(PROXY)"'","api_proxy_force_http":true}'

# New plain-K8S helpers
headscale-namespace: ## Ensure Headscale namespace and emit keys (CLUSTER=...)
	CLUSTER=$${CLUSTER:-$${GN_CLUSTER_NAME:-default}} bash ./scripts/headscale-namespace-and-keys.sh

router-ensure: ## Deploy Tailscale subnet router DaemonSet (uses tmp/cluster-<id>-headscale.json when present)
		@set -e; \
		CL=$${CLUSTER:-$${GN_CLUSTER_NAME:-}}; \
		if [ -z "$$CL" ]; then \
			CNT=$$(ls -1 tmp/cluster-*-headscale.json 2>/dev/null | wc -l | tr -d ' '); \
			if [ "$$CNT" = "1" ]; then \
				J=$$(ls -1 tmp/cluster-*-headscale.json); \
				CL=$$(basename "$$J" | sed -E 's/^cluster-(.+)-headscale\.json/\1/'); \
			fi; \
		fi; \
		: $${CL:=$${GN_CLUSTER_NAME:-default}}; \
		J=tmp/cluster-$$CL-headscale.json; \
		if [ ! -f $$J ]; then \
			CNT=$$(ls -1 tmp/cluster-*-headscale.json 2>/dev/null | wc -l | tr -d ' '); \
			if [ "$$CNT" = "1" ]; then \
				J=$$(ls -1 tmp/cluster-*-headscale.json); \
				CL=$$(basename "$$J" | sed -E 's/^cluster-(.+)-headscale\.json/\1/'); \
				echo "[router-ensure] Auto-detected cluster: $$CL"; \
			else \
				echo "Missing $$J; run: make headscale-namespace CLUSTER=$$CL"; exit 0; \
			fi; \
		fi; \
		if [ ! -f "$(GN_KUBECONFIG)" ]; then \
			echo "[router-ensure] No kubeconfig at $(GN_KUBECONFIG); skipping"; exit 0; \
		fi; \
		if ! kubectl version --request-timeout=3s >/dev/null 2>&1; then \
			echo "[router-ensure] Kubernetes API not reachable; skipping"; exit 0; \
		fi; \
		TS_AUTHKEY=$$(jq -r '.routerAuthKey' $$J); \
		TS_LOGIN_SERVER=$$(jq -r '.loginServer' $$J); \
		: $${TS_ROUTES:=$${GN_TS_ROUTES:-10.96.0.0/12,10.244.0.0/16}}; \
		: $${TS_HOSTNAME:=router-$$CL}; \
		TS_AUTHKEY="$$TS_AUTHKEY" TS_LOGIN_SERVER="$$TS_LOGIN_SERVER" TS_ROUTES="$$TS_ROUTES" TS_HOSTNAME="$$TS_HOSTNAME" bash ./scripts/deploy-tailscale-router.sh

plain-quickstart: ## Alias to setup-all for plain K8S flow
	$(MAKE) setup-all

# ---------- Kind (local Kubernetes) ----------
kind-up: ## Create a local kind cluster and write kubeconfig to $(GN_KUBECONFIG)
	bash ./scripts/kind-setup.sh
