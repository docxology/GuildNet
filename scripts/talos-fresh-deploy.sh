#!/usr/bin/env bash
set -euo pipefail
cat <<'MSG'
[DEPRECATED] scripts/talos-fresh-deploy.sh has been replaced.
Use the modular Talos setup instead:
  - make setup-talos            # full orchestrated flow
  - make setup-talos-preflight  # reachability + overlay checks
  - make setup-talos-config     # talosctl gen config
  - make setup-talos-apply      # reset/apply/bootstrap
  - make setup-talos-wait-kube  # fetch kubeconfig and wait for API/nodes
Kubeconfig is written to ~/.guildnet/kubeconfig.
MSG
exit 2
