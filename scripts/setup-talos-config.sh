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

mkdir -p "$OUT_DIR"
FORCE=${FORCE:-0}

echo "[1/7] Generating cluster config..."
if [[ ${FORCE} -eq 1 ]]; then
  echo "  --force specified: regenerating config into $OUT_DIR" >&2
  talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR" --force
else
  if [[ -f "$OUT_DIR/controlplane.yaml" ]]; then
    echo "  existing config detected (use FORCE=1 to regenerate); skipping generation"
  else
    talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR"
  fi
fi
