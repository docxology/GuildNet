#!/bin/bash
# Generate comprehensive debug information bundle

set -e

OUTPUT_DIR="${1:-./guildnet-debug-$(date +%Y%m%d-%H%M%S)}"

echo "Generating GuildNet debug information..."
echo "Output directory: $OUTPUT_DIR"
echo ""

mkdir -p "$OUTPUT_DIR"

# Collect logs first
echo "==> Collecting logs..."
bash "$(dirname "$0")/log-collector.sh" "$OUTPUT_DIR/logs"

# System diagnostics
echo ""
echo "==> System diagnostics..."

cat > "$OUTPUT_DIR/system.txt" << EOF
=== System Information ===
Date: $(date)
Hostname: $(hostname)
OS: $(cat /etc/os-release 2>/dev/null || echo "Unknown")
Kernel: $(uname -r)
Uptime: $(uptime)

=== CPU Information ===
$(lscpu 2>/dev/null || sysctl -a | grep cpu || echo "CPU info not available")

=== Memory Information ===
$(free -h 2>/dev/null || vm_stat || echo "Memory info not available")

=== Disk Information ===
$(df -h)

=== Docker Information ===
$(docker version 2>&1 || echo "Docker not available")
$(docker info 2>&1 || echo "Docker info not available")
$(docker ps -a 2>&1 || echo "Docker ps not available")

EOF

# Kubernetes diagnostics
echo ""
echo "==> Kubernetes diagnostics..."

cat > "$OUTPUT_DIR/kubernetes.txt" << EOF
=== Kubernetes Version ===
$(kubectl version 2>&1 || echo "kubectl not available")

=== Cluster Info ===
$(kubectl cluster-info 2>&1 || echo "Cluster info not available")

=== Nodes ===
$(kubectl get nodes -o wide 2>&1 || echo "Nodes info not available")
$(kubectl describe nodes 2>&1 || echo "Node details not available")

=== Namespaces ===
$(kubectl get namespaces 2>&1 || echo "Namespaces not available")

=== All Pods ===
$(kubectl get pods --all-namespaces -o wide 2>&1 || echo "Pods not available")

=== Services ===
$(kubectl get services --all-namespaces 2>&1 || echo "Services not available")

=== PersistentVolumes ===
$(kubectl get pv 2>&1 || echo "PVs not available")

=== PersistentVolumeClaims ===
$(kubectl get pvc --all-namespaces 2>&1 || echo "PVCs not available")

=== StorageClasses ===
$(kubectl get storageclass 2>&1 || echo "StorageClasses not available")

=== Ingresses ===
$(kubectl get ingress --all-namespaces 2>&1 || echo "Ingresses not available")

=== ConfigMaps ===
$(kubectl get configmaps --all-namespaces 2>&1 || echo "ConfigMaps not available")

=== Secrets (names only) ===
$(kubectl get secrets --all-namespaces 2>&1 || echo "Secrets not available")

=== CRDs ===
$(kubectl get crd 2>&1 || echo "CRDs not available")

=== Events ===
$(kubectl get events --all-namespaces --sort-by='.lastTimestamp' 2>&1 || echo "Events not available")

EOF

# GuildNet specific diagnostics
echo ""
echo "==> GuildNet diagnostics..."

cat > "$OUTPUT_DIR/guildnet.txt" << EOF
=== GuildNet Version ===
$(hostapp --version 2>&1 || echo "GuildNet version not available")

=== GuildNet Process ===
$(pgrep -a hostapp 2>&1 || echo "GuildNet not running")

=== GuildNet Configuration ===
$(cat ~/.config/guildnet/config.yaml 2>&1 || echo "Config not found")

=== Clusters ===
$(mgn cluster list 2>&1 || curl -s http://localhost:8090/api/clusters 2>&1 || echo "Cannot list clusters")

=== Workspaces ===
$(kubectl get workspaces --all-namespaces -o yaml 2>&1 || echo "Workspaces not available")

=== Workspace CRDs ===
$(kubectl get crd workspaces.guildnet.io -o yaml 2>&1 || echo "Workspace CRD not found")

=== Capability CRDs ===
$(kubectl get crd capabilities.guildnet.io -o yaml 2>&1 || echo "Capability CRD not found")

=== RethinkDB Status ===
$(kubectl get pods -n default -l app=rethinkdb 2>&1 || echo "RethinkDB pods not found")

EOF

# Network diagnostics
echo ""
echo "==> Network diagnostics..."

cat > "$OUTPUT_DIR/network.txt" << EOF
=== Network Interfaces ===
$(ip addr show 2>&1 || ifconfig 2>&1 || echo "Network info not available")

=== Routes ===
$(ip route show 2>&1 || netstat -rn 2>&1 || echo "Routes not available")

=== DNS ===
$(cat /etc/resolv.conf 2>&1 || echo "DNS config not available")

=== Listening Ports ===
$(netstat -tuln 2>&1 || ss -tuln 2>&1 || echo "Port info not available")

=== Tailscale Status ===
$(tailscale status 2>&1 || echo "Tailscale not available")

=== Headscale Status ===
$(systemctl status headscale 2>&1 || echo "Headscale not running")

=== Headscale Nodes ===
$(headscale nodes list 2>&1 || echo "Cannot list headscale nodes")

=== Headscale Routes ===
$(headscale routes list 2>&1 || echo "Cannot list headscale routes")

EOF

# Test connectivity
echo ""
echo "==> Testing connectivity..."

cat > "$OUTPUT_DIR/connectivity.txt" << EOF
=== DNS Resolution ===
EOF

for host in localhost github.com google.com; do
    echo "Testing $host..." >> "$OUTPUT_DIR/connectivity.txt"
    nslookup "$host" >> "$OUTPUT_DIR/connectivity.txt" 2>&1 || echo "Failed to resolve $host" >> "$OUTPUT_DIR/connectivity.txt"
    echo "" >> "$OUTPUT_DIR/connectivity.txt"
done

cat >> "$OUTPUT_DIR/connectivity.txt" << EOF

=== HTTP Connectivity ===
EOF

# Test GuildNet API
echo "Testing GuildNet API..." >> "$OUTPUT_DIR/connectivity.txt"
curl -v http://localhost:8090/api/health >> "$OUTPUT_DIR/connectivity.txt" 2>&1 || echo "GuildNet API not reachable" >> "$OUTPUT_DIR/connectivity.txt"

# Create a summary report
echo ""
echo "==> Creating summary report..."

cat > "$OUTPUT_DIR/README.md" << EOF
# GuildNet Debug Information

Generated: $(date)
Hostname: $(hostname)
User: $(whoami)

## Contents

This debug bundle contains:

1. **logs/** - Complete log collection from all components
2. **system.txt** - System information and diagnostics
3. **kubernetes.txt** - Kubernetes cluster state and resources
4. **guildnet.txt** - GuildNet specific information
5. **network.txt** - Network configuration and status
6. **connectivity.txt** - Connectivity test results

## Quick Start

1. Review **logs/SUMMARY.txt** for an overview
2. Check **guildnet.txt** for GuildNet specific issues
3. Look at **kubernetes.txt** for cluster problems
4. Review **connectivity.txt** for network issues

## Common Issues

### GuildNet not responding
- Check **guildnet.txt** for process status
- Review **logs/hostapp-*.log** for errors
- Verify **connectivity.txt** shows API is reachable

### Workspaces not starting
- Check **kubernetes.txt** for pod errors
- Review **logs/workspace-pods/** for individual workspace logs
- Check **logs/k8s-events.log** for scheduling issues

### Network problems
- Review **network.txt** for interface/route issues
- Check **connectivity.txt** for DNS/HTTP problems
- Verify Tailscale/Headscale status in **network.txt**

## Support

Share this bundle when requesting support. Sensitive information like
secrets and keys have been redacted where possible.

Size: $(du -sh "$OUTPUT_DIR" | cut -f1)

EOF

# Compress the bundle
echo ""
echo "==> Compressing debug bundle..."
tar -czf "${OUTPUT_DIR}.tar.gz" "$OUTPUT_DIR" 2>/dev/null || {
    echo "  ⚠ Failed to compress (continuing anyway)"
}

echo ""
echo "✓ Debug information collection complete"
echo ""
echo "Debug bundle: $OUTPUT_DIR"
if [ -f "${OUTPUT_DIR}.tar.gz" ]; then
    echo "Compressed archive: ${OUTPUT_DIR}.tar.gz"
    echo "Size: $(du -h "${OUTPUT_DIR}.tar.gz" | cut -f1)"
    echo ""
    echo "Share the .tar.gz file when requesting support."
else
    echo "Share the directory when requesting support."
fi
echo ""
echo "Review README.md in the bundle for details."

