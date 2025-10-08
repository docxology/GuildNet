# GuildNet

GuildNet helps teams launch and reach “workload servers” inside or near their network/cluster with:

- A local Go host app that proxies traffic using Tailscale (tsnet)
- A simple web UI to list, launch, and view logs
- A developer-friendly agent image (VS Code via code-server behind an iframe-friendly proxy)

Tailscale (tsnet) is required. For a deeper dive, see `architecture.md`.

## Goals

- Simple, HTTPS-first dev experience
- One host app binary with built-in tsnet
- Browser UI for managing and opening workspaces
- Agent image that exposes a ready-to-use IDE endpoint

## Architecture (overview)

- Host App (Go + tsnet)
  - Serves a local TLS endpoint and a Tailscale listener
  - Provides minimal APIs (health, images, servers, logs, launch)
  - Proxies to agents or in-cluster HTTP endpoints
- Web UI (SolidJS + Vite)
  - Lists servers, shows details/logs, and provides a Launch form
  - Pulls image presets from the backend (no hardcoding)
- Agent Image (code-server + Caddy)
  - Single-port HTTP (default 8080), iframe-friendly headers
  - Non-root, health at `/healthz`, password from env or generated
- Kubernetes Integration
  - Deployments + Services for agents, logs retrieval

## Quickstart

Prereqs: Go, Node.js, Docker (for agent builds), and access to Tailscale/Headscale. Ensure your Tailscale/Headscale settings are available to the host app (e.g., `~/.guildnet/config.json`). Optional helper: `scripts/sync-env-from-config.sh`.

1. Setup (UI deps + local TLS certs)

```sh
make setup
```

2. Run the backend (dev, tsnet + CORS)

```sh
# Optional: override origin/listen address
# ORIGIN=https://localhost:5173 LISTEN_LOCAL=127.0.0.1:8080 make dev-backend
make dev-backend
```

3. Run the UI (Vite)

```sh
# Optional: override API base, defaults to https://localhost:8080
# VITE_API_BASE=https://127.0.0.1:8080 make dev-ui
make dev-ui
```

4. Verify

```sh
# Backend health (self-signed):
curl -k https://127.0.0.1:8080/healthz
# Open the UI:
open https://localhost:5173
```

Tip: `make help` lists all common targets (build, test, lint, utilities like TLS checks and stop-all).

# Progress

- [x] Join/create Headscale/Tailscale network
- [x] Create Talos cluster running Tailscale Tailnet
- [x] Build & run code-server image inside Talos cluster
- [x] Create dashboard server to run scripts and report status
- [x] Create UI for dashboard server to join/create network, manage clusters and observe code servers
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
