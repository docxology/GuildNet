# GuildNet: Linux-only Tailscale Subnet Router + Talos Deploy Guide

This document captures the exact context and the step-by-step procedure to complete the overlay and deploy a working Talos cluster. The subnet router step is Linux-only. macOS is supported for running Headscale and acting as the operator machine, but not for advertising/forwarding the Talos subnets.

## Context snapshot

- Headscale is running in Docker on the macOS host and bound to the LAN. You can show the effective URL via:
  - `scripts/headscale-run.sh status` → look for `Effective URL: http://<LAN-IP>:8082`
- The repository `.env` (on macOS) should include:
  - `TS_LOGIN_SERVER=http://<LAN-IP>:8082` (or the effective URL from the command above)
  - `TS_AUTHKEY=<preauth key>` (issued by Headscale)
  - `TS_ROUTES=10.0.0.0/24,10.96.0.0/12,10.244.0.0/16`
  - `ENDPOINT=https://127.0.0.1:56443` (Kubernetes API forward)
  - `CP_NODES=127.0.0.1:50010`, `WK_NODES=127.0.0.1:50020`
  - `CP_NODES_REAL=10.0.0.10`, `WK_NODES_REAL=10.0.0.20`
- The fresh deploy script `scripts/talos-fresh-deploy.sh` performs an early overlay check:
  - `talosctl version -e 127.0.0.1:50010 -n 10.0.0.10 -i`
  - This must succeed before the deploy proceeds; it requires the 10.0.0.0/24 subnet route to be advertised and usable.

## Requirements

- A Linux host (VM or physical) that can reach the Talos node LAN (defaults use `10.0.0.0/24`).
- Network reachability from that Linux host to the macOS Headscale’s Effective URL (`http://<LAN-IP>:8082`).
- Admin privileges on the Linux host (to enable IP forwarding and run tailscale).

## Step 1: Prepare Headscale and credentials (on macOS)

- Start Headscale and sync .env to the LAN mapping:
  - `make headscale-up`
  - `make headscale-status` → note the `Effective URL` (likely `http://<LAN-IP>:8082`)
  - `make env-sync-lan` → updates `.env` with the effective URL
- Bootstrap a preauth key and write it to `.env`:
  - `make headscale-bootstrap` → creates a user, issues a key, sets `TS_AUTHKEY`

## Step 2: Configure the Linux subnet router (Linux-only)

- Transfer the following values from your macOS `.env` to the Linux host environment:
  - `TS_LOGIN_SERVER=http://<mac-lan-ip>:8082`
  - `TS_AUTHKEY=<value from .env>`
  - `TS_ROUTES=10.0.0.0/24,10.96.0.0/12,10.244.0.0/16`

- Install and start tailscale (Debian/Ubuntu example):

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo systemctl enable --now tailscaled
```

- Enable IP forwarding:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
sudo sysctl -w net.ipv6.conf.all.forwarding=1
# Persist it:
printf "net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1\n" | sudo tee /etc/sysctl.d/99-tailscale-forwarding.conf
sudo sysctl --system
```

- Bring the router up non-interactively (uses preauth key):

```bash
sudo tailscale up \
  --login-server="$TS_LOGIN_SERVER" \
  --authkey="$TS_AUTHKEY" \
  --advertise-routes="$TS_ROUTES" \
  --hostname="guildnet-router" \
  --accept-routes \
  --accept-dns=false
```

- Verify the router:

```bash
tailscale status
tailscale ip -4
```

## Step 3: Approve routes in Headscale (one-time)

From the macOS machine hosting the Headscale container:

```bash
docker exec -it guildnet-headscale headscale routes list
# Find the router machine and enable routes for it:
docker exec -it guildnet-headscale headscale routes enable -r <machine-name-or-id>
# Verify
Docker exec -it guildnet-headscale headscale routes list
```

Once enabled, `tailscale status` on Linux should reflect the subnets advertised for the router device.

## Step 4: Deploy the Talos cluster (from macOS)

- Quick check (should pass now):

```bash
talosctl version -e 127.0.0.1:50010 -n 10.0.0.10 -i
```

- Fresh deploy:

```bash
make talos-fresh
```

The script will:
- Confirm TCP reachability via the local forwards
- Confirm Talos maintenance API reachability via the overlay
- Generate and apply control-plane/worker configs via forwarder+real IP mapping
- Bootstrap etcd (idempotent)
- Fetch kubeconfig

- Verify Kubernetes:

```bash
kubectl get nodes -o wide
kubectl -n kube-system get pods -o wide
```

## Troubleshooting

- Control URL mismatch or unreachable:
  - Ensure TS_LOGIN_SERVER on Linux matches the Headscale `Effective URL` (usually `http://<LAN-IP>:8082`).
  - From Linux, test reachability: `curl -sS http://<LAN-IP>:8082`.
- Routes not active:
  - Confirm tailscale status on Linux shows the router online.
  - Ensure you approved routes in Headscale: `headscale routes enable -r <machine>`.
- Forwarder mapping differences:
  - Defaults assume: `127.0.0.1:50010→10.0.0.10:50000`, `127.0.0.1:50020→10.0.0.20:50000`, `127.0.0.1:56443→10.0.0.10:6443`.
  - If you changed these, adjust `.env` variables accordingly before running `make talos-fresh`.

## macOS deprecation notes (subnet router)

- Do not attempt to run the subnet router on macOS. The native client requires interactive permissions, and tsnet on macOS operates in userspace with a no-op router.
- Action items:
  - Update `scripts/tailscale-router.sh` to print a clear error and exit on Darwin for `up/down` commands.
  - Add a runtime warning in `cmd/tsnet-subnet-router/main.go` if GOOS=darwin, explaining that subnet routing must run on Linux.
  - Update the Makefile target descriptions (`router-up`, `local-overlay-up`) to specify “Linux only for subnet routing.”
  - README: Reflect Linux-only requirement for subnet routing and link to this guide.

## Summary

- Headscale runs on macOS and is reachable at the `Effective URL` (e.g., `http://<LAN-IP>:8082`).
- A Linux host on the Talos LAN acts as the subnet router, using a preauth key from Headscale to non-interactively `tailscale up` and advertise `10.0.0.0/24` (and related service subnets).
- With the overlay up, the `talos-fresh-deploy.sh` flow completes end-to-end using the localhost forwarder.
