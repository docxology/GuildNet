#!/bin/bash
# Verify Kubernetes cluster

echo "Verifying Kubernetes cluster..."

export KUBECONFIG="$HOME/.guildnet/kubeconfig"

if [ ! -f "$KUBECONFIG" ]; then
    echo "✗ Kubeconfig not found"
    exit 1
fi

if kubectl version --client > /dev/null 2>&1; then
    echo "✓ kubectl works"
else
    echo "✗ kubectl failed"
    exit 1
fi

if kubectl get nodes > /dev/null 2>&1; then
    echo "✓ Cluster reachable"
    kubectl get nodes
    exit 0
else
    echo "✗ Cluster not reachable"
    exit 1
fi

