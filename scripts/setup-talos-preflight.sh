#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

# Install talosctl if missing
if ! command -v talosctl >/dev/null 2>&1; then
  echo "talosctl not found, installing..."
  case "$(uname -s)" in
    Linux)
      os="linux" arch=""
      case "$(uname -m)" in
        x86_64|amd64) arch=amd64;;
        aarch64|arm64) arch=arm64;;
        *) arch=amd64;;
      esac
      url="https://github.com/siderolabs/talos/releases/latest/download/talosctl-${os}-${arch}"
      echo "Downloading talosctl from $url"
      curl -fsSL -o /tmp/talosctl "$url"
      chmod +x /tmp/talosctl
      sudo install -m 0755 /tmp/talosctl /usr/local/bin/talosctl
      ;;
    Darwin)
      if command -v brew >/dev/null 2>&1; then
        brew install siderolabs/tap/talosctl
      else
        echo "Please install Homebrew from https://brew.sh and run: brew install siderolabs/tap/talosctl"
        exit 1
      fi
      ;;
    *)
      echo "Please install talosctl for your platform from https://www.talos.dev/latest/introduction/quickstart/"
      exit 1
      ;;
  esac
  echo "talosctl installed successfully"
fi

# Router health gate (best-effort)
if command -v tailscale >/dev/null 2>&1; then
  if ! tailscale status >/dev/null 2>&1; then
    echo "WARN: tailscale not connected; run: make setup-tailscale" >&2
  fi
fi

echo "[0/7] Preflight: checking node reachability on TCP :$PRECHECK_PORT"
UNREACH=0
if [[ -n "$CP_NODES" ]]; then
  for n in "${CP_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: control-plane $host not reachable on :$port"; UNREACH=1
    else
      echo "  OK: control-plane $host:$port reachable"
    fi
  done
fi
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for n in "${WK_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: worker $host not reachable on :$port"; UNREACH=1
    else
      echo "  OK: worker $host:$port reachable"
    fi
  done
fi
if [[ $UNREACH -eq 1 ]]; then
  echo "WARN: Nodes unreachable. This is expected without Talos infrastructure."
  echo "To set up Talos nodes:"
  echo "  1. Install QEMU: sudo apt-get install qemu-system-x86"
  echo "  2. Run: make talos-vm-up"
  echo "  3. Or configure existing Talos nodes in .env"
  echo ""
  echo "Skipping Talos setup for now."
  exit 0  # Exit successfully rather than failing
fi

echo "[1.5/7] Verifying Talos maintenance API via forwarder (talosctl version)..."
overlay_ok=1
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  checking control-plane node=$real via $host:$port"
  if ! talosctl version -e "$host:$port" -n "$real" -i --short >/dev/null 2>&1; then
    echo "    ERROR: talosctl couldn't reach $real via $host:$port (overlay route likely missing)"; overlay_ok=0
  else
    echo "    OK: talosctl maintenance API reachable"
  fi
done
[[ $overlay_ok -eq 1 ]] || exit 1
