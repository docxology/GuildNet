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

2. Run the backend (dev, tsnet + CORS)

```sh
# Optional: override origin/listen address
# ORIGIN=https://127.0.0.1:8080 LISTEN_LOCAL=127.0.0.1:8080 make dev-backend
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
open https://127.0.0.1:8080
```

Tip: `make help` lists all common targets (build, test, lint, utilities like TLS checks and stop-all).

## Share and join network

As an organizer (already running a Host App):

- Create a join file you can send to teammates (contains the Host App URL, optional CA, and optional pre-auth key):
  - scripts/create_join_info.sh --hostapp-url https://<your-ts-fqdn>:443 --include-ca certs/server.crt --login-server https://headscale.example.com --auth-key tskey-... --hostname teammate-1 --name "Dev Cluster" --out guildnet.config
  - Share the resulting guildnet.config securely.

As a teammate (to join):

- Run: scripts/join.sh /path/to/guildnet.config
- If it includes a pre-auth key, your ~/.guildnet/config.json will be created so you can run your own Host App if desired.
- Open the shared Host App URL in a browser and you’re in.

Verify the flow end-to-end (isolated):

- scripts/verify_join.sh --hostapp-url https://<your-ts-fqdn>:443 --include-ca certs/server.crt --login-server https://headscale.example.com --auth-key tskey-...
- The script runs in a temp HOME, calls create_join_info.sh and join.sh, and checks /healthz.

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
