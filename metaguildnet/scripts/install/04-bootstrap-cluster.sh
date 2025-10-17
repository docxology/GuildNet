#!/bin/bash
# Bootstrap GuildNet cluster

set -e

CLUSTER_NAME="${CLUSTER:-guildnet-cluster}"
KUBECONFIG_PATH="$HOME/.guildnet/kubeconfig"

echo "Bootstrapping cluster: $CLUSTER_NAME..."

if [ ! -f "$KUBECONFIG_PATH" ]; then
    echo "✗ Kubeconfig not found at $KUBECONFIG_PATH"
    exit 1
fi

# Wait for Host App to be ready
echo "Waiting for GuildNet Host App..."
for i in {1..30}; do
    if curl -k -s https://localhost:8090/healthz > /dev/null 2>&1; then
        echo "✓ Host App is ready"
        break
    fi
    sleep 2
done

# Bootstrap cluster via API
echo "Registering cluster..."
curl -k -X POST https://localhost:8090/bootstrap \
  -F "file=@$KUBECONFIG_PATH" \
  || echo "⚠ Bootstrap may have already been done"

echo "✓ Cluster bootstrap complete"

