#!/bin/bash
# Backup GuildNet configurations and data

set -e

BACKUP_DIR="${1:-./guildnet-backup-$(date +%Y%m%d-%H%M%S)}"
INCLUDE_DATA="${INCLUDE_DATA:-true}"
INCLUDE_SECRETS="${INCLUDE_SECRETS:-false}"

usage() {
    cat << EOF
Usage: $0 [BACKUP_DIR]

Backup GuildNet configurations and data.

ARGUMENTS:
    BACKUP_DIR      Directory to store backup (default: ./guildnet-backup-TIMESTAMP)

ENVIRONMENT:
    INCLUDE_DATA=false      Skip backing up RethinkDB data
    INCLUDE_SECRETS=true    Include Kubernetes secrets (use with caution)

EXAMPLES:
    # Basic backup
    $0

    # Backup to specific directory
    $0 /backups/guildnet-prod

    # Backup configs only (skip data)
    INCLUDE_DATA=false $0

    # Full backup including secrets
    INCLUDE_SECRETS=true $0

EOF
    exit 0
}

if [[ "$1" == "--help" ]]; then
    usage
fi

echo "GuildNet Backup Utility"
echo "======================="
echo ""
echo "Backup directory: $BACKUP_DIR"
echo "Include data: $INCLUDE_DATA"
echo "Include secrets: $INCLUDE_SECRETS"
echo ""

mkdir -p "$BACKUP_DIR"

# Backup configuration files
echo "==> Backing up configuration files..."

if [ -d ~/.config/guildnet ]; then
    echo "  • GuildNet user config"
    cp -r ~/.config/guildnet "$BACKUP_DIR/guildnet-config" 2>/dev/null || echo "    ⚠ Failed"
fi

if [ -f ~/.guildnet.yaml ]; then
    echo "  • GuildNet YAML config"
    cp ~/.guildnet.yaml "$BACKUP_DIR/guildnet.yaml" 2>/dev/null || echo "    ⚠ Failed"
fi

if [ -f /etc/headscale/config.yaml ]; then
    echo "  • Headscale config"
    mkdir -p "$BACKUP_DIR/headscale"
    cp /etc/headscale/config.yaml "$BACKUP_DIR/headscale/" 2>/dev/null || echo "    ⚠ Failed (need root?)"
fi

# Backup Kubernetes resources
echo ""
echo "==> Backing up Kubernetes resources..."

mkdir -p "$BACKUP_DIR/k8s"

echo "  • Namespaces"
kubectl get namespaces -o yaml > "$BACKUP_DIR/k8s/namespaces.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • CustomResourceDefinitions"
kubectl get crd -o yaml > "$BACKUP_DIR/k8s/crds.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • Workspaces"
kubectl get workspaces --all-namespaces -o yaml > "$BACKUP_DIR/k8s/workspaces.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • Capabilities"
kubectl get capabilities --all-namespaces -o yaml > "$BACKUP_DIR/k8s/capabilities.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • ConfigMaps"
kubectl get configmaps --all-namespaces -o yaml > "$BACKUP_DIR/k8s/configmaps.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • Services"
kubectl get services --all-namespaces -o yaml > "$BACKUP_DIR/k8s/services.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • PersistentVolumeClaims"
kubectl get pvc --all-namespaces -o yaml > "$BACKUP_DIR/k8s/pvcs.yaml" 2>/dev/null || echo "    ⚠ Failed"

echo "  • PersistentVolumes"
kubectl get pv -o yaml > "$BACKUP_DIR/k8s/pvs.yaml" 2>/dev/null || echo "    ⚠ Failed"

if [ "$INCLUDE_SECRETS" = "true" ]; then
    echo "  • Secrets (encrypted)"
    kubectl get secrets --all-namespaces -o yaml > "$BACKUP_DIR/k8s/secrets.yaml" 2>/dev/null || echo "    ⚠ Failed"
    echo "    ⚠ WARNING: Secrets are backed up in base64 encoding"
    echo "    ⚠ Protect this backup file appropriately"
fi

# Backup RethinkDB data
if [ "$INCLUDE_DATA" = "true" ]; then
    echo ""
    echo "==> Backing up RethinkDB data..."
    
    rethinkdb_pod=$(kubectl get pods -n default -l app=rethinkdb -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    
    if [ -n "$rethinkdb_pod" ]; then
        echo "  Found RethinkDB pod: $rethinkdb_pod"
        mkdir -p "$BACKUP_DIR/rethinkdb"
        
        # Export databases
        echo "  • Exporting databases..."
        kubectl exec -n default "$rethinkdb_pod" -- rethinkdb dump -f /tmp/rethinkdb_dump.tar.gz 2>/dev/null || {
            echo "    ⚠ Failed to dump (rethinkdb dump may not be available)"
        }
        
        # Copy the dump
        kubectl cp "default/$rethinkdb_pod:/tmp/rethinkdb_dump.tar.gz" "$BACKUP_DIR/rethinkdb/dump.tar.gz" 2>/dev/null || {
            echo "    ⚠ Failed to copy dump"
        }
        
        # Clean up
        kubectl exec -n default "$rethinkdb_pod" -- rm -f /tmp/rethinkdb_dump.tar.gz 2>/dev/null || true
        
        if [ -f "$BACKUP_DIR/rethinkdb/dump.tar.gz" ]; then
            echo "    ✓ Database backup complete"
        fi
    else
        echo "  ⚠ RethinkDB pod not found, skipping data backup"
    fi
fi

# Backup Headscale data
echo ""
echo "==> Backing up Headscale data..."

if [ -f /var/lib/headscale/db.sqlite ]; then
    echo "  • Headscale database"
    mkdir -p "$BACKUP_DIR/headscale"
    cp /var/lib/headscale/db.sqlite "$BACKUP_DIR/headscale/db.sqlite" 2>/dev/null || echo "    ⚠ Failed (need root?)"
fi

# Create backup metadata
echo ""
echo "==> Creating backup metadata..."

cat > "$BACKUP_DIR/BACKUP_INFO.txt" << EOF
GuildNet Backup
===============

Created: $(date)
Hostname: $(hostname)
User: $(whoami)

Backup Contents:
- Configuration files (GuildNet, Headscale)
- Kubernetes resources (CRDs, Workspaces, ConfigMaps, etc.)
$([ "$INCLUDE_SECRETS" = "true" ] && echo "- Kubernetes secrets (⚠ SENSITIVE)" || echo "- Kubernetes secrets (NOT included)")
$([ "$INCLUDE_DATA" = "true" ] && echo "- RethinkDB data" || echo "- RethinkDB data (NOT included)")
- Headscale database

Versions:
- Kubernetes: $(kubectl version --short 2>/dev/null | head -1 || echo "Unknown")
- GuildNet: $(hostapp --version 2>/dev/null || echo "Unknown")
- Headscale: $(headscale version 2>/dev/null || echo "Unknown")

Cluster Info:
- Nodes: $(kubectl get nodes --no-headers 2>/dev/null | wc -l || echo "Unknown")
- Namespaces: $(kubectl get namespaces --no-headers 2>/dev/null | wc -l || echo "Unknown")
- Workspaces: $(kubectl get workspaces --all-namespaces --no-headers 2>/dev/null | wc -l || echo "Unknown")

Restore Instructions:
1. Review the contents of this backup
2. Restore configuration files to their original locations
3. Apply Kubernetes resources: kubectl apply -f k8s/
4. Restore RethinkDB data if needed
5. Verify cluster status with: mgn cluster list

IMPORTANT:
- Test restore procedure in a non-production environment first
- Secrets are sensitive - protect this backup appropriately
- Database backups may require matching RethinkDB versions

EOF

# Create restore script
cat > "$BACKUP_DIR/restore.sh" << 'EOF'
#!/bin/bash
# Restore GuildNet from backup

set -e

echo "GuildNet Restore Utility"
echo "========================"
echo ""
echo "⚠ WARNING: This will restore from backup"
echo "⚠ Existing resources may be overwritten"
echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cancelled"
    exit 0
fi

BACKUP_DIR="$(dirname "$0")"

echo ""
echo "==> Restoring Kubernetes resources..."
kubectl apply -f "$BACKUP_DIR/k8s/crds.yaml" || echo "  ⚠ CRDs failed"
sleep 5
kubectl apply -f "$BACKUP_DIR/k8s/namespaces.yaml" || echo "  ⚠ Namespaces failed"
kubectl apply -f "$BACKUP_DIR/k8s/configmaps.yaml" || echo "  ⚠ ConfigMaps failed"

if [ -f "$BACKUP_DIR/k8s/secrets.yaml" ]; then
    echo "  • Restoring secrets"
    kubectl apply -f "$BACKUP_DIR/k8s/secrets.yaml" || echo "  ⚠ Secrets failed"
fi

kubectl apply -f "$BACKUP_DIR/k8s/pvs.yaml" || echo "  ⚠ PVs failed"
kubectl apply -f "$BACKUP_DIR/k8s/pvcs.yaml" || echo "  ⚠ PVCs failed"
kubectl apply -f "$BACKUP_DIR/k8s/services.yaml" || echo "  ⚠ Services failed"
kubectl apply -f "$BACKUP_DIR/k8s/workspaces.yaml" || echo "  ⚠ Workspaces failed"
kubectl apply -f "$BACKUP_DIR/k8s/capabilities.yaml" || echo "  ⚠ Capabilities failed"

echo ""
echo "==> Restoring configuration files..."
if [ -d "$BACKUP_DIR/guildnet-config" ]; then
    mkdir -p ~/.config/guildnet
    cp -r "$BACKUP_DIR/guildnet-config/"* ~/.config/guildnet/ || echo "  ⚠ Failed"
fi

echo ""
echo "✓ Restore complete"
echo ""
echo "Next steps:"
echo "1. Verify cluster status: mgn cluster list"
echo "2. Check workspaces: kubectl get workspaces --all-namespaces"
echo "3. Restore RethinkDB data if needed (manual step)"
echo "4. Verify Headscale connectivity"

EOF

chmod +x "$BACKUP_DIR/restore.sh"

# Compress the backup
echo ""
echo "==> Compressing backup..."
tar -czf "${BACKUP_DIR}.tar.gz" "$BACKUP_DIR" 2>/dev/null || {
    echo "  ⚠ Failed to compress (continuing anyway)"
}

# Calculate sizes
BACKUP_SIZE=$(du -sh "$BACKUP_DIR" | cut -f1)
if [ -f "${BACKUP_DIR}.tar.gz" ]; then
    ARCHIVE_SIZE=$(du -h "${BACKUP_DIR}.tar.gz" | cut -f1)
fi

echo ""
echo "✓ Backup complete"
echo ""
echo "Backup location: $BACKUP_DIR"
echo "Backup size: $BACKUP_SIZE"

if [ -f "${BACKUP_DIR}.tar.gz" ]; then
    echo "Compressed archive: ${BACKUP_DIR}.tar.gz"
    echo "Archive size: $ARCHIVE_SIZE"
    echo ""
    echo "Store the .tar.gz file in a safe location."
fi

echo ""
echo "To restore from this backup:"
echo "  1. Extract: tar -xzf ${BACKUP_DIR}.tar.gz"
echo "  2. Run: ./${BACKUP_DIR}/restore.sh"
echo ""
echo "Review BACKUP_INFO.txt for details."

if [ "$INCLUDE_SECRETS" = "true" ]; then
    echo ""
    echo "⚠ WARNING: This backup contains sensitive secrets"
    echo "⚠ Store securely and restrict access"
fi

