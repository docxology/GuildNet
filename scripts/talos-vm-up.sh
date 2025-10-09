#!/usr/bin/env bash
set -euo pipefail

# GuildNet Talos VM bootstrapper (macOS/Linux)
# - Creates a local single-node Talos Kubernetes cluster via talosctl (QEMU backend)
# - Ensures kubectl context and kubeconfig are set
# - Deploys a Tailscale subnet router DaemonSet to advertise cluster CIDRs to your tailnet
#
# Requirements:
# - talosctl, kubectl, qemu-system-x86_64 (Linux) or QEMU via brew (macOS)
# - Environment: TS_AUTHKEY (required), optional TS_LOGIN_SERVER, TS_ROUTES
#   Defaults: TS_LOGIN_SERVER=https://login.tailscale.com, TS_ROUTES="10.96.0.0/12,10.244.0.0/16"
# - For Headscale, set TS_LOGIN_SERVER to your Headscale URL and use a pre-auth key

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# Load shared env from repo root if available (supports Headscale)
if [ -f "$ROOT/.env" ]; then
  # shellcheck disable=SC1090
  . "$ROOT/.env"
fi

CLUSTER_NAME=${CLUSTER_NAME:-guildnet}
NAMESPACE=${NAMESPACE:-default}

# Support Headscale aliases
if [ -n "${HEADSCALE_URL:-}" ] && [ -z "${TS_LOGIN_SERVER:-}" ]; then
  TS_LOGIN_SERVER="$HEADSCALE_URL"
fi
if [ -n "${HEADSCALE_AUTHKEY:-}" ] && [ -z "${TS_AUTHKEY:-}" ]; then
  TS_AUTHKEY="$HEADSCALE_AUTHKEY"
fi

TS_AUTHKEY=${TS_AUTHKEY:-}
TS_LOGIN_SERVER=${TS_LOGIN_SERVER:-https://login.tailscale.com}
TS_ROUTES=${TS_ROUTES:-10.96.0.0/12,10.244.0.0/16}
TS_HOSTNAME=${TS_HOSTNAME:-}

log() { printf "%s | %s\n" "$(date -Iseconds)" "$*"; }
err() { printf "ERR: %s\n" "$*" >&2; }

need() {
  command -v "$1" >/dev/null 2>&1
}

install_brew_pkg() {
  local pkg="$1"
  if brew list --formula "$pkg" >/dev/null 2>&1; then
    log "brew: $pkg already installed"
  else
    log "brew: installing $pkg"
    brew install "$pkg"
  fi
}

install_talosctl_macos() {
  if need talosctl; then log "talosctl present"; return; fi
  install_brew_pkg siderolabs/tap/talosctl
}

install_kubectl_macos() {
  if need kubectl; then log "kubectl present"; return; fi
  install_brew_pkg kubernetes-cli
}

install_qemu_macos() {
  if need qemu-system-x86_64 || need qemu-system-aarch64; then log "QEMU present"; return; fi
  install_brew_pkg qemu
}

install_linux_pkg() {
  local pkg="$1"
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update -y
    sudo apt-get install -y "$pkg"
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    sudo dnf install -y "$pkg"
    return
  fi
  if command -v pacman >/dev/null 2>&1; then
    sudo pacman -Sy --noconfirm "$pkg"
    return
  fi
  if command -v zypper >/dev/null 2>&1; then
    sudo zypper install -y "$pkg"
    return
  fi
  err "No supported package manager found to install $pkg"
  return 1
}

install_qemu_linux() {
  if need qemu-system-x86_64; then log "qemu-system-x86_64 present"; return; fi
  install_linux_pkg qemu-system-x86 || install_linux_pkg qemu-system-x86_64 || true
}

install_kubectl_linux() {
  if need kubectl; then log "kubectl present"; return; fi
  # Try package manager first
  if command -v apt-get >/dev/null 2>&1; then
    # Use Kubernetes apt repo for a recent kubectl
    sudo apt-get update -y
    sudo apt-get install -y apt-transport-https ca-certificates curl gnupg
    sudo mkdir -p /etc/apt/keyrings
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.30/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.30/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
    sudo apt-get update -y
    sudo apt-get install -y kubectl
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    sudo dnf install -y kubectl || true
    if need kubectl; then return; fi
  fi
  # Fallback: download binary
  local os="linux" arch
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64;;
    aarch64|arm64) arch=arm64;;
    *) arch=amd64;;
  esac
  local ver="v1.30.0"
  log "Downloading kubectl $ver ($os/$arch)"
  curl -fsSL -o "/tmp/kubectl" "https://dl.k8s.io/release/${ver}/bin/${os}/${arch}/kubectl"
  sudo install -m 0755 /tmp/kubectl /usr/local/bin/kubectl
}

install_talosctl_linux() {
  if need talosctl; then log "talosctl present"; return; fi
  local os="linux" arch
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64;;
    aarch64|arm64) arch=arm64;;
    *) arch=amd64;;
  esac
  local url="https://github.com/siderolabs/talos/releases/latest/download/talosctl-${os}-${arch}"
  log "Downloading talosctl from $url"
  curl -fsSL -o /tmp/talosctl "$url"
  chmod +x /tmp/talosctl
  sudo install -m 0755 /tmp/talosctl /usr/local/bin/talosctl
}

install_hint() {
  case "$(uname -s)" in
    Darwin)
      echo "brew install siderolabs/tap/talosctl kubernetes-cli qemu";;
    Linux)
      echo "# Example (Debian/Ubuntu):\nsudo apt-get update && sudo apt-get install -y qemu-system-x86 kubectl\n# talosctl: https://www.talos.dev/latest/introduction/quickstart/#installing-talosctl";;
    *) echo "Please install talosctl, kubectl, and QEMU for your platform.";;
  esac
}

main() {
  if [[ -z "$TS_AUTHKEY" ]]; then
    err "TS_AUTHKEY is required (Tailscale/Headscale pre-auth key)."
    err "Hint: create $ROOT/.env with values, e.g.:"
    err "  TS_LOGIN_SERVER=https://headscale.example.com"
    err "  TS_AUTHKEY=tskey-..."
    err "  TS_ROUTES=10.96.0.0/12,10.244.0.0/16"
    exit 1
  fi

  case "$(uname -s)" in
    Darwin)
      if ! need brew; then err "Homebrew is required on macOS. Install from https://brew.sh"; exit 1; fi
      install_talosctl_macos
      install_kubectl_macos
      install_qemu_macos
      # Use docker by default on macOS to avoid requiring sudo for QEMU
      TALOS_PROVISIONER=${TALOS_PROVISIONER:-docker}
      ;;
    Linux)
      install_talosctl_linux
      install_kubectl_linux
      install_qemu_linux
  TALOS_PROVISIONER=${TALOS_PROVISIONER:-docker}
      ;;
    *)
      err "Unsupported OS: $(uname -s)"; exit 1
      ;;
  esac

  TALOS_CIDR=${TALOS_CIDR:-10.55.0.0/24}
  # Default hostname if not provided
  if [ -z "${TS_HOSTNAME:-}" ]; then
    TS_HOSTNAME="guildnet-host-$(hostname | tr 'A-Z' 'a-z' | cut -c1-12)"
  fi

  # Reuse detection: if context exists and control plane responds, skip create
  if kubectl config get-contexts -o name 2>/dev/null | grep -q "admin@${CLUSTER_NAME}"; then
    # Try each existing suffix (latest first)
    ctx=$(kubectl config get-contexts -o name | grep "admin@${CLUSTER_NAME}" | tail -n 1)
    if [ -n "$ctx" ]; then
      kubectl config use-context "$ctx" >/dev/null 2>&1 || true
      if kubectl get nodes >/dev/null 2>&1; then
        log "Cluster '$CLUSTER_NAME' appears up (context=$ctx); skipping create"
        reuse=1
      fi
    fi
  fi
  reuse=${reuse:-0}
  if [ $reuse -ne 1 ]; then
    log "Creating Talos cluster: $CLUSTER_NAME (provisioner=${TALOS_PROVISIONER:-auto}, cidr=${TALOS_CIDR})"
    set +e
    talosctl cluster create --name "$CLUSTER_NAME" --workers 0 --wait ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"} --cidr "$TALOS_CIDR"
    rc=$?
    set -e
    if [ $rc -ne 0 ]; then
      err "cluster create failed (rc=$rc). Destroying and retrying with alternate CIDR"
      set +e
      talosctl cluster destroy --name "$CLUSTER_NAME" ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"}
      set -e
      ALT_CIDR=${ALT_CIDR:-10.66.0.0/24}
      log "Recreating cluster with alternate CIDR ${ALT_CIDR}"
      talosctl cluster create --name "$CLUSTER_NAME" --workers 0 --wait ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"} --cidr "$ALT_CIDR"
    fi
    # After creation, capture newest context
    ctx=$(kubectl config get-contexts -o name | grep "admin@${CLUSTER_NAME}" | tail -n 1 || true)
    if [ -n "$ctx" ]; then
      kubectl config use-context "$ctx" >/dev/null 2>&1 || true
      log "Selected kube context: $ctx"
    fi
    # Force export of KUBECONFIG path for subshells (best effort)
    if [ -z "${KUBECONFIG:-}" ]; then
      if [ -f "$HOME/.kube/config" ]; then
        export KUBECONFIG="$HOME/.kube/config"
      fi
    fi
  fi

  log "Fetching kubeconfig"
  talosctl kubeconfig --name "$CLUSTER_NAME" >/dev/null 2>&1 || true

  log "Checking cluster health (kube API readiness)"
  total_wait=0; max_wait=300; interval=5
  until kubectl version --short >/dev/null 2>&1; do
    sleep $interval
    total_wait=$((total_wait+interval))
    if [ $total_wait -ge $max_wait ]; then
      err "kube API not ready after ${max_wait}s; continuing (will still try DS apply)"
      break
    fi
  done

  # Reconcile (self-heal) subnet router DS each run
  if ! kubectl get nodes >/dev/null 2>&1; then
    err "kube API still unreachable; aborting before Tailscale DS apply"
    return 1
  fi
  log "Reconciling Tailscale subnet router (routes=$TS_ROUTES, login=$TS_LOGIN_SERVER)"
  kubectl -n kube-system apply -f - <<YAML
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: tailscale-subnet-router
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
          value: "$TS_AUTHKEY"
        - name: TS_LOGIN_SERVER
          value: "$TS_LOGIN_SERVER"
        - name: TS_ROUTES
          value: "$TS_ROUTES"
        - name: TS_HOSTNAME
          value: "$TS_HOSTNAME"
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
          HOSTNAME_ARG="--hostname=${TS_HOSTNAME:-talos-subnet-router-$(hostname)}"
          tailscale up --authkey="$TS_AUTHKEY" --login-server="$TS_LOGIN_SERVER" --advertise-routes="$TS_ROUTES" $HOSTNAME_ARG --accept-routes
          # keep foreground to hold the pod
          tail -f /dev/null
      volumes:
      - name: state
        emptyDir: {}
      - name: tun
        hostPath:
          path: /dev/net/tun
          type: CharDevice
YAML

  log "Waiting for Tailscale router to be ready (rollout status)"
  kubectl -n kube-system rollout status ds/tailscale-subnet-router --timeout=240s || true
  # Pod self-check
  if ! kubectl -n kube-system get pods -l app=tailscale-subnet-router >/dev/null 2>&1; then
    err "Tailscale subnet router pods not found; investigate manually (kubectl -n kube-system get pods)."
  fi
  log "Talos cluster ready; Tailscale DS applied (check logs for auth success)."
  log "Kubeconfig: \${KUBECONFIG:-\"~/.kube/config\"}; Namespace: $NAMESPACE; Cluster: $CLUSTER_NAME"
}

main "$@"
