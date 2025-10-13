#!/usr/bin/env bash
set -euo pipefail
# Deploy a Tailscale subnet router DaemonSet into the current cluster.
# Requires: kubectl, TS_AUTHKEY; optional TS_LOGIN_SERVER, TS_ROUTES, TS_HOSTNAME.
# Uses hostNetwork and advertises routes for cluster subnets so other tailnet nodes (tsnet hostapp machines)
# can reach the kube‑API and service/pod CIDRs directly.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"

export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need kubectl

if [ -z "${TS_AUTHKEY:-}" ]; then
  echo "WARN: TS_AUTHKEY not set; skipping tailscale subnet router deploy." >&2
  exit 0
fi

TS_LOGIN_SERVER=${TS_LOGIN_SERVER:-https://login.tailscale.com}
# Include control-plane/node LAN, Service CIDR, and Pod CIDR by default
TS_ROUTES=${TS_ROUTES:-10.0.0.0/24,10.96.0.0/12,10.244.0.0/16}
TS_HOSTNAME=${TS_HOSTNAME:-subnet-router}

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: tailscale-subnet-router
  namespace: kube-system
  labels:
    app: tailscale-subnet-router
spec:
  selector:
    matchLabels:
      app: tailscale-subnet-router
  template:
    metadata:
      labels:
        app: tailscale-subnet-router
    spec:
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      tolerations:
      - operator: Exists
      containers:
      - name: tailscale
        image: tailscale/tailscale:stable
        securityContext:
          capabilities:
            add: ["NET_ADMIN", "NET_RAW"]
          privileged: true
        env:
        - name: TS_AUTHKEY
          value: "${TS_AUTHKEY}"
        - name: TS_LOGIN_SERVER
          value: "${TS_LOGIN_SERVER}"
        - name: TS_ROUTES
          value: "${TS_ROUTES}"
        - name: TS_HOSTNAME
          value: "${TS_HOSTNAME}"
        volumeMounts:
        - name: state
          mountPath: /var/lib/tailscale
        - name: tun
          mountPath: /dev/net/tun
        args:
        - /bin/sh
        - -c
        - |
          set -e
          /usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state &
          sleep 2
          tailscale up --authkey="${TS_AUTHKEY}" --login-server="${TS_LOGIN_SERVER}" --advertise-routes="${TS_ROUTES}" --hostname="${TS_HOSTNAME}" --accept-routes
          tail -f /dev/null
      volumes:
      - name: state
        emptyDir: {}
      - name: tun
        hostPath:
          path: /dev/net/tun
          type: CharDevice
YAML

echo "Waiting for tailscale subnet router to be ready..."
if ! kubectl -n kube-system rollout status ds/tailscale-subnet-router --timeout=300s; then
  echo "DaemonSet not ready; showing pod status and recent logs:" >&2
  kubectl -n kube-system get pods -l app=tailscale-subnet-router -o wide || true
  for p in $(kubectl -n kube-system get pods -l app=tailscale-subnet-router -o name 2>/dev/null | sed 's#pod/##'); do
    echo "--- logs: $p (last 50 lines) ---"
    kubectl -n kube-system logs "$p" --tail=50 || true
  done
  exit 1
fi

# Best-effort: show pod status and logs hint
kubectl -n kube-system get pods -l app=tailscale-subnet-router -o wide || true
echo "Hint: kubectl -n kube-system logs -l app=tailscale-subnet-router -f --tail=100"
