# GuildNet: Migrate from Talos to Plain Kubernetes (Real Nodes)

This document defines the requirements, target architecture, and a step-by-step plan to migrate GuildNet off Talos and onto plain Kubernetes clusters provisioned with kubeadm or an existing K8s environment. It also establishes multi-cluster connectivity where each cluster lives in its own tailnet (Headscale namespace) and the Host App can connect to many clusters concurrently from a separate device.

Any new features or configurations must work by default, using the provided Makefile and scripts. No manual steps should be required for a simple setup.

## Goals
- Host App runs on a device separate from the Kubernetes cluster(s).
- Support multiple clusters concurrently.
- Each cluster uses its own tailnet (Headscale namespace isolation).
- No global route hijacks on the Host App machine; per-cluster connections are contained.
- One-command setup for a simple path (out-of-the-box defaults) with verification.

## System Requirements

### Clusters (per cluster)
- Kubernetes provisioned with kubeadm (or equivalent).
- CNI installed (Calico/Cilium/Flannel) with known Pod/Service CIDRs.
- /dev/net/tun available on nodes.
- Default StorageClass present.

### Headscale / Tailscale
- One Headscale server reachable by clusters and Host App devices.
- One Headscale namespace (tailnet) per cluster.
- Preauth keys:
  - Router key (used by in-cluster subnet-router DS), per cluster.
  - Client key (used by Host App per-cluster connector), per cluster.

### Host App Device
- Linux, Go 1.22+, Node.js 18+, Docker.
- kubectl.
- Local TLS certs via `make regen-certs` (127.0.0.1:8090).

## Target Architecture

- Headscale namespaces: cluster-alpha, cluster-beta, ...
- In-cluster Tailscale subnet-router DaemonSet:
  - Joins cluster’s namespace with router preauth key.
  - Advertises Pod/Service CIDRs.
  - Privileged, mounts /dev/net/tun.
- Host App per-cluster connector using embedded tsnet:
  - One tsnet instance per cluster with isolated state under `~/.guildnet/tsnet/cluster-<id>`.
  - Create a per-cluster net.Dialer bound to tsnet; use it for kube client HTTP transport.
  - No global host routes, no single shared tailscale daemon.
- Persistence per cluster: kubeconfig, ts login server, client auth key, routes (for diagnostics).

## Migration Plan (Talos → Plain K8S)

1) Provision Clusters (external to repo)
- Use kubeadm or your infra tool to create clusters with explicit Pod/Service CIDRs.
- Install CNI.
- Ensure StorageClass exists.

2) Headscale Multi-Namespace Setup (repo automation)
- New script: `scripts/headscale-namespace-and-keys.sh`
  - Create Headscale namespace for the cluster (id or name).
  - Generate two preauth keys (router and client).
  - Output keys and namespace in a machine-readable format (JSON/yaml) to `tmp/cluster-<id>-headscale.json`.

3) In-Cluster Tailscale Router (repo automation)
- Update `scripts/deploy-tailscale-router.sh` to accept env/flags:
  - `TS_LOGIN_SERVER`, `TS_AUTHKEY`, `TS_ROUTES`, `TS_HOSTNAME`.
  - Use provided kubeconfig/current context for the target cluster.
- Ensure DaemonSet uses privileged mode and mounts `/dev/net/tun`.
- Make target `router-ensure` should:
  - Read the cluster’s Headscale info from `tmp/cluster-<id>-headscale.json` or `.env`.
  - Apply the DS and wait for rollout.

4) Host App Per-Cluster Connector (code change)
- New Go package: `internal/ts/connector` (or `internal/connector`):
  - API:
    - `Start(ctx, cfg) error` (cfg includes clusterID, TS login, client auth key, stateDir).
    - `DialContext(ctx, network, addr) (net.Conn, error)` (returns tsnet dialer conn).
    - `Health() (status, details)` returns reachability and peer state.
    - `Stop() error` cleanup.
  - Internals:
    - Manage one `tsnet.Server` per cluster; persist to `~/.guildnet/tsnet/cluster-<id>` with 0700 perms.
    - Join the specified Headscale namespace using the client preauth key.
- Wire kube clients to use per-cluster DialContext:
  - Build `http.Transport` with custom `DialContext` from connector.
  - Build `rest.Config` from stored kubeconfig; override transport.
  - Cache per-cluster clients and recycle on connector state change.

5) Persistence & API (backend)
- Extend cluster model to store:
  - `tsLoginServer`, `tsClientAuthKey` (or secret reference), `tsRoutes`, `tsStatePath`.
  - Optional: Headscale namespace name/id.
- Enforce secrets are protected; avoid echoing them back via APIs.

6) Makefile & Scripts
- Remove Talos from the repository (scripts, helpers, and targets) and default path.
- Add targets:
  - `headscale-namespace`: create namespace and keys for a cluster (runs `scripts/headscale-namespace-and-keys.sh`).
  - `router-ensure`: apply DS for current kube context using env/keys.
  - `verify-e2e`: add per-cluster tsnet dial check and kube /readyz check via connector.
- Keep/addons:
  - `deploy-metallb.sh` and `rethinkdb-setup.sh` stay as-is; ensure `.env` contains a valid L2 pool.

7) Documentation & Defaults
- README updates:
  - Real-nodes quickstart: kubeadm init, CNI install, CIDR alignment with TS_ROUTES.
  - Headscale namespaces: one per cluster; how to create keys via Make targets.
  - Router DS deploy per cluster; approve routes if needed.
  - Host App per-cluster connectivity model (tsnet instances).
- `.env` updates:
  - Add examples for common CNIs (Calico/Cilium/k3s) and their Pod/Service CIDRs.
  - Ensure defaults match the DS and verify scripts.

8) Verification
- `make verify-e2e` should:
  - Check Headscale is reachable and keys/namespace exist.
  - Ensure router DS is Ready.
  - Start (or ping) the Host App per-cluster connector and verify kube `/readyz`.
  - Print a concise health summary per cluster.

## Acceptance Criteria
- Host App can connect to multiple clusters simultaneously, each through its own tsnet connection into its own Headscale namespace.
- No host-wide route changes; per-cluster dials use the right connector.
- One-command flow for a new user (using defaults) brings up Headscale, creates keys, deploys router DS, runs Host App, and passes verify.

---

## Implementation Notes
- Prefer tsnet in-process per cluster to avoid single-tailnet limitations of a global tailscale client.
- Use 0700 perms on `~/.guildnet/tsnet/*` and sanitize logs.
- Keep the existing router DS securityContext (privileged, NET_ADMIN) and /dev/net/tun hostPath.
- Where `.env` contains 127.0.0.1 for a login server, rewrite to LAN IP for pods using `env-sync-lan`.
- Remove Talos scripts from repo entirely; no backwards compatibility required in this prototype.
