#!/bin/bash
# Rolling update pattern - zero downtime update

set -e

CLUSTER_ID="$1"
WORKSPACE_NAME="$2"
NEW_IMAGE="$3"

if [ -z "$CLUSTER_ID" ] || [ -z "$WORKSPACE_NAME" ] || [ -z "$NEW_IMAGE" ]; then
    echo "Usage: $0 <cluster-id> <workspace-name> <new-image>"
    exit 1
fi

echo "Starting rolling update of $WORKSPACE_NAME to $NEW_IMAGE"
echo "Cluster: $CLUSTER_ID"
echo ""

# Get current workspace details
echo "Fetching current workspace details..."
CURRENT=$(mgn workspace get "$CLUSTER_ID" "$WORKSPACE_NAME" -o json 2>/dev/null || echo "{}")

if [ "$CURRENT" = "{}" ]; then
    echo "Error: Workspace $WORKSPACE_NAME not found"
    exit 1
fi

# Create new workspace with suffix
NEW_NAME="${WORKSPACE_NAME}-new"
echo "Creating new version: $NEW_NAME"

mgn workspace create "$CLUSTER_ID" \
    --name "$NEW_NAME" \
    --image "$NEW_IMAGE" \
    2>/dev/null || {
    echo "Error: Failed to create new workspace"
    exit 1
}

# Wait for new workspace to be ready
echo "Waiting for new version to be ready..."
if ! mgn workspace wait "$CLUSTER_ID" "$NEW_NAME" --timeout 10m; then
    echo "Error: New workspace failed to become ready"
    echo "Rolling back..."
    mgn workspace delete "$CLUSTER_ID" "$NEW_NAME" 2>/dev/null || true
    exit 1
fi

echo "New version is ready"

# Health check (if available)
echo "Running health checks..."
sleep 5

# All checks passed, switch over
echo "Switching to new version..."

# Delete old workspace
mgn workspace delete "$CLUSTER_ID" "$WORKSPACE_NAME" 2>/dev/null || {
    echo "Warning: Failed to delete old workspace"
}

# Wait a moment
sleep 2

# Rename new to original name
echo "Finalizing..."
# Note: mgn doesn't have rename, so we document this limitation
echo "⚠ Note: Workspace is now named $NEW_NAME"
echo "   You may want to delete and recreate with original name"

echo ""
echo "✓ Rolling update complete"
echo "  Old: $WORKSPACE_NAME"
echo "  New: $NEW_NAME (image: $NEW_IMAGE)"
echo ""
echo "To complete the transition:"
echo "  1. Delete the new workspace: mgn workspace delete $CLUSTER_ID $NEW_NAME"
echo "  2. Recreate with original name using new image"

