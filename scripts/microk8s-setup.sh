#!/usr/bin/env bash
set -euo pipefail

# microk8s-setup.sh
# Start microk8s (if not running), enable community addons useful for GuildNet,
# and write a kubeconfig that other machines can use (optionally replacing the
# API server host with TAILSCALE_IP or KUBE_API_SERVER_OVERRIDE).

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
LOGFILE=${LOGFILE:-/tmp/microk8s-setup.log}
echolog() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" | tee -a "$LOGFILE"; }


need() { command -v "$1" >/dev/null 2>&1 || return 1; }

# Auto-install microk8s via snap if not present (requires sudo)
if ! need microk8s; then
  echolog "microk8s not found. Attempting to auto-install via snap (requires sudo)."
  if ! need snap; then
    echolog "snap not found. Attempting to install snapd via package manager."
    if [ -x "$(command -v apt-get)" ]; then
      echolog "Installing snapd via apt-get"
      sudo apt-get update && sudo apt-get install -y snapd
    else
      echolog "Unsupported OS for auto-install. Please install snapd and microk8s manually."; exit 2
    fi
  fi
  echolog "Installing microk8s via snap (this requires network and sudo)"
  sudo snap install microk8s --classic || { echolog "snap install microk8s failed"; exit 2; }
  echolog "microk8s installed via snap"
fi

echolog "Ensuring microk8s is running (may require sudo)"
try_status() {
  # Run microk8s status, preferring sudo (some installs require it)
  if sudo microk8s status --wait-ready >/dev/null 2>&1; then
    return 0
  fi
  if microk8s status --wait-ready >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

if ! try_status; then
  echolog "Starting microk8s (may require sudo)"
  sudo microk8s start 2>&1 | tee -a "$LOGFILE" || echolog "microk8s start returned non-zero"
  echolog "Waiting for microk8s to become ready"
  # If status fails due to permissions, attempt to fix permissions and retry
  OUT=$(sudo microk8s status --wait-ready 2>&1 || true)
  if echo "$OUT" | grep -qi "Insufficient permissions to access MicroK8s"; then
    echolog "Detected MicroK8s permission issue; attempting auto-fix: adding $USER to microk8s group and chowning ~/.kube"
    if sudo usermod -a -G microk8s "$USER"; then
      echolog "Added $USER to microk8s group"
    else
      echolog "Failed to add $USER to microk8s group"
    fi
    if sudo test -d "$HOME/.kube"; then
      sudo chown -R "$USER" "$HOME/.kube" || echolog "chown ~/.kube failed"
    fi
    echolog "Retrying microk8s status after applying permission fixes"
    # Give groups a moment; newgrp won't affect this non-interactive shell, but sudo commands will work
    OUT2=$(sudo microk8s status --wait-ready 2>&1 || true)
    echolog "$OUT2"
  else
    echolog "$OUT"
  fi
fi

echolog "Enabling recommended addons: dns, storage"
sudo microk8s enable dns storage 2>&1 | tee -a "$LOGFILE" || echolog "microk8s enable returned non-zero"

if command -v kubectl >/dev/null 2>&1; then
  echolog "microk8s kubectl available"
fi

# Compose kubeconfig path to emit
OUT_KUBECONFIG=${1:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}
mkdir -p "$(dirname "$OUT_KUBECONFIG")"

echolog "Writing microk8s kubeconfig to $OUT_KUBECONFIG"
# Use sudo to read microk8s config when running as non-microk8s user
sudo microk8s config > "$OUT_KUBECONFIG" || microk8s config > "$OUT_KUBECONFIG" || echolog "Failed to write kubeconfig from microk8s"

# Optionally replace server host with TAILSCALE_IP or KUBE_API_SERVER_OVERRIDE so other machines can reach the API
  if [ -n "${KUBE_API_SERVER_OVERRIDE:-}" ] || [ -n "${TAILSCALE_IP:-}" ]; then
  HOST=${KUBE_API_SERVER_OVERRIDE:-${TAILSCALE_IP}}
  # If HOST looks like an IP, set port 16443 for microk8s
  if [[ "$HOST" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    NEW_SERVER="https://$HOST:16443"
  else
    NEW_SERVER="$HOST"
  fi
  echolog "Replacing API server URL in kubeconfig with $NEW_SERVER"
  # Use yq if available, otherwise sed replace server line
  if command -v yq >/dev/null 2>&1; then
    yq eval ".clusters[0].cluster.server = \"$NEW_SERVER\"" -i "$OUT_KUBECONFIG"
  else
    sed -i -E "s#(server:).*#\1 $NEW_SERVER#g" "$OUT_KUBECONFIG" || true
  fi
fi

echolog "microk8s setup complete; kubeconfig at $OUT_KUBECONFIG"
echo "$OUT_KUBECONFIG"
