#!/usr/bin/env bash
set -euo pipefail

# Adds 127.0.0.10 and 127.0.0.20 as loopback aliases if missing.
# macOS: requires sudo for ifconfig lo0 alias ...
# Linux: tries ip addr add 127.0.0.10/8 dev lo

ALIAS_IPS=(127.0.0.10 127.0.0.20)

os=$(uname -s)
case "$os" in
  Darwin)
    for ip in "${ALIAS_IPS[@]}"; do
      if ifconfig lo0 | grep -q "${ip} "; then
        echo "lo0 already has ${ip}"
      else
        echo "Adding ${ip} to lo0 (requires sudo)"
        sudo ifconfig lo0 alias "${ip}"/32
      fi
    done
    ;;
  Linux)
    for ip in "${ALIAS_IPS[@]}"; do
      if ip addr show lo | grep -q "${ip}/"; then
        echo "lo already has ${ip}"
      else
        echo "Adding ${ip} to lo"
        sudo ip addr add "${ip}"/8 dev lo
      fi
    done
    ;;
  *)
    echo "Unsupported OS: $os" >&2
    exit 2
    ;;
esac

echo "Loopback aliases ensured."
