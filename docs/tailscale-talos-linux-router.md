# Copilot Prompt: Fresh End-to-End Deploy on Linux (Headscale + Tailscale Subnet Router + Talos)

Run this on a Linux host that can reach the Talos node LAN (defaults: 10.0.0.0/24). This flow redeploys EVERYTHING from scratch, including Headscale state.

Important: Subnet routing is Linux-only. Do not attempt to run the router on macOS. This prompt will handle all steps locally on this Linux machine.

## Goal

Stand up Headscale (Docker), issue a preauth key, bring up a Tailscale subnet router that advertises 10.0.0.0/24, then run the Talos fresh-deploy flow and verify Kubernetes is up.

## Assumptions

- This repo is already cloned at `~/GuildNet` (adjust paths if different).
- You have sudo privileges.
- You’re okay with nuking any existing Headscale/Tailscale state on this host.

## Plan for Copilot (execute step-by-step)

1) Install prerequisites if missing (Docker, curl, git, jq, talosctl, kubectl, tailscale):

```bash
set -euo pipefail
sudo apt-get update -y || true
sudo apt-get install -y docker.io curl git jq || true

# Install talosctl (latest stable)
if ! command -v talosctl >/dev/null 2>&1; then
  curl -fsSL https://talos.dev/install | sh
fi

# Install kubectl (stable)
if ! command -v kubectl >/dev/null 2>&1; then
  curl -fsSL https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl -o /tmp/kubectl
  chmod +x /tmp/kubectl && sudo mv /tmp/kubectl /usr/local/bin/kubectl
fi

# Install tailscale
if ! command -v tailscale >/dev/null 2>&1; then
  curl -fsSL https://tailscale.com/install.sh | sh
fi
sudo systemctl enable --now tailscaled || true
```

2) Enter the repo and reset to a clean slate:

```bash
cd ~/GuildNet
# Stop any managed workloads and clean prior artifacts
make router-down || true
make headscale-down || true
make clean || true

# Remove prior Headscale state and tailscale local state on this host (DANGEROUS)
rm -rf "$HOME/.guildnet/headscale" || true
sudo tailscale down || true
sudo tailscale logout || true
sudo systemctl restart tailscaled || true
```

3) Prepare .env fresh with sane defaults for a single-CP, single-Worker Talos lab:

```bash
if [ ! -f .env ]; then cp .env.example .env; fi
sed -i 's#^TS_ROUTES=.*#TS_ROUTES=10.0.0.0/24,10.96.0.0/12,10.244.0.0/16#' .env
sed -i 's#^ENDPOINT=.*#ENDPOINT=https://127.0.0.1:56443#' .env
sed -i 's#^CP_NODES=.*#CP_NODES=127.0.0.1:50010#' .env
sed -i 's#^WK_NODES=.*#WK_NODES=127.0.0.1:50020#' .env
sed -i 's#^CP_NODES_REAL=.*#CP_NODES_REAL=10.0.0.10#' .env
sed -i 's#^WK_NODES_REAL=.*#WK_NODES_REAL=10.0.0.20#' .env
```

4) Start Headscale on this Linux host, bind to LAN, and sync .env:

```bash
make headscale-up
make headscale-status
make env-sync-lan
```

5) Bootstrap Headscale (create user + preauth key) and write it into .env:

```bash
make headscale-bootstrap
head -n 8 .env
```

6) Enable IP forwarding and bring up the Linux subnet router (advertising 10.0.0.0/24):

```bash
sudo sysctl -w net.ipv4.ip_forward=1
sudo sysctl -w net.ipv6.conf.all.forwarding=1
printf "net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1\n" | sudo tee /etc/sysctl.d/99-tailscale-forwarding.conf >/dev/null
sudo sysctl --system

make router-install
make router-up
make router-status || true
```

7) Approve the advertised routes in Headscale (one-time per router):

```bash
docker exec -it guildnet-headscale headscale routes list || true
# Attempt to auto-enable for a host named "guildnet-router"; adjust if different
RID=$(docker exec -i guildnet-headscale headscale machines list | awk '/guildnet-router/ {print $1; exit}')
if [ -n "$RID" ]; then
  docker exec -i guildnet-headscale headscale routes enable -r "$RID" || true
fi
docker exec -it guildnet-headscale headscale routes list || true
```

8) Verify overlay reachability to Talos maintenance API via local forwards:

```bash
talosctl version -e 127.0.0.1:50010 -n 10.0.0.10 -i || (echo "Overlay check failed" && exit 1)
```

9) Fresh deploy the Talos cluster:

```bash
make talos-fresh || (echo "Talos fresh deploy failed" && exit 1)
```

10) Validate Kubernetes is up:

```bash
kubectl get nodes -o wide || true
kubectl -n kube-system get pods -o wide || true
```

11) Summary & reminders:

- If route approval failed, re-run step 7 after identifying the router machine name.
- If the overlay check (step 8) fails, confirm TS_LOGIN_SERVER in `.env` matches the Headscale Effective URL and that the router is online.
- If you changed node IPs or forwards, update `.env` accordingly before re-running step 9.

## Notes on macOS

Subnet routing must not be attempted on macOS. Keep Headscale and the deploy operator flows here on Linux for this runbook. If your environment involves mixed OS hosts, ensure the subnet router role is assigned to a Linux node.
