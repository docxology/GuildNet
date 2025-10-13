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

# Cluster name
CLUSTER=${CLUSTER:-guildnet}
NAMESPACE=${NAMESPACE:-default}

# Optional: TALOS_DISK (MB) - when set, passed to `talosctl cluster create --disk` (qemu)
TALOS_DISK=${TALOS_DISK:-}
# Optional: TALOS_INSTALL_IMAGE - explicit installer image to pass to talosctl (overrides bundle)
TALOS_INSTALL_IMAGE=${TALOS_INSTALL_IMAGE:-}

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

# If using docker provisioner and login server points to 127.0.0.1, swap to host.docker.internal
if [ "${TALOS_PROVISIONER:-}" = "docker" ]; then
  if printf '%s' "$TS_LOGIN_SERVER" | grep -qE '127.0.0.1'; then
    TS_LOGIN_SERVER=$(printf '%s' "$TS_LOGIN_SERVER" | sed 's#127.0.0.1#host.docker.internal#g')
  fi
fi

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
  if [[ -z "${TS_AUTHKEY}" ]]; then
    log "TS_AUTHKEY not provided; proceeding without Tailscale subnet router (cluster will still be created)."
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

  # Rewrite login server host for container reachability when using docker provisioner
  if [ "${TALOS_PROVISIONER}" = "docker" ] && printf '%s' "$TS_LOGIN_SERVER" | grep -qE '127.0.0.1'; then
    orig=$TS_LOGIN_SERVER
    TS_LOGIN_SERVER=$(printf '%s' "$TS_LOGIN_SERVER" | sed 's#127.0.0.1#host.docker.internal#g')
    log "Rewriting TS_LOGIN_SERVER host from 127.0.0.1 to host.docker.internal for container access (was: $orig now: $TS_LOGIN_SERVER)"
  fi

  TALOS_CIDR=${TALOS_CIDR:-10.55.0.0/24}
  # Default hostname if not provided
  if [ -z "${TS_HOSTNAME:-}" ]; then
    TS_HOSTNAME="guildnet-host-$(hostname | tr 'A-Z' 'a-z' | cut -c1-12)"
  fi

  # Reuse detection: choose highest numeric suffix (admin@CLUSTER-N)
  if kubectl config get-contexts -o name 2>/dev/null | grep -q "admin@${CLUSTER}"; then
    ctx=$(kubectl config get-contexts -o name | grep "admin@${CLUSTER}" | sort -V | tail -n 1)
    if [ -n "$ctx" ]; then
      kubectl config use-context "$ctx" >/dev/null 2>&1 || true
      if kubectl get nodes >/dev/null 2>&1; then
  log "Cluster '$CLUSTER' appears up (context=$ctx); skipping create"
        reuse=1
      else
        log "Context $ctx found but cluster not responding; will attempt create"
      fi
    fi
  fi
  reuse=${reuse:-0}
  if [ $reuse -ne 1 ]; then
  log "Creating Talos cluster: $CLUSTER (provisioner=${TALOS_PROVISIONER:-auto}, cidr=${TALOS_CIDR})"
    set +e
  # If TALOS_DISK is set and we're using the qemu provisioner, include disk size flag
  DISK_FLAG=""
  if [ -n "${TALOS_DISK:-}" ]; then
    if [ "${TALOS_PROVISIONER}" = "qemu" ]; then
      DISK_FLAG="--disk ${TALOS_DISK}"
      log "Using TALOS_DISK=${TALOS_DISK} (MB) -> passing ${DISK_FLAG} to talosctl"
    else
      log "disk flag has been set but has no effect with the ${TALOS_PROVISIONER:-docker} provisioner"
    fi
  fi
  # Optionally pass an explicit installer image
  INSTALL_FLAG=""
  if [ -n "${TALOS_INSTALL_IMAGE:-}" ]; then
    INSTALL_FLAG="--install-image ${TALOS_INSTALL_IMAGE}"
    log "Using TALOS_INSTALL_IMAGE=${TALOS_INSTALL_IMAGE} -> passing ${INSTALL_FLAG} to talosctl"
  fi
  talosctl cluster create --name "$CLUSTER" --workers 0 --wait ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"} ${DISK_FLAG} ${INSTALL_FLAG} --cidr "$TALOS_CIDR"
    rc=$?
    set -e
    if [ $rc -ne 0 ]; then
      err "cluster create failed (rc=$rc). Destroying and retrying with alternate CIDR"
      set +e
  talosctl cluster destroy --name "$CLUSTER" ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"}
      set -e
      ALT_CIDR=${ALT_CIDR:-10.66.0.0/24}
      log "Recreating cluster with alternate CIDR ${ALT_CIDR}"
  talosctl cluster create --name "$CLUSTER" --workers 0 --wait ${TALOS_PROVISIONER:+--provisioner "$TALOS_PROVISIONER"} ${DISK_FLAG} ${INSTALL_FLAG} --cidr "$ALT_CIDR"
    fi
  # After creation, capture newest context (version sort)
  ctx=$(kubectl config get-contexts -o name | grep "admin@${CLUSTER}" | sort -V | tail -n 1 || true)
    if [ -n "$ctx" ]; then
      kubectl config use-context "$ctx" >/dev/null 2>&1 || true
      log "Selected kube context: $ctx"
    fi
    # Force export of KUBECONFIG path for subshells (best effort)
    if [ -z "${KUBECONFIG:-}" ]; then
      export KUBECONFIG="${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
    fi
  fi

  log "Fetching kubeconfig"
  talosctl kubeconfig --name "$CLUSTER" >/dev/null 2>&1 || true
  # Move generated repo-local kubeconfig to user-scoped location
  if [ -s "./kubeconfig" ]; then
    mkdir -p "$(dirname "$KUBECONFIG")"
    mv -f "./kubeconfig" "$KUBECONFIG" || true
  fi

  log "Checking cluster health (kube API readiness)"
  total_wait=0; max_wait=300; interval=5
  while true; do
    if kubectl get nodes >/dev/null 2>&1; then
      # Require at least one Ready node
      if kubectl get nodes -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null | grep -q True; then
        log "kube API reachable and node Ready"
        break
      fi
    fi
    sleep $interval
    total_wait=$((total_wait+interval))
    if [ $total_wait -ge $max_wait ]; then
      err "kube API not ready after ${max_wait}s; continuing (will still try DS apply)"
      break
    fi
  done

  # Optionally reconcile subnet router DS if TS_AUTHKEY provided
  if ! kubectl get nodes >/dev/null 2>&1; then
    err "kube API still unreachable; aborting"
    return 1
  fi
  if [[ -n "${TS_AUTHKEY}" ]]; then
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
        readinessProbe:
          exec:
            command: ["/bin/sh","-c","tailscale status >/dev/null 2>&1"]
          initialDelaySeconds: 5
          periodSeconds: 10
        livenessProbe:
          exec:
            command: ["/bin/sh","-c","tailscale status >/dev/null 2>&1"]
          initialDelaySeconds: 30
          periodSeconds: 30
        args:
        - /bin/sh
        - -c
        - |
          set -e
          /usr/local/bin/tailscaled --state=/var/lib/tailscale/tailscaled.state &
          # Wait up to 60s for tailscaled to answer (static list avoids host shell var expansion)
          for _ in 1 2 3 4 5 6 7 8 9 10 \
            11 12 13 14 15 16 17 18 19 20 \
            21 22 23 24 25 26 27 28 29 30 \
            31 32 33 34 35 36 37 38 39 40 \
            41 42 43 44 45 46 47 48 49 50 \
            51 52 53 54 55 56 57 58 59 60; do
            if tailscale status >/dev/null 2>&1; then break; fi
            sleep 1
          done
          tailscale up --authkey="${TS_AUTHKEY}" --login-server="${TS_LOGIN_SERVER}" --advertise-routes="${TS_ROUTES}" --hostname="${TS_HOSTNAME}" --accept-routes || true
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
  else
    log "Skipping Tailscale subnet router deploy (TS_AUTHKEY not set)."
  fi
  log "Kubeconfig: ${KUBECONFIG:-"$HOME/.guildnet/kubeconfig"}; Namespace: $NAMESPACE; Cluster: $CLUSTER"
  # Ensure RethinkDB service is present and exposed for this local cluster
  if kubectl get nodes >/dev/null 2>&1; then
    if [ -x "$ROOT/scripts/rethinkdb-setup.sh" ]; then
      log "Ensuring RethinkDB is deployed and reachable (local VM cluster)"
      bash "$ROOT/scripts/rethinkdb-setup.sh" || log "WARNING: rethinkdb-setup encountered an issue"
    else
      log "WARNING: rethinkdb-setup.sh not found; skipping DB setup"
    fi
  fi
}

main "$@"
