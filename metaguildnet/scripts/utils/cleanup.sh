#!/bin/bash
# Clean up test resources and temporary files

set -e

DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Clean up GuildNet test resources and temporary files.

OPTIONS:
    --dry-run       Show what would be deleted without actually deleting
    --force         Skip confirmation prompts
    --all           Clean everything including configs (use with caution)
    --workspaces    Clean only workspace resources
    --logs          Clean only logs
    --help          Show this help message

EXAMPLES:
    # Preview what would be cleaned
    $0 --dry-run

    # Clean workspaces only
    $0 --workspaces

    # Clean everything without confirmation
    $0 --all --force

ENVIRONMENT:
    DRY_RUN=true    Enable dry-run mode
    FORCE=true      Enable force mode

EOF
    exit 0
}

# Parse arguments
CLEAN_WORKSPACES=false
CLEAN_LOGS=false
CLEAN_ALL=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --all)
            CLEAN_ALL=true
            shift
            ;;
        --workspaces)
            CLEAN_WORKSPACES=true
            shift
            ;;
        --logs)
            CLEAN_LOGS=true
            shift
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# If no specific clean target, clean common temporary items
if [ "$CLEAN_ALL" = "false" ] && [ "$CLEAN_WORKSPACES" = "false" ] && [ "$CLEAN_LOGS" = "false" ]; then
    CLEAN_WORKSPACES=true
    CLEAN_LOGS=true
fi

echo "GuildNet Cleanup Utility"
echo "========================"
echo ""

if [ "$DRY_RUN" = "true" ]; then
    echo "DRY RUN MODE - No changes will be made"
    echo ""
fi

# Confirm unless force mode
if [ "$FORCE" = "false" ] && [ "$DRY_RUN" = "false" ]; then
    echo "This will delete temporary resources and files."
    read -p "Continue? (y/N) " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled"
        exit 0
    fi
fi

# Helper function to execute or preview
execute() {
    local cmd="$1"
    if [ "$DRY_RUN" = "true" ]; then
        echo "[DRY RUN] $cmd"
    else
        echo "  Running: $cmd"
        eval "$cmd" || echo "  ⚠ Command failed (continuing)"
    fi
}

# Clean workspaces
if [ "$CLEAN_WORKSPACES" = "true" ] || [ "$CLEAN_ALL" = "true" ]; then
    echo ""
    echo "==> Cleaning workspace resources..."
    
    # Delete test workspaces
    test_workspaces=$(kubectl get workspaces --all-namespaces -o jsonpath='{range .items[?(@.metadata.labels.test=="true")]}{.metadata.namespace}{","}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
    
    if [ -n "$test_workspaces" ]; then
        echo "Found test workspaces:"
        echo "$test_workspaces" | while IFS=',' read -r ns name; do
            if [ -n "$ns" ] && [ -n "$name" ]; then
                echo "  - $ns/$name"
                execute "kubectl delete workspace -n $ns $name"
            fi
        done
    else
        echo "  No test workspaces found"
    fi
    
    # Delete failed pods
    failed_pods=$(kubectl get pods --all-namespaces --field-selector=status.phase=Failed -o jsonpath='{range .items[*]}{.metadata.namespace}{","}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
    
    if [ -n "$failed_pods" ]; then
        echo ""
        echo "Cleaning failed pods:"
        echo "$failed_pods" | while IFS=',' read -r ns pod; do
            if [ -n "$ns" ] && [ -n "$pod" ]; then
                echo "  - $ns/$pod"
                execute "kubectl delete pod -n $ns $pod"
            fi
        done
    else
        echo "  No failed pods found"
    fi
    
    # Delete completed jobs
    completed_jobs=$(kubectl get jobs --all-namespaces --field-selector=status.successful=1 -o jsonpath='{range .items[*]}{.metadata.namespace}{","}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
    
    if [ -n "$completed_jobs" ]; then
        echo ""
        echo "Cleaning completed jobs:"
        echo "$completed_jobs" | while IFS=',' read -r ns job; do
            if [ -n "$ns" ] && [ -n "$job" ]; then
                echo "  - $ns/$job"
                execute "kubectl delete job -n $ns $job"
            fi
        done
    else
        echo "  No completed jobs found"
    fi
fi

# Clean logs
if [ "$CLEAN_LOGS" = "true" ] || [ "$CLEAN_ALL" = "true" ]; then
    echo ""
    echo "==> Cleaning logs..."
    
    # Clean temporary log directories
    for pattern in "guildnet-logs-*" "guildnet-debug-*"; do
        if ls -d "$pattern" 2>/dev/null; then
            for dir in $pattern; do
                echo "  - $dir"
                execute "rm -rf $dir"
            done
        fi
    done
    
    # Clean log archives
    for pattern in "guildnet-logs-*.tar.gz" "guildnet-debug-*.tar.gz"; do
        if ls "$pattern" 2>/dev/null; then
            for file in $pattern; do
                echo "  - $file"
                execute "rm -f $file"
            done
        fi
    done
    
    # Clean temporary files in /tmp
    if [ -d /tmp ]; then
        tmp_files=$(find /tmp -name "guildnet-*" -type f -mtime +7 2>/dev/null || true)
        if [ -n "$tmp_files" ]; then
            echo ""
            echo "Cleaning old temp files:"
            echo "$tmp_files" | while read -r file; do
                echo "  - $file"
                execute "rm -f $file"
            done
        fi
    fi
fi

# Clean all (including configs)
if [ "$CLEAN_ALL" = "true" ]; then
    echo ""
    echo "==> Full cleanup (including configs)..."
    
    # Clean Docker images
    echo ""
    echo "Cleaning unused Docker images:"
    execute "docker image prune -af --filter 'label=guildnet.io/test=true'"
    
    # Clean Docker volumes
    echo ""
    echo "Cleaning unused Docker volumes:"
    execute "docker volume prune -f"
    
    # Warning about config deletion
    if [ "$FORCE" = "false" ]; then
        echo ""
        echo "⚠ WARNING: About to delete configuration files!"
        read -p "Delete configs too? (y/N) " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Skipping config deletion"
        else
            # Clean config files
            echo ""
            echo "Cleaning config files:"
            if [ -d ~/.config/guildnet ]; then
                echo "  - ~/.config/guildnet"
                execute "rm -rf ~/.config/guildnet"
            fi
            
            if [ -f ~/.guildnet.yaml ]; then
                echo "  - ~/.guildnet.yaml"
                execute "rm -f ~/.guildnet.yaml"
            fi
        fi
    fi
fi

# Summary
echo ""
echo "✓ Cleanup complete"

if [ "$DRY_RUN" = "true" ]; then
    echo ""
    echo "This was a dry run. Run without --dry-run to actually delete resources."
fi

echo ""
echo "Current resource usage:"
echo "  Workspaces: $(kubectl get workspaces --all-namespaces 2>/dev/null | wc -l) total"
echo "  Pods: $(kubectl get pods --all-namespaces 2>/dev/null | wc -l) total"
echo "  Docker images: $(docker images | wc -l) total"

