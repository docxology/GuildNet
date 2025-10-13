#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# 1) Enable IP forwarding
bash "$ROOT/scripts/enable-ip-forwarding.sh" || true

# 2) Ensure tailscaled is running and grant operator (prompts once)
sudo systemctl enable --now tailscaled || sudo service tailscaled start || true
# Ensure tailscale operator is set (best-effort)
if command -v tailscale >/dev/null 2>&1; then
	sudo tailscale set --operator="$USER" || true
fi

echo "Bootstrap-sudo complete. You can now run: make setup-all-provision"
