#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# 1) Enable IP forwarding
bash "$ROOT/scripts/enable-ip-forwarding.sh" || true

# 2) Ensure tailscaled is running and grant operator (prompts once)
sudo systemctl enable --now tailscaled || sudo service tailscaled start || true
sudo tailscale set --operator="$USER" || true

echo "Bootstrap-sudo complete. You can now run: make setup-all-provision"
