#!/bin/bash
# Collect logs from all GuildNet components

set -e

OUTPUT_DIR="${1:-./guildnet-logs-$(date +%Y%m%d-%H%M%S)}"

echo "Collecting GuildNet logs..."
echo "Output directory: $OUTPUT_DIR"
echo ""

mkdir -p "$OUTPUT_DIR"

# Function to safely collect logs
collect_logs() {
    local name="$1"
    local command="$2"
    
    echo "Collecting $name..."
    if eval "$command" > "$OUTPUT_DIR/$name.log" 2>&1; then
        echo "  ✓ $name"
    else
        echo "  ⚠ $name (failed or not available)"
    fi
}

# System information
collect_logs "system-info" "uname -a; cat /etc/os-release"
collect_logs "docker-info" "docker version; docker ps"
collect_logs "k8s-version" "kubectl version"

# Kubernetes cluster logs
echo ""
echo "Collecting Kubernetes logs..."
collect_logs "k8s-nodes" "kubectl get nodes -o wide"
collect_logs "k8s-pods-all" "kubectl get pods --all-namespaces -o wide"
collect_logs "k8s-services" "kubectl get services --all-namespaces"
collect_logs "k8s-events" "kubectl get events --all-namespaces --sort-by='.lastTimestamp'"

# GuildNet Host App logs
echo ""
echo "Collecting GuildNet Host App logs..."
if systemctl is-active --quiet guildnet-hostapp 2>/dev/null; then
    collect_logs "hostapp-systemd" "journalctl -u guildnet-hostapp -n 1000 --no-pager"
fi

if [ -f /var/log/guildnet/hostapp.log ]; then
    collect_logs "hostapp-file" "cat /var/log/guildnet/hostapp.log"
fi

# Try to get from process
collect_logs "hostapp-process" "pgrep -a hostapp"

# Headscale logs
echo ""
echo "Collecting Headscale logs..."
if systemctl is-active --quiet headscale 2>/dev/null; then
    collect_logs "headscale-systemd" "journalctl -u headscale -n 1000 --no-pager"
fi

if [ -f /var/log/headscale/headscale.log ]; then
    collect_logs "headscale-file" "cat /var/log/headscale/headscale.log"
fi

# RethinkDB logs
echo ""
echo "Collecting RethinkDB logs..."
collect_logs "rethinkdb-pods" "kubectl get pods -n default -l app=rethinkdb -o wide"
collect_logs "rethinkdb-logs" "kubectl logs -n default -l app=rethinkdb --all-containers --tail=1000"

# Workspace pods
echo ""
echo "Collecting Workspace logs..."
collect_logs "workspaces-list" "kubectl get workspaces --all-namespaces"
collect_logs "workspace-pods" "kubectl get pods -l guildnet.io/workspace --all-namespaces -o wide"

# Get logs from each workspace pod
workspace_pods=$(kubectl get pods -l guildnet.io/workspace --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{","}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
if [ -n "$workspace_pods" ]; then
    mkdir -p "$OUTPUT_DIR/workspace-pods"
    echo "$workspace_pods" | while IFS=',' read -r ns pod; do
        if [ -n "$ns" ] && [ -n "$pod" ]; then
            kubectl logs -n "$ns" "$pod" --all-containers --tail=500 > "$OUTPUT_DIR/workspace-pods/${ns}_${pod}.log" 2>&1 || true
            echo "  ✓ $ns/$pod"
        fi
    done
fi

# Operator logs
echo ""
echo "Collecting Operator logs..."
collect_logs "operator-pods" "kubectl get pods -n guildnet-system -o wide"
collect_logs "operator-logs" "kubectl logs -n guildnet-system -l app=guildnet-operator --all-containers --tail=1000"

# Network information
echo ""
echo "Collecting Network information..."
collect_logs "tailscale-status" "tailscale status"
collect_logs "ip-routes" "ip route show"
collect_logs "ip-addresses" "ip addr show"
collect_logs "netstat" "netstat -tuln"

# Configuration files
echo ""
echo "Collecting Configuration files..."
mkdir -p "$OUTPUT_DIR/configs"

if [ -f ~/.config/guildnet/config.yaml ]; then
    cp ~/.config/guildnet/config.yaml "$OUTPUT_DIR/configs/" 2>/dev/null || true
    echo "  ✓ guildnet config"
fi

if [ -f /etc/headscale/config.yaml ]; then
    # Redact sensitive information
    grep -v 'private_key\|secret\|password' /etc/headscale/config.yaml > "$OUTPUT_DIR/configs/headscale-config.yaml" 2>/dev/null || true
    echo "  ✓ headscale config (redacted)"
fi

# Database information
echo ""
echo "Collecting Database information..."
collect_logs "rethinkdb-status" "kubectl exec -n default -it \$(kubectl get pods -n default -l app=rethinkdb -o jsonpath='{.items[0].metadata.name}') -- rethinkdb --version"

# Summary
echo ""
echo "Creating summary..."
cat > "$OUTPUT_DIR/SUMMARY.txt" << EOF
GuildNet Log Collection Summary
Generated: $(date)
Hostname: $(hostname)
User: $(whoami)

This directory contains logs and diagnostic information from:
- Kubernetes cluster
- GuildNet Host App
- Headscale
- RethinkDB
- Workspaces
- Network configuration

Review the logs for errors or warnings.
Look for files marked with ⚠ which may indicate unavailable components.

Key files to check first:
- hostapp-systemd.log or hostapp-file.log
- k8s-events.log
- workspace-pods/*.log

EOF

# Compress the logs
echo ""
echo "Compressing logs..."
tar -czf "${OUTPUT_DIR}.tar.gz" "$OUTPUT_DIR" 2>/dev/null || {
    echo "  ⚠ Failed to compress (tar not available or error)"
}

echo ""
echo "✓ Log collection complete"
echo ""
echo "Logs saved to: $OUTPUT_DIR"
if [ -f "${OUTPUT_DIR}.tar.gz" ]; then
    echo "Compressed archive: ${OUTPUT_DIR}.tar.gz"
    echo "Size: $(du -h "${OUTPUT_DIR}.tar.gz" | cut -f1)"
fi
echo ""
echo "You can share these logs for troubleshooting."
echo "Review SUMMARY.txt for an overview."

