#!/usr/bin/env bash
set -euo pipefail

# Simple launcher for GuildNet HostApp (no supervision, no DB lock handling)
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="${ROOT_DIR}/bin/hostapp"

# Prefer GuildNet kubeconfig for hostapp if present
if [ -f "$HOME/.guildnet/kubeconfig" ]; then
  export KUBECONFIG="${KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
fi

exec "$BIN" serve
