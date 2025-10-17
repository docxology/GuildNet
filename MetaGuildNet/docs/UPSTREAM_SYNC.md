# Upstream Synchronization Guide

**Maintaining sync with upstream GuildNet**

## Overview

MetaGuildNet is designed to maintain clean compatibility with upstream GuildNet while adding fork-specific enhancements. This guide covers synchronization procedures, conflict resolution, and contribution workflows.

## Sync Strategy

### Design Principles

1. **Isolation**: All fork code lives in `MetaGuildNet/`
2. **Non-Invasive**: Minimal changes to upstream files
3. **Composable**: Enhancements wrap rather than replace
4. **Upstream-First**: Bug fixes go upstream when possible

### Sync Cadence

**Regular Sync** (recommended):
- **Weekly**: Pull upstream changes
- **Monthly**: Review for breaking changes
- **Before Major Release**: Comprehensive sync and test

**On-Demand Sync**:
- When upstream releases new version
- When upstream fixes critical bug
- Before contributing back to upstream

## Synchronization Process

### 1. Setup Upstream Remote

```bash
# One-time setup
cd /path/to/GuildNet

# Add upstream remote (if not already added)
git remote add upstream https://github.com/original-org/GuildNet.git

# Verify remotes
git remote -v
# origin    https://github.com/your-org/GuildNet.git (fetch)
# origin    https://github.com/your-org/GuildNet.git (push)
# upstream  https://github.com/original-org/GuildNet.git (fetch)
# upstream  https://github.com/original-org/GuildNet.git (push)
```

### 2. Fetch Upstream Changes

```bash
# Fetch all upstream branches
git fetch upstream

# View upstream changes
git log HEAD..upstream/main --oneline

# View changed files
git diff HEAD..upstream/main --name-only
```

### 3. Merge Upstream Changes

```mermaid
flowchart TD
    Start([Start Sync Process]) --> A[Ensure main branch is clean<br/>git checkout main && git status]

    A --> B[Create backup branch<br/>git branch backup-before-sync-$(date +%Y%m%d)]

    B --> C{Merge Strategy}
    C -->|Merge| D[Merge upstream changes<br/>git merge upstream/main]
    C -->|Rebase| E[Rebase for cleaner history<br/>git rebase upstream/main]

    D --> F[Resolve any conflicts<br/>git status && manual resolution]
    E --> G[Resolve any conflicts<br/>git status && manual resolution]

    F --> H[Continue merge<br/>git merge --continue]
    G --> I[Continue rebase<br/>git rebase --continue]

    H --> J[Run verification<br/>make meta-verify]
    I --> J

    J --> K{Success?}
    K -->|Yes| L[Push to fork<br/>git push origin main]
    K -->|No| M[Rollback to backup<br/>git reset --hard backup-branch]
    M --> N[Investigate issues<br/>Check logs and diagnostics]

    style Start fill:#e8f5e8
    style J fill:#fff3e0
    style L fill:#d4edda
    style M fill:#f8d7da
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for branch management strategies.

### 4. Resolve Conflicts

If conflicts occur (rare if isolation is maintained):

```bash
# View conflicted files
git status

# For each conflict:
# 1. Open file in editor
# 2. Resolve conflict markers (<<<<, ====, >>>>)
# 3. Stage resolved file
git add <file>

# Common conflict scenarios:

# A. Makefile conflict
# Resolution: Keep both upstream targets and MetaGuildNet targets
# Example:
# <<<<<<< HEAD
# meta-setup: ## MetaGuildNet setup
# =======
# new-upstream-target: ## New target
# >>>>>>> upstream/main
# 
# Resolved:
# meta-setup: ## MetaGuildNet setup
# new-upstream-target: ## New target

# B. README conflict
# Resolution: Merge content, prefer upstream structure
# Add MetaGuildNet section at end

# C. AGENTS.md conflict
# Resolution: Keep upstream content, add MetaGuildNet reference

# After resolving all conflicts
git merge --continue
# or
git rebase --continue
```

### 5. Verify After Sync

```bash
# Run tests
make test

# Run MetaGuildNet verification
make meta-verify

# Run integration tests
make -C MetaGuildNet test-integration

# Manual smoke test
make run
# Access UI and test basic workflows
```

### 6. Push Changes

```bash
# Push to your fork
git push origin main

# If rebased (and force push is needed)
git push origin main --force-with-lease
```

## File-by-File Sync Guide

### Files MetaGuildNet Modifies

#### Makefile

**Modifications**: Adds MetaGuildNet targets

**Sync Strategy**:
```bash
# 1. Accept upstream Makefile changes
# 2. Re-add MetaGuildNet targets at end
# 3. Ensure no conflicts with upstream targets

# MetaGuildNet section in Makefile:
# ---------- MetaGuildNet ----------
meta-setup: ## MetaGuildNet automated setup
	bash ./MetaGuildNet/scripts/setup/setup_wizard.sh

meta-verify: ## MetaGuildNet comprehensive verification
	bash ./MetaGuildNet/scripts/verify/verify_all.sh

# (More MetaGuildNet targets...)
```

#### AGENTS.md

**Modifications**: May add MetaGuildNet-specific guidance

**Sync Strategy**:
```bash
# 1. Keep upstream content
# 2. Add MetaGuildNet reference if needed

# Addition at end:
## MetaGuildNet Extensions

For fork-specific features, setup automation, and verification:
- See `MetaGuildNet/README.md`
- Setup: `make meta-setup`
- Verify: `make meta-verify`
```

#### README.md (Root)

**Modifications**: May add MetaGuildNet section

**Sync Strategy**:
```bash
# 1. Keep upstream content
# 2. Add MetaGuildNet badge/section if desired

# Addition at top (badges):
[![MetaGuildNet](https://img.shields.io/badge/MetaGuildNet-Enhanced-blue)](MetaGuildNet/README.md)

# Addition in setup section:
### MetaGuildNet Quick Start

For automated setup and verification:
```bash
make meta-setup
```

See [MetaGuildNet/README.md](MetaGuildNet/README.md) for details.
```

### Files MetaGuildNet Never Modifies

These should auto-merge cleanly:
- `architecture.md` (reference, don't modify)
- `go.mod` / `go.sum` (no fork code in Go)
- `cmd/`, `internal/`, `pkg/` (no modifications)
- `ui/` (no modifications unless needed)
- Scripts in `scripts/` (no modifications)
- Kubernetes manifests in `k8s/`, `config/`

## Conflict Resolution Strategies

### Strategy 1: Accept Upstream (Preferred)

When upstream changes don't affect MetaGuildNet:

```bash
# Accept upstream version
git checkout --theirs <file>
git add <file>
```

### Strategy 2: Keep Fork Changes

When MetaGuildNet additions are independent:

```bash
# Keep fork version
git checkout --ours <file>
git add <file>
```

### Strategy 3: Manual Merge

When both have valid changes:

```bash
# Edit file manually
$EDITOR <file>

# Resolve conflicts
# Remove conflict markers
# Keep both changes when possible

git add <file>
```

### Strategy 4: Three-Way Merge Tool

For complex conflicts:

```bash
# Use merge tool
git mergetool

# Popular tools:
# - vimdiff
# - meld
# - kdiff3
# - vscode

# Configure merge tool
git config merge.tool meld
git config mergetool.meld.cmd 'meld "$LOCAL" "$BASE" "$REMOTE" --output "$MERGED"'
```

## Testing After Sync

### Automated Testing

```bash
# Full test suite
make test                              # Upstream tests
make meta-verify                       # MetaGuildNet verification
make -C MetaGuildNet test-integration  # Integration tests
make -C MetaGuildNet test-e2e          # End-to-end tests
```

### Manual Testing

```bash
# 1. Fresh setup from scratch
make clean
make meta-setup

# 2. Verify all layers
make meta-verify

# 3. Test key workflows
# - Create workspace via UI
# - Access workspace via proxy
# - Execute commands
# - Delete workspace

# 4. Test on clean environment (Docker/VM)
docker run -it --rm \
  -v $(pwd):/workspace \
  -w /workspace \
  ubuntu:22.04 \
  bash -c "apt update && apt install -y make git && make meta-setup"
```

## Contributing Back to Upstream

### When to Contribute

**Good candidates for upstream**:
- Bug fixes in GuildNet core
- Performance improvements
- Security fixes
- Generic feature enhancements
- Documentation improvements

**Keep in MetaGuildNet**:
- Setup automation (fork-specific)
- Verification tooling (fork-specific)
- Integration tests (fork-specific)
- Fork-specific documentation

### Contribution Process

```bash
# 1. Create upstream-focused branch
git checkout -b upstream-fix-xyz

# 2. Extract changes for upstream
# Create minimal patch with only necessary changes
# Remove MetaGuildNet-specific code

# 3. Test against upstream
git remote add upstream-test https://github.com/original-org/GuildNet.git
git fetch upstream-test
git checkout upstream-test/main
git cherry-pick <commit>

# 4. Create PR to upstream
# - Open PR against upstream repository
# - Follow upstream contribution guidelines
# - Reference related issues

# 5. After upstream merge, sync back to fork
git fetch upstream
git merge upstream/main
```

### Example: Contributing Bug Fix

```bash
# Found bug in internal/proxy/reverse_proxy.go

# 1. Fix in fork
git checkout -b fix/proxy-cookie-handling
# Edit file, commit fix
git commit -m "fix: correct cookie handling in proxy"

# 2. Test in fork
make test
make meta-verify

# 3. Create clean branch for upstream
git checkout upstream/main
git checkout -b upstream-fix-proxy-cookies
git cherry-pick fix/proxy-cookie-handling

# 4. Remove any MetaGuildNet-specific changes
# Edit if needed, test

# 5. Push to your fork of upstream
git push fork-upstream upstream-fix-proxy-cookies

# 6. Create PR to upstream
# Via GitHub UI

# 7. After upstream merge, sync back
git fetch upstream
git checkout main
git merge upstream/main
```

## Handling Breaking Changes

### Detecting Breaking Changes

```bash
# Review upstream commits
git log upstream/main --oneline --since="1 week ago"

# Look for:
# - BREAKING CHANGE: in commit messages
# - Major version bumps
# - API changes
# - Configuration changes

# Detailed diff
git diff HEAD..upstream/main
```

### Adapting to Breaking Changes

**Process**:

1. **Identify Impact**
   ```bash
   # Check what MetaGuildNet uses
   grep -r "changed_function" MetaGuildNet/
   ```

2. **Update MetaGuildNet Code**
   ```bash
   # Update scripts/wrappers
   vim MetaGuildNet/scripts/affected_script.sh
   ```

3. **Update Documentation**
   ```bash
   # Update docs
   vim MetaGuildNet/docs/SETUP.md
   ```

4. **Update Tests**
   ```bash
   # Update test expectations
   vim MetaGuildNet/tests/integration/affected_test.sh
   ```

5. **Test Thoroughly**
   ```bash
   make meta-verify
   make -C MetaGuildNet test-all
   ```

6. **Update CHANGELOG**
   ```bash
   cat >> MetaGuildNet/CHANGELOG.md << 'EOF'
   ## [1.2.0-meta] - 2025-10-13
   
   ### Changed
   - Adapted to upstream breaking change in proxy API
   - Updated setup scripts for new configuration format
   EOF
   ```

## Sync Automation

### Automated Sync Checks (GitHub Actions)

```yaml
# .github/workflows/sync-check.yml
name: Check Upstream Sync

on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly
  workflow_dispatch:

jobs:
  check-sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0
      
      - name: Add upstream remote
        run: |
          git remote add upstream https://github.com/original-org/GuildNet.git
          git fetch upstream
      
      - name: Check for updates
        run: |
          BEHIND=$(git rev-list --count HEAD..upstream/main)
          echo "Commits behind upstream: $BEHIND"
          
          if [ "$BEHIND" -gt 0 ]; then
            echo "::warning::Fork is $BEHIND commits behind upstream"
            git log HEAD..upstream/main --oneline
          fi
      
      - name: Create issue if behind
        if: steps.check.outputs.behind > 10
        uses: actions/github-script@v6
        with:
          script: |
            github.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: 'Upstream sync needed',
              body: 'Fork is more than 10 commits behind upstream. Consider syncing.'
            })
```

### Sync Helper Script

```bash
#!/bin/bash
# MetaGuildNet/scripts/utils/sync_upstream.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

main() {
    log_section "Upstream Sync"
    
    # Fetch upstream
    log_info "Fetching upstream..."
    git fetch upstream
    
    # Show status
    local behind
    behind=$(git rev-list --count HEAD..upstream/main)
    log_info "Commits behind upstream: $behind"
    
    if [ "$behind" -eq 0 ]; then
        log_success "Already up to date"
        return 0
    fi
    
    # Show commits
    log_info "Upstream commits:"
    git log --oneline HEAD..upstream/main
    
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
        return 1
    fi
    
    # Verify
    log_info "Running verification..."
    if make meta-verify; then
        log_success "Verification passed"
    else
        log_error "Verification failed"
        log_info "Fix issues or rollback: git reset --hard $backup"
        return 1
    fi
    
    log_success "Sync complete"
}

main "$@"
```

Usage:
```bash
bash MetaGuildNet/scripts/utils/sync_upstream.sh
```

## Maintenance Schedule

### Weekly
- [ ] Check upstream for updates
- [ ] Review upstream commits
- [ ] Sync if low-risk changes

### Monthly
- [ ] Full sync with upstream
- [ ] Run comprehensive tests
- [ ] Update documentation if needed
- [ ] Review for breaking changes

### Before Release
- [ ] Sync with upstream latest
- [ ] Full test suite
- [ ] Update CHANGELOG
- [ ] Tag release

### After Upstream Release
- [ ] Immediate sync check
- [ ] Review release notes
- [ ] Test compatibility
- [ ] Update version references

## Troubleshooting

### Problem: Merge Conflicts Every Sync

**Solution**: Review fork modifications
```bash
# Find modified upstream files
git diff upstream/main --name-only | grep -v "^MetaGuildNet/"

# These shouldn't exist or should be minimal
# If many conflicts, consider:
# 1. Moving more code to MetaGuildNet/
# 2. Using wrapper pattern instead of modification
# 3. Contributing changes upstream
```

### Problem: Tests Fail After Sync

**Solution**: Incremental debugging
```bash
# 1. Identify what broke
make test               # Upstream tests
make meta-verify        # MetaGuildNet verification

# 2. Bisect to find breaking commit
git bisect start
git bisect bad HEAD
git bisect good <last-working-commit>
# Test at each step, mark good/bad

# 3. Fix the issue
# 4. Update MetaGuildNet if needed
```

### Problem: Can't Contribute Back

**Solution**: Extract to separate branch
```bash
# 1. Create clean branch from upstream
git fetch upstream
git checkout -b upstream-contribution upstream/main

# 2. Cherry-pick relevant commits
git cherry-pick <commit-sha>

# 3. Clean up MetaGuildNet-specific code
# 4. Test against upstream
# 5. Submit PR
```

## Best Practices

1. **Sync Regularly**: Weekly or bi-weekly
2. **Keep Fork Minimal**: Most code in MetaGuildNet/
3. **Test Thoroughly**: After every sync
4. **Document Changes**: Update CHANGELOG
5. **Backup Before Sync**: Create backup branch
6. **Contribute Back**: Share improvements upstream
7. **Monitor Upstream**: Watch upstream repository
8. **Communicate**: Discuss major changes with upstream maintainers

## Resources

### GuildNet Repository
- [Upstream Repository](https://github.com/original-org/GuildNet) - Original GuildNet project
- [Fork Repository](https://github.com/your-org/GuildNet) - Your MetaGuildNet fork
- [Core Architecture](../../architecture.md) - Upstream design principles
- [Makefile](../../Makefile) - Build targets and automation
- [Go Module](../../go.mod) - Dependency management

### MetaGuildNet Extensions
- [Architecture Guide](ARCHITECTURE.md) - Fork-specific design decisions
- [Setup Guide](SETUP.md) - Installation and configuration procedures
- [Verification Guide](VERIFICATION.md) - Testing and health check procedures
- [Contributing Guide](CONTRIBUTING.md) - Development and testing workflows

### External Resources
- [GitHub Syncing a Fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/syncing-a-fork) - Official GitHub fork sync guide
- [Git Merge Strategies](https://git-scm.com/docs/merge-strategies) - Git merge strategy documentation
- [Git Rebase](https://git-scm.com/docs/git-rebase) - Interactive rebase guide
- [Conventional Commits](https://www.conventionalcommits.org/) - Commit message standards
- [Semantic Versioning](https://semver.org/) - Version numbering guidelines

### Tools and Scripts
- [Sync Helper Script](../scripts/utils/sync_upstream.sh) - Automated sync script
- [Git Configuration](https://git-scm.com/docs/git-config) - Git configuration options
- [Makefile Targets](../../Makefile) - MetaGuildNet automation commands

### Community and Support
- [GitHub Issues](https://github.com/your-org/GuildNet/issues) - Bug reports and feature requests
- [GitHub Discussions](https://github.com/your-org/GuildNet/discussions) - Community discussions
- [Git Community Book](https://git-scm.com/book) - Comprehensive Git guide

---

**Remember**: Good isolation in MetaGuildNet/ makes syncing painless. Keep upstream files clean!

## Best Practices Summary

1. **Sync Regularly**: Weekly or bi-weekly to avoid large conflicts
2. **Maintain Isolation**: Keep MetaGuildNet-specific code in MetaGuildNet/
3. **Test Thoroughly**: Run `make meta-verify` after every sync
4. **Backup Before Sync**: Always create a backup branch
5. **Contribute Back**: Share improvements with upstream when possible
6. **Document Changes**: Update CHANGELOG for significant changes
7. **Monitor Upstream**: Watch upstream repository for breaking changes

