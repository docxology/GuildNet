#!/bin/bash
# Sync with upstream GuildNet

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

PROJECT_ROOT="$(get_project_root)"

main() {
    log_section "Upstream Sync"
    
    cd "$PROJECT_ROOT" || return 1
    
    # Check if upstream remote exists
    if ! git remote | grep -q "^upstream$"; then
        log_info "Adding upstream remote..."
        if ! confirm "Add upstream remote github.com/HexaField/GuildNet?"; then
            log_info "Sync cancelled"
            return 0
        fi
        git remote add upstream https://github.com/HexaField/GuildNet.git
    fi
    
    # Fetch upstream
    log_info "Fetching upstream..."
    git fetch upstream
    
    # Show status
    local behind
    behind=$(git rev-list --count HEAD..upstream/main 2>/dev/null || echo "0")
    log_info "Commits behind upstream: $behind"
    
    if [[ "$behind" == "0" ]]; then
        log_success "Already up to date"
        return 0
    fi
    
    # Show commits
    log_info "Upstream commits:"
    git log --oneline HEAD..upstream/main | head -10
    
    # Prompt to continue
    if ! confirm "Merge these commits?"; then
        log_info "Sync cancelled"
        return 0
    fi
    
    # Create backup
    local backup="backup-before-sync-$(date +%Y%m%d-%H%M%S)"
    log_info "Creating backup branch: $backup"
    git branch "$backup"
    
    # Merge
    log_info "Merging upstream/main..."
    if git merge upstream/main; then
        log_success "Merge successful"
    else
        log_error "Merge conflicts detected"
        log_info "Resolve conflicts and run: git merge --continue"
        log_info "Or rollback: git reset --hard $backup"
        return 1
    fi
    
    # Verify
    log_info "Running verification..."
    if bash "${SCRIPT_DIR}/../verify/verify_all.sh"; then
        log_success "Verification passed"
    else
        log_warn "Verification failed"
        log_info "Fix issues or rollback: git reset --hard $backup"
        return 1
    fi
    
    log_success "Sync complete"
    log_info "Backup available at: $backup"
}

main "$@"

