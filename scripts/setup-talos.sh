#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

# 1) Preflight (reachability + overlay)
bash "$ROOT/scripts/setup-talos-preflight.sh"

# 2) Generate configs (respect FORCE=1)
bash "$ROOT/scripts/setup-talos-config.sh"

# 3) Reset/apply/bootstrap
bash "$ROOT/scripts/setup-talos-apply.sh"

# 4) Wait for Kubernetes and fetch kubeconfig
bash "$ROOT/scripts/setup-talos-wait-kube.sh"

echo "Talos setup complete."