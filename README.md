# GuildNet

GuildNet is a lightweight platform to launch and reach “workload servers” inside or near your cluster/network, with a browser UI and a developer-friendly agent image that exposes a VS Code (code-server) session behind an iframe-friendly reverse proxy.

It includes:

- A Go Host App that embeds Tailscale (tsnet) to proxy traffic and expose simple APIs.
- A SolidJS web UI (Vite + Tailwind) to list servers, view details/logs, and launch new workloads.
- A containerized Agent image (code-server + Caddy) that serves an embeddable VS Code UI over one HTTP port.
- A Kubernetes Deployment + Service example for the agent.
- HTTPS-by-default for local dev, plus a helper script to generate shared certificates.

## Features

- tsnet-powered connectivity (no external tailscaled): listens on a local TCP address and a Tailscale listener.
- Minimal HTTP APIs:
  - `GET /healthz` – liveness
  - `GET /api/ping?addr=<host-or-ip>:<port>` – TCP dial RTT over tsnet dialer with allowlist enforcement
  - `GET /proxy?to=<ip:port>&path=/...` – reverse proxy to in-cluster HTTP, allowlist-gated
  - `WS /ws/echo` – WebSocket echo (dev/test)
- UI features (SolidJS):
  - Servers list and detail pages
  - Logs stream (WebSocket) and historic log fetch
  - Launch form with presets (includes the GuildNet Agent image), env/labels, ports, and resource hints
  - API/WS base configurable via `VITE_API_BASE`/`VITE_WS_BASE`
- Agent image (images/agent):
  - code-server behind Caddy with iframe-friendly headers (CSP frame-ancestors, removes X-Frame-Options)
  - Single port (default 8080) exposed; code-server listens on loopback and is proxied by Caddy
  - Non-root, tini as PID1, health endpoint `/healthz`, landing at `/landing`
  - Password via `PASSWORD` or auto-generated and stored in `/data/.code-server-password`
  - Volumes: `/workspace` and `/data`
  - Example K8s manifest: `k8s/agent-example.yaml`
- HTTPS-first dev experience:
  - Backend serves HTTPS locally with a self-signed cert (or your own)
  - Vite dev server uses HTTPS (self-signed or provided)
  - Script to generate a shared local CA and issue certs for both sides

## Quick start

1) Build and run backend (TLS, CORS prepped)

```bash
# one-liner helper (build + certs + CORS + serve; skips tsnet for local dev)
make dev-run ORIGIN=https://localhost:5173
# or manually
make build && scripts/generate-certs.sh && FRONTEND_ORIGIN=https://localhost:5173 DEV_NO_TSNET=1 ./bin/hostapp serve
```

2) Run the UI (HTTPS)

```bash
cd ui
npm install
VITE_API_BASE=https://127.0.0.1:8080 \
VITE_WS_BASE=wss://127.0.0.1:8080 \
npm run dev
```

3) Verify it works

```bash
# API health (self-signed):
curl -k https://127.0.0.1:8080/healthz
# UI loads over https://localhost:5173 and can call the backend
```

- `VITE_API_BASE=https://127.0.0.1:8080` (or your host app URL)
- `VITE_WS_BASE=wss://127.0.0.1:8080`

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
curl -k 'https://127.0.0.1:8080/proxy?to=10.96.0.1:443&path=/'
```

WebSocket echo:

```bash
# Using wscat (install with npm i -g wscat)
wscat -c wss://127.0.0.1:8080/ws/echo --no-check
```

## Paths & config

- Config: `~/.guildnet/config.json`
- State: `~/.guildnet/state/`
- Env: `FRONTEND_ORIGIN` (CORS allow for dev UI, default `https://localhost:5173`)

## Security notes

- The host app enforces an allowlist for `/api/ping` and `/proxy`.
- The agent image runs as non-root and doesn’t bundle secrets; set `PASSWORD` or persist `/data`.
- For production, use proper certificates (or terminate TLS at ingress) and PVCs for `/data` and `/workspace`.

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
