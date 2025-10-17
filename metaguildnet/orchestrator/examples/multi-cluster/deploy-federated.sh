#!/bin/bash
# Deploy workspace to federated clusters

set -e

FEDERATION_CONFIG="${1:-federation.yaml}"
WORKSPACE_SPEC="${2:-../../templates/workspace-codeserver.yaml}"

echo "Deploying to federated clusters..."
echo "Federation config: $FEDERATION_CONFIG"
echo "Workspace spec: $WORKSPACE_SPEC"
echo ""

# Extract cluster IDs from federation config
CLUSTERS=$(yq eval '.federation.clusters[].id' "$FEDERATION_CONFIG")

# Extract workspace name
WORKSPACE_NAME=$(yq eval '.workspace.name' "$WORKSPACE_SPEC")
IMAGE=$(yq eval '.workspace.image' "$WORKSPACE_SPEC")

echo "Deploying workspace '$WORKSPACE_NAME' ($IMAGE)"
echo ""

SUCCESSFUL=0
FAILED=0

for cluster in $CLUSTERS; do
    echo "Deploying to $cluster..."
    
    if mgn workspace create "$cluster" \
        --name "$WORKSPACE_NAME" \
        --image "$IMAGE" \
        2>/dev/null; then
        echo "✓ Deployed to $cluster"
        SUCCESSFUL=$((SUCCESSFUL + 1))
    else
        echo "✗ Failed to deploy to $cluster"
        FAILED=$((FAILED + 1))
    fi
    echo ""
done

echo "===================================="
echo "Deployment Summary"
echo "===================================="
echo "Successful: $SUCCESSFUL"
echo "Failed: $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo "✓ All deployments successful"
    
    # Wait for all workspaces to be ready
    echo ""
    echo "Waiting for workspaces to be ready..."
    for cluster in $CLUSTERS; do
        echo "Waiting for $cluster..."
        mgn workspace wait "$cluster" "$WORKSPACE_NAME" --timeout 5m || true
    done
    
    exit 0
else
    echo "✗ Some deployments failed"
    exit 1
fi

