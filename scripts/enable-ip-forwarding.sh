#!/usr/bin/env bash
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1; }

# Check current settings
v4=$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo 0)
v6=$(sysctl -n net.ipv6.conf.all.forwarding 2>/dev/null || echo 0)

if [ "$v4" = "1" ] && [ "$v6" = "1" ]; then
  echo "IP forwarding already enabled (IPv4 and IPv6)."
  exit 0
fi

echo "Enabling IP forwarding (requires sudo)..."
if ! need sudo; then
  echo "sudo not found; please enable forwarding manually:"
  echo "  sysctl -w net.ipv4.ip_forward=1"
  echo "  sysctl -w net.ipv6.conf.all.forwarding=1"
  exit 1
fi

sudo sysctl -w net.ipv4.ip_forward=1
sudo sysctl -w net.ipv6.conf.all.forwarding=1
printf "net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1\n" | sudo tee /etc/sysctl.d/99-tailscale-forwarding.conf >/dev/null
sudo sysctl --system >/dev/null || true

echo "IP forwarding enabled."