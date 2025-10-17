#!/bin/bash
# Verify GuildNet installation

echo "Verifying GuildNet installation..."

# Check Host App
if curl -k -s https://localhost:8090/healthz > /dev/null 2>&1; then
    echo "✓ Host App running"
else
    echo "✗ Host App not responding"
    exit 1
fi

# Check API
if curl -k -s https://localhost:8090/api/health | jq . > /dev/null 2>&1; then
    echo "✓ API responding"
else
    echo "✗ API not responding"
    exit 1
fi

# Check CRDs
export KUBECONFIG="$HOME/.guildnet/kubeconfig"
if kubectl get crd workspaces.guildnet.io > /dev/null 2>&1; then
    echo "✓ CRDs installed"
else
    echo "✗ CRDs not found"
    exit 1
fi

echo "✓ GuildNet verification passed"
exit 0

