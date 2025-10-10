# GuildNet

GuildNet is a private self-hostable stack that puts human-in-the-loop with agent prompting on top of a knowledge gardening and coding version control foundation. For end users, it has simple interfaces that attempt to bring the cost to experiment down whilst bringing the capacity to experiment up, so non-engineers can have appropriate guardrails and higher level tools that upgrade them to collaborate fast, whilst allowing engineers and agents to ensure architectural integrity across time. Eventually it will have a DAG for tasks, prompting templates & infrastructure for robust agentic workflows, and much more.

## Key components

### Distributed private network cluster

- **Host App**: A local server that runs on all machines and exposes the UI to the network via tsnet as well as reverse-proxies traffic between the Talos cluster.
- **Talos Cluster**: A Kubernetes cluster running on Talos OS, which hosts the code-server instances and other services.
- **Tailscale/Headscale**: Used for secure networking, allowing devices to connect to the Host App and Talos cluster.
- **UI**: A web interface for users to interact with the system, manage clusters, and access code servers.
- **Image Registry**: A private Docker image registry running within the Talos cluster to store and manage container images.

## Services

- **Persistent Storage**: Provides persistent storage for code-server instances, ensuring data is retained across restarts.
- **Event Bus**: Facilitates communication between servers and hosts, enabling scheduling, notifications and updates.
- **Scheduler**: Distributes workloads across available server instances based on current load and availability.
- **Load Balancer**: Manages incoming requests and routes them to the appropriate server instance.
- **Public Tunnel**: Exposes services to the internet securely, allowing remote access to code servers and the UI.

### Agent coding workflow

- **code-server**: Provides a web-based VSCode environment for coding and interaction.
- **Ollama**: Runs locally on the host machines to provide LLM capabilities for agents.
- **OpenAI Codex**: Used within code-server instances to assist with coding tasks.
- **Radicle**: A decentralized git hosting solution for managing code repositories within the cluster.
- **Knowledge Base**: A system for storing and managing knowledge, integrated with agent workflows.

## Quickstart

Prereqs: Go, Node.js, Docker (for agent builds), and access to Tailscale/Headscale. Ensure your Tailscale/Headscale settings are available to the host app (e.g., `~/.guildnet/config.json`). Optional helper: `scripts/sync-env-from-config.sh`.

1. Setup (UI deps + local TLS certs)

```sh
make setup
```

2. Build and run (prod mode only)

```sh
# Build backend + UI, deploy DB manifest (best-effort), then run hostapp
make run
```

3. Verify

```sh
# Backend health (self-signed):
curl -k https://127.0.0.1:8080/healthz
# Open the UI:
open https://127.0.0.1:8080
```

Tip: `make help` lists all common targets (build, test, lint, CRD apply, utilities).

## Scripts and Makefile responsibilities (spec)

Core setup scripts (one per component):
- `scripts/setup-headscale.sh`
  - Start/ensure Headscale (Docker) is running.
  - Detect LAN bind and sync `.env` (TS_LOGIN_SERVER, HEADSCALE_URL).
  - Bootstrap a user and reusable preauth key; write TS_AUTHKEY into `.env`.
- `scripts/setup-tailscale.sh`
  - Ensure IP forwarding (invokes `scripts/enable-ip-forwarding.sh`, may prompt once for sudo).
  - Install Tailscale client if missing; bring up router advertising `TS_ROUTES`.
  - Approve advertised routes in Headscale (best effort).
- `scripts/setup-talos.sh`
  - Orchestrates modular Talos steps:
    - `scripts/setup-talos-preflight.sh` (reachability + overlay checks)
    - `scripts/setup-talos-config.sh` (talosctl gen config)
    - `scripts/setup-talos-apply.sh` (reset/apply/bootstrap)
    - `scripts/setup-talos-wait-kube.sh` (fetch kubeconfig, wait for API/nodes)
  - Kubeconfig is written to `~/.guildnet/kubeconfig` and exported for subsequent kubectl operations.

Support/verification scripts:
- `scripts/enable-ip-forwarding.sh` – idempotent forwarding enable (sudo).
- `scripts/detect-lan-and-sync-env.sh` – sync `.env` URLs to reachable LAN URL.
- `scripts/rethinkdb-setup.sh` – optional DB service deployment/validation.
- `scripts/verify_cluster.sh` – extra k8s readiness checks (future).

Make targets:
- `make setup-headscale` – runs Headscale setup.
- `make setup-tailscale` – runs Tailscale router setup.
- `make setup-talos` – runs Talos setup.
- `make setup-all` – runs all three in order.

Kubeconfig:
- Default path is user-scoped: `~/.guildnet/kubeconfig`.
- The Makefile exports `KUBECONFIG=$(HOME)/.guildnet/kubeconfig` for kubectl-based targets (e.g., `crd-apply`).

Operational tasks:
- Join a device to Headscale/Tailscale: use the preauth key in `.env` (TS_AUTHKEY) on the device; subnet router uses `scripts/tailscale-router.sh` via `make router-up`.
- Tear down:
  - `make headscale-down` to stop/remove container.
  - `make router-down` to bring down router.
  - `make clean` to remove build artifacts; `scripts/cleanup.sh --all` to purge local state.

## No-DNS overlay (tsnet) quickstart

Run everything over an embedded tsnet overlay without installing Tailscale on the host or using MagicDNS.

1. Ensure a Headscale/Tailscale control server is reachable and generate a preauth key.
2. Create `.env` with at least:
	- `TS_LOGIN_SERVER=http://127.0.0.1:8082`
	- `TS_AUTHKEY=<preauth-key>`
	- `TS_HOSTNAME=<your-hostname>`
	- `TS_ROUTES=10.0.0.0/24,10.96.0.0/12,10.244.0.0/16`
3. Choose a route propagation method (one is enough):
	 - Option A (recommended to bootstrap): Host subnet router on a LAN machine with native Tailscale
		 - `make router-install` (one-time)
		 - `make router-up` (advertises TS_ROUTES, includes 10.0.0.0/24 by default)
		 - `make router-status` (verify it’s up)
	 - Option B (containerized or in-cluster):
		 - Local helper: `make tsnet-subnet-router` then `make run-subnet-router` on a machine that can reach 10.0.0.0/24
		 - Or deploy the DaemonSet snippet from `scripts/talos-vm-up.sh` into Kubernetes later
4. Start the hostapp: `make run` and open `https://127.0.0.1:8080`.
5. Agents register via `/api/v1/agents/register` and can be resolved with `/api/v1/resolve?id=...`.

No DNS is required—numeric IPs are resolved via the registry and routed by tsnet.

## Networking / Multi-Device

Run the Host App on any tailnet device; others reach it at `https://<ts-hostname-or-ip>:443`. Include that hostname/IP in the server certificate SANs (see cert generation script) or accept the self-signed cert in dev.

## Workspace Lifecycle

1. UI POST `/api/jobs` with image (and optional env/ports — enrichment in progress).
2. Host App creates a Workspace CR (name derived from job) in the target namespace.
3. Workspace controller reconciles to Deployment + Service.
4. Status fields (phase, proxyTarget) updated when Pod and Service become Ready.
5. UI lists Workspaces via `/api/servers` (CRD-backed) and proxy iframe loads `/proxy/server/{id}/`.

Proxy resolution order:
1. `status.proxyTarget` (scheme://host:port) if set.
2. Fallback: Service ClusterIP + chosen port (prefers 8443/443 for HTTPS, then 8080, else first port).

## Current API Surface

```
GET  /healthz
GET  /api/ui-config
GET  /api/images
GET  /api/image-defaults?image=<ref>
GET  /api/servers
GET  /api/servers/{id}
GET  /api/servers/{id}/logs?level=&limit=
POST /api/jobs
POST /api/admin/stop-all   (also /api/stop-all)
GET  /sse/logs?target=&level=&tail=
GET  /api/proxy-debug
/  (static UI served from ui/dist if present; set UI_DEV_ORIGIN to proxy Vite explicitly)
/proxy[...] (reverse proxy variants)
```

Stop-all permission: Allowed if at least one Capability matches action `stopAll` and its selector matches the Workspace labels; if zero Capability objects exist, action is allowed (permissive prototype semantics).

## Capability Prototype

- Capability CRD spec fields used: `actions[]`, `selector.matchLabels`.
- Cache refresh interval: ~10s (config code sets 10s in Host App main).
- Unsupported (ignored) fields: constraints, rate limits, images, ports.

## Certificate Strategy

Preference order:
1. `./certs/server.crt|server.key`
2. `./certs/dev.crt|dev.key`
3. `~/.guildnet/state/certs/server.crt|server.key` (auto self-signed)

Regenerate with SANs: `scripts/generate-server-cert.sh -H "localhost,127.0.0.1,<ts-hostname>,<ts-ip>" -f`.

## Runtime Env

| Variable | Purpose |
|----------|---------|
| LISTEN_LOCAL | Local HTTPS bind address (e.g. 127.0.0.1:8080) |
| FRONTEND_ORIGIN | CORS allowlist origin (default: https://localhost:5173 for convenience) |
| UI_DEV_ORIGIN | Optional Vite dev origin (proxied at / when set) |
| HOSTAPP_DISABLE_API_PROXY | Dial services directly (ClusterIP) instead of API pod proxy |
| WORKSPACE_DOMAIN | (Future) Ingress domain base (currently unused in CRD prototype) |

## Logs

- REST: `/api/servers/{id}/logs` returns last N lines (default 200).
- SSE: `/sse/logs` streams recent lines then heartbeats every 20s.

## Limitations / Known Gaps (Prototype)

- No user authentication / multi-tenant isolation.
// Workspace spec now includes image, env, and ports from `/api/jobs` requests.
- Permission system is minimal; destructive actions only.
- No metrics or structured tracing yet.
- Logs tail only; no incremental follow streaming from Kubernetes watch.
- Ingress / external exposure flows are placeholders for future design.

## Contributing / Extending

Focus areas that add immediate value:

- Add env + ports mapping from JobSpec into Workspace spec.
- Improve pod selection & multi-replica log aggregation.
- Introduce basic Prometheus metrics (proxy request count, workspace phase gauge).
- Harden error responses with structured JSON codes.

## License

Prototype – license to be defined.

# Progress

- [x] Join/create Headscale/Tailscale network
- [x] Create Talos cluster running Tailscale Tailnet
- [x] Build & run code-server image inside Talos cluster
- [x] Create dashboard server to run scripts and report status
- [x] Create UI for dashboard server to join/create network, manage clusters and observe code servers
- [ ] Ensure multi-user support with orgs/clusters
- [ ] Automatic TLS certs for tailnet access
- [ ] Fully generic and configurable docker deploys via subdomain on tailnet
- [ ] Docker image registry inside Talos cluster
- [ ] Run Ollama on host machine and OpenAI Codex inside code servers, opening terminal to interact with agent via web UI
- [ ] Event bus for agent-host communication (e.g. notify users of PR created, code pushed, etc) with web UI
- [ ] Add persistent storage to cluster via Longhorn, save code server data there
- [ ] Add Radicle for git hosting inside cluster, hook up to agent workflow for PR creation
- [ ] Create UI for code review and PR management
- [ ] Prompt engineering for agent workflows, provide templates and examples
- [ ] Add MCPs for agent integration/interaction/memory/thinking etc
- [ ] Add Obsidian or similar for personal and collective knowledge management inside cluster
- [ ] Add task management system inside cluster, hook up to agent workflows, with 2D/3D graphical interface
