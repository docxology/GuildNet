#!/usr/bin/env bash
set -euo pipefail
cat <<'MSG'
[DEPRECATED] scripts/tsnet-router.sh is no longer used.
Use the native Tailscale router instead:
  - make setup-tailscale   # full router setup (forwarding + up + route approve)
  - make router-up         # bring up router (after .env is set)
  - make router-status
Underlying script: scripts/tailscale-router.sh
MSG
exit 2
