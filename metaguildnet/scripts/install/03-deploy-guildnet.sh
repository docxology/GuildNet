#!/bin/bash
# Deploy GuildNet components
# Wraps main GuildNet deployment

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo "Deploying GuildNet components..."

export KUBECONFIG="$HOME/.guildnet/kubeconfig"

# Deploy k8s addons (MetalLB, CRDs, DB)
cd "$PROJECT_ROOT"
make deploy-k8s-addons || true

# Deploy operator
make deploy-operator || true

# Build and run Host App
make build
make deploy-hostapp &

echo "✓ GuildNet deployment complete"
echo "  Host App will be available at: https://localhost:8090"

