# GuildNet

GuildNet is a private self-hostable stack that puts human-in-the-loop with agent prompting on top of a knowledge gardening and coding version control foundation. For end users, it has simple interfaces that attempt to bring the cost to experiment down whilst bringing the capacity to experiment up, so non-engineers can have appropriate guardrails and higher level tools that upgrade them to collaborate fast, whilst allowing engineers and agents to ensure architectural integrity across time. Eventually it will have a DAG for tasks, prompting templates & infrastructure for robust agentic workflows, and much more.

## Key components

### Distributed private network cluster

- **Host App**: A local server that runs on all machines and exposes the UI to the network via tsnet as well as reverse-proxies traffic into Kubernetes.
- **Kubernetes Cluster**: Your existing real-node Kubernetes. Each cluster can have its own Tailnet and router.
- **Tailscale/Headscale**: Used for secure networking. Each cluster can use a per‑cluster embedded tsnet connector and an in-cluster subnet router.
- **UI**: A web interface for users to interact with the system, manage clusters, and access code servers.
- **Image Registry**: A private Docker image registry running within the cluster to store and manage container images.

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

## Quickstart overview

Choose your path:

- Default path (Plain K8S with per-cluster Tailnets):
  1) Headscale up → 2) Per‑cluster Headscale namespace/keys → 3) Deploy in‑cluster Tailscale router → 4) Deploy addons/operator → 5) Run the Host App → 6) Verify
  - One command: `make setup-all`
  - Health: `make verify-e2e`

- Join existing pieces (e.g., your team already runs Headscale/Kubernetes):
  - Connect your Tailscale to their Headscale, ensure routes or deploy the router into the target cluster, acquire kubeconfig, then run the Server and open the UI.

When the Server is running:

- Local UI: https://127.0.0.1:8090
- Tailnet UI (from other devices): https://<ts-hostname-or-ip>:443

Tip: `make help` lists the most useful targets.

## 1) Headscale (create or use existing)

Create (Docker-based, local):

- Start Headscale and bind it to your LAN IP
  - `make headscale-up`
- Bootstrap a reusable pre-auth key and sync it into `.env`
  - `make headscale-bootstrap`
- (Optional) Adjust `.env` to use your LAN URL instead of 127.0.0.1
  - `make env-sync-lan`
- Inspect status
  - `make headscale-status`

Use existing:

- Obtain your Headscale URL and a pre-auth key from your admin.
- Set `TS_LOGIN_SERVER` and `TS_AUTHKEY` in `.env` (or keep them handy for Tailscale join steps below).

Tear down:

- `make headscale-down`

## 2) Tailscale (per‑cluster router and clients)

Goal: connect machines to the Tailnet and ensure routes to the cluster subnets are available.

Create (recommended, automated):

- Ensure per‑cluster Headscale namespace and keys
  - `make headscale-namespace CLUSTER=<id>` → writes `tmp/cluster-<id>-headscale.json`
- Deploy the in‑cluster Tailscale Subnet Router (privileged DaemonSet)
  - `make router-ensure CLUSTER=<id>`
- The Make default flow wires these steps for you: `make setup-all`

Join existing (another device):

- Install Tailscale, then connect to the Headscale with the pre-auth key
  - Login server: `TS_LOGIN_SERVER`
  - Auth key: `TS_AUTHKEY`
- After joining, you should see the Server’s tailnet address and any advertised routes. If you do not see the cluster subnets, ask the admin to run a subnet router and approve routes.

Notes:

- You can also use `sudo make setup-tailscale` to run the end-to-end router setup (enables IP forwarding, brings Tailscale up, attempts route approval).
- Route examples commonly include cluster/service/pod CIDRs (e.g., `10.96.0.0/12`, `10.244.0.0/16`) plus any node CIDRs.

## 3) Kubernetes

Two simple options to get a cluster for the quickstart:

- Local cluster (quick, developer):
  - `make kind-up` — creates a local kind cluster and writes the kubeconfig to `$(GN_KUBECONFIG)`.

- Full setup (one-command full demo stack):
  - `make setup-all` — runs the full provisioning flow (Headscale, router, cluster setup, addons, operator, hostapp, verify).

After running either command ensure the kubeconfig is available to the scripts (set `KUBECONFIG` or leave it at `~/.guildnet/kubeconfig`).

## 4) Server (Host App + UI)

One‑time local setup:

- Install UI deps and generate local TLS certs
  - `make setup`

Run the Server locally:

- `make run`
- Open https://127.0.0.1:8090
- From other Tailnet devices, open https://<ts-hostname-or-ip>:443

What happens when running:

- The Server exposes a single HTTPS origin with API + reverse proxy.
- It can create and reconcile Workspace resources into Deployments/Services.
- The UI can launch and open IDEs (e.g., code-server) via a proxy on the same origin.

TLS note:

- For other devices to connect without warnings, include your tailnet hostname/IP in the server cert SANs: `scripts/generate-server-cert.sh -H "localhost,127.0.0.1,<ts-fqdn>,<ts-ip>" -f`.

## Join vs Create: putting it together

- If your team already runs Headscale and a subnet router, and you have a kubeconfig:

  1. Join Tailscale with the provided pre-auth key (Headscale URL + key).
  2. Place your kubeconfig at `~/.guildnet/kubeconfig` (or set `KUBECONFIG`).
  3. `make setup` then `make run`, and open the UI.

-- If you’re starting fresh on a single machine:
  - Use the default `make setup-all` which will: start Headscale, create per‑cluster keys, deploy the router DS, install addons/operator, run Host App, and verify.

Either path ends with the same UI, where you can create workspaces and access them from any Tailnet device.

## Developer Quickstart (one-machine)

These steps are the quickest path to get a fully functional local development environment (microk8s or existing kubeconfig). The scripts automate registering the cluster with the Host App and attaching the kubeconfig so the UI can control Workspaces without manual DB edits.

1) Prepare (ensure tools installed): docker, kubectl, microk8s (or kind), jq, curl, python3
2) Provision or point to a cluster and run the verify flow:

```bash
# create or use existing kubeconfig; the script will try microk8s/minikube/k3d/kind
./scripts/verify-cluster.sh
```

This script will:
- create or reuse a local cluster (depending on environment and USE_KIND)
- build and load the operator image into microk8s/kind when needed
- deploy CRDs, operator, and RethinkDB
- generate `guildnet.config` and auto-attach the kubeconfig to the Host App
- start the Host App locally (if not already running)

3) Create and verify a workspace via helper:

```bash
# usage: scripts/verify-workspace.sh <cluster-id> <workspace-name> [image] [password]
CLUSTER_ID=$(curl -k https://127.0.0.1:8090/api/deploy/clusters | jq -r '.[] | select(.name=="microk8s-local") | .id')
./scripts/verify-workspace.sh "$CLUSTER_ID" verify-cs-local
```

If everything succeeded, you'll see the workspace reach Running, code-server logs, and the helper will probe the proxied root through the Host App.

Troubleshooting notes:
- If Host App returns empty lists for per-cluster endpoints, ensure `guildnet.config` was created and that the script attached the kubeconfig. You can attach manually:

```bash
curl -k -X POST "https://127.0.0.1:8090/api/deploy/clusters/<id>?action=attach-kubeconfig" -H 'Content-Type: application/json' -d '{"kubeconfig":"<base kubeconfig content here>"}'
```

- If `curl` fails with TLS read errors when probing the proxy, try --http1.1 or use a browser; the Host App terminates TLS locally and proxying large streaming responses can sometimes trigger client-side TLS library quirks in curl.

## Troubleshooting (quick)

- UI not reachable on another device:
  - Ensure you joined the same Tailnet and the server’s tsnet listener is up (default :443).
  - Ensure the certificate includes the tailnet hostname/IP or accept the self-signed cert for dev.
- Cluster services not reachable:
  - Verify a subnet router is advertising the cluster CIDRs and that routes are approved in Headscale.
- Kubernetes access errors:
  - Confirm `~/.guildnet/kubeconfig` points to the intended cluster and your RBAC permits operations.

## Useful commands

- `make help` – show available targets
- `make headscale-namespace` – create per‑cluster Headscale namespace and emit keys JSON
- `make router-ensure` – deploy per‑cluster Tailscale router DS (reads the keys JSON)
- `make verify-e2e` – end-to-end checks: headscale reachability, router DS readiness, kube API
- `make clean` – remove build artifacts
- `make stop-all` – delete managed workloads via the Server API

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
 
 
- [x] Create dashboard server to run scripts and report status
- [x] Create UI for dashboard server to join/create network, manage clusters and observe code servers
- [ ] Ensure multi-user support with orgs/clusters
- [ ] Automatic TLS certs for tailnet access
- [ ] Fully generic and configurable docker deploys via subdomain on tailnet
 
- [ ] Run Ollama on host machine and OpenAI Codex inside code servers, opening terminal to interact with agent via web UI
- [ ] Event bus for agent-host communication (e.g. notify users of PR created, code pushed, etc) with web UI
- [ ] Add persistent storage to cluster via Longhorn, save code server data there
- [ ] Add Radicle for git hosting inside cluster, hook up to agent workflow for PR creation
- [ ] Create UI for code review and PR management
- [ ] Prompt engineering for agent workflows, provide templates and examples
- [ ] Add MCPs for agent integration/interaction/memory/thinking etc
- [ ] Add Obsidian or similar for personal and collective knowledge management inside cluster
- [ ] Add task management system inside cluster, hook up to agent workflows, with 2D/3D graphical interface
