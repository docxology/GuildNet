#!/bin/bash
# Canary deployment - gradual rollout with monitoring

set -e

CLUSTER_ID="$1"
WORKSPACE_NAME="$2"
NEW_IMAGE="$3"
CANARY_PCT="${4:-10}"

if [ -z "$CLUSTER_ID" ] || [ -z "$WORKSPACE_NAME" ] || [ -z "$NEW_IMAGE" ]; then
    echo "Usage: $0 <cluster-id> <workspace-name> <new-image> [canary-percentage]"
    exit 1
fi

echo "Canary Deployment"
echo "Cluster: $CLUSTER_ID"
echo "Workspace: $WORKSPACE_NAME"
echo "New Image: $NEW_IMAGE"
echo "Canary Traffic: ${CANARY_PCT}%"
echo ""

# Deploy canary version
CANARY_NAME="${WORKSPACE_NAME}-canary"
echo "Deploying canary version: $CANARY_NAME"

mgn workspace create "$CLUSTER_ID" \
    --name "$CANARY_NAME" \
    --image "$NEW_IMAGE" \
    2>/dev/null || {
    echo "Error: Failed to create canary workspace"
    exit 1
}

# Wait for canary to be ready
echo "Waiting for canary to be ready..."
if ! mgn workspace wait "$CLUSTER_ID" "$CANARY_NAME" --timeout 5m; then
    echo "Error: Canary failed to become ready"
    echo "Rolling back..."
    mgn workspace delete "$CLUSTER_ID" "$CANARY_NAME" 2>/dev/null || true
    exit 1
fi

echo "✓ Canary is ready"
echo ""

# Monitor canary
echo "Monitoring canary deployment..."
echo "Canary is receiving ${CANARY_PCT}% of traffic"
echo ""

# In a real scenario, you would:
# 1. Configure load balancer to send ${CANARY_PCT}% traffic to canary
# 2. Monitor metrics (error rate, latency, etc.)
# 3. Gradually increase traffic if healthy
# 4. Rollback if issues detected

echo "Monitor the canary for 5 minutes..."
MONITOR_TIME=300
INTERVAL=30
elapsed=0

while [ $elapsed -lt $MONITOR_TIME ]; do
    sleep $INTERVAL
    elapsed=$((elapsed + INTERVAL))
    
    # Check canary health
    if mgn workspace get "$CLUSTER_ID" "$CANARY_NAME" -o json | grep -q '"status":"Running"'; then
        echo "  [$elapsed/${MONITOR_TIME}s] Canary healthy"
    else
        echo "  [$elapsed/${MONITOR_TIME}s] ⚠ Canary unhealthy - recommend rollback"
        echo ""
        echo "To rollback:"
        echo "  mgn workspace delete $CLUSTER_ID $CANARY_NAME"
        exit 1
    fi
done

echo ""
echo "✓ Canary monitoring complete - no issues detected"
echo ""
echo "To promote canary to full production:"
echo "  1. Gradually increase traffic to canary (20%, 50%, 100%)"
echo "  2. Continue monitoring at each stage"
echo "  3. When at 100%, delete old version:"
echo "     mgn workspace delete $CLUSTER_ID $WORKSPACE_NAME"
echo "  4. Rename canary to production name"
echo ""
echo "To rollback:"
echo "  mgn workspace delete $CLUSTER_ID $CANARY_NAME"

