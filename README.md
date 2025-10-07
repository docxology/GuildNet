# GuildNet

GuildNet is a lightweight platform to launch and reach “workload servers” inside or near your cluster/network, with a browser UI and a developer-friendly agent image that exposes a VS Code (code-server) session behind an iframe-friendly reverse proxy.

It includes:

- A Go Host App that embeds Tailscale (tsnet) to proxy traffic and expose simple APIs.
- A SolidJS web UI (Vite + Tailwind) to list servers, view details/logs, and launch new workloads.
- A containerized Agent image (code-server + Caddy) that serves an embeddable VS Code UI over one HTTP port.
- Kubernetes client integration to deploy workloads (Deployments + Services) and read logs.
- A Kubernetes Deployment + Service example for the agent.
- HTTPS-by-default for local dev, plus a helper script to generate shared certificates.
  Note: Tailscale (tsnet) is mandatory; the host app always runs via Tailscale.

## Features

- tsnet-powered connectivity (no external tailscaled): listens on a local TLS address and a Tailscale listener.
- Minimal HTTP APIs:
  - `GET /healthz` – liveness
  - `GET /api/ping?addr=<host-or-ip>:<port>` – TCP dial RTT over tsnet dialer
  - `GET /proxy?to=<ip:port>&path=/...` – reverse proxy to in-cluster HTTP (explicit)
  - `GET /proxy/server/{id}/...` – server-aware proxy; backend resolves the upstream from K8s metadata
  - `GET /sse/logs` – Server-Sent Events stream for logs
- UI features (SolidJS):
  - Servers list and detail pages
  - Logs stream (SSE) and historic log fetch
  - Launch form with presets (includes the GuildNet Agent image), env/labels, ports, and resource hints
  - API base configurable via `VITE_API_BASE`
- Agent image (images/agent):
  - code-server behind Caddy with iframe-friendly headers (CSP frame-ancestors, removes X-Frame-Options)
  - Single port (default 8080) exposed; code-server listens on loopback and is proxied by Caddy
  - Non-root, tini as PID1, health endpoint `/healthz`, landing at `/landing`
  - Password via `PASSWORD` or auto-generated and stored in `/data/.code-server-password`
  - Volumes: `/workspace` and `/data`
  - Example K8s manifest: `k8s/agent-example.yaml`
- HTTPS-first dev experience:
  - Backend serves HTTPS locally with repo CA-signed certs (or self-signed fallback)
  - Vite dev server uses HTTPS with the same repo certs when available
  - Script to generate a shared local CA and issue certs for both sides

## Quick start (Tailscale/Headscale required)

1) Share Tailscale/Headscale config via .env

Create a `.env` at the repo root or generate it from your existing host config:

```bash
# Option A: generate from ~/.guildnet/config.json
scripts/sync-env-from-config.sh

# Option B: create manually
cp .env.example .env
vi .env  # set TS_LOGIN_SERVER, TS_AUTHKEY, TS_HOSTNAME, TS_ROUTES
```

2) Initialize or run the backend

```bash
make build
# Non-interactive init from .env (if config missing)
scripts/dev-host-run.sh --no-certs --origin https://localhost:5173 & sleep 1; kill %1 || true
./bin/hostapp init  # optional interactive init
```

3) Build and run backend (TLS, CORS prepped, tsnet mandatory)

```bash
# one-liner helper (build + certs + CORS + serve)
make dev-run ORIGIN=https://localhost:5173
# or manually (ensure CA-signed server cert is present)
make build && scripts/generate-server-cert.sh -f && FRONTEND_ORIGIN=https://localhost:5173 ./bin/hostapp serve
```

4) Run the UI (HTTPS)

```bash
cd ui
npm install
VITE_API_BASE=https://127.0.0.1:8080 npm run dev
```

5) Verify it works

```bash
# API health (self-signed):
curl -k https://127.0.0.1:8080/healthz
# UI loads over https://localhost:5173 and can call the backend
```

- `VITE_API_BASE=https://127.0.0.1:8080` (or your host app URL)

## Agent image (code-server + Caddy)

Build locally:

```bash
docker build -t guildnet/agent:dev images/agent
```

Run locally:

```bash
mkdir -p workspace data
docker run --rm \
  -p 8080:8080 \
  -e PASSWORD=changeme \
  -v "$(pwd)/workspace:/workspace" \
  -v "$(pwd)/data:/data" \
  guildnet/agent:dev
```

Open http://localhost:8080 for code-server; check /healthz. Headers allow iframe embedding (`frame-ancestors 'self' *`).

### Defaults


**Default agent port:** The agent always exposes code-server via Caddy on port 8080 (HTTP, iframe-friendly). The UI and backend will auto-detect and use this port.

**Agent host normalization:** If the server record provides a bare `AGENT_HOST` like `agent`, the UI will resolve it as `agent.<namespace>.svc.cluster.local` (default namespace `default`, override with `VITE_K8S_NAMESPACE`). You can also set `AGENT_HOST` to a FQDN (Service DNS or Pod IP) explicitly in deployments.

Allowlist has been removed; proxying relies on your trusted Tailnet. The `allowlist` setting in config is ignored.

Kubernetes example (Deployment + Service): see `k8s/agent-example.yaml`.

## API usage examples

Health:

```bash
curl -k https://127.0.0.1:8080/healthz
```

Ping:

```bash
curl -k 'https://127.0.0.1:8080/api/ping?addr=10.96.0.1:443'
```

Reverse proxy:

```bash
# Explicit target
curl -k 'https://127.0.0.1:8080/proxy?to=10.96.0.1:443&path=/'

# Server-aware target
curl -k 'https://127.0.0.1:8080/proxy/server/<id>/'
```

Logs SSE stream:

```bash
# Tail and stream logs for a target server id
curl -Nk 'https://127.0.0.1:8080/sse/logs?target=demo-1&level=info&tail=50'
```

## Optional: run a local Headscale (dev only)

This repo includes a convenience script to launch a Headscale server in Docker for local/dev use. You only need one Headscale per cluster/team; if you already have one, skip this section.

```bash
# start local Headscale on http://127.0.0.1:8081
scripts/headscale-run.sh up

# create a user and issue a reusable pre-auth key
scripts/headscale-run.sh create-user myuser
scripts/headscale-run.sh preauth-key myuser

# point GuildNet host app at it
# ~/.guildnet/config.json -> login_server: http://127.0.0.1:8081
# use the printed pre-auth key for auth_key and set a hostname
```

Notes:
- For production, deploy Headscale properly with HTTPS and persistent storage.
- Some clients require HTTPS; if needed locally, put a trusted reverse proxy in front and set `login_server` to its https URL.

## Paths & config

- Config: `~/.guildnet/config.json`
- State: `~/.guildnet/state/`
- Env: `FRONTEND_ORIGIN` (CORS allow for dev UI, default `https://localhost:5173`)
- The host app always starts a Tailscale listener in addition to the local TLS listener; ensure your Headscale/Tailscale settings are valid in `~/.guildnet/config.json`.

## Security notes

- The host app does not enforce an allowlist for `/api/ping` or `/proxy`.
- The agent image runs as non-root and doesn’t bundle secrets; set `PASSWORD` or persist `/data`.
- For production, use proper certificates (or terminate TLS at ingress) and PVCs for `/data` and `/workspace`.
- Tailscale (tsnet) is required; disabling it is not supported.

## Development

- Makefile targets: `build`, `run`, `tidy`, `test`, `dev-run`, `talos-fresh`, `talos-upgrade`.
- UI: `npm run dev`, `preview`, `lint`, `format`.
- Tests (Go): `go test ./...`

## Talos helpers (optional)

Fresh cluster (wipe & recreate):
```bash
scripts/talos-fresh-deploy.sh \
  --cluster mycluster \
  --endpoint https://<control-plane-endpoint>:6443 \
  --cp 10.0.0.10,10.0.0.11 \
  --workers 10.0.0.20,10.0.0.21
```

In-place Talos OS upgrade (preserve data):
```bash
scripts/talos-upgrade-inplace.sh \
  --image ghcr.io/siderolabs/installer:v1.7.0 \
  --nodes 10.0.0.10,10.0.0.11,10.0.0.20,10.0.0.21 \
  --k8s v1.30.2   # optional
```

Notes:
- Ensure `talosctl` is installed and versions align with your target Talos.
- For fresh deploys, `talosctl reset --reboot` is used opportunistically before applying configs.

# Progress

- [ ] Join/create Headscale/Tailscale network
- [ ] Create Talos cluster inside Tailnet
- [ ] Build & run Code Server image inside Talos cluster
- [ ] Create dashboard server to run scripts and report status
- [ ] Create UI for dashboard server to join/create network, manage clusters and observe code servers
- [ ] Fully generic and configurable docker deploys via subdomain on tailnet
- [ ] Ensure multi-user support with orgs/clusters
- [ ] Run Ollama on host machine and OpenAI Codex inside code servers, opening terminal to interact with agent via web UI
- [ ] Event bus for agent-host communication (e.g. notify users of PR created, code pushed, etc) with web UI
- [ ] Add persistent storage to cluster via Longhorn, save code server data there
- [ ] Add Radicle for git hosting inside cluster, hook up to agent workflow for PR creation
- [ ] Create UI for code review and PR management
- [ ] Prompt engineering for agent workflows, provide templates and examples
- [ ] Add MCPs for agent integration/interaction/memory/thinking etc
- [ ] Add Obsidian or similar for personal and collective knowledge management inside cluster
- [ ] Add task management system inside cluster, hook up to agent workflows, with 2D/3D graphical interface
