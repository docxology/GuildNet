#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need talosctl

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
