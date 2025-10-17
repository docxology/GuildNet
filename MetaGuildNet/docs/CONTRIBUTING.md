# Contributing to MetaGuildNet

**Guidelines for contributing to this GuildNet fork**

## Welcome

Thank you for considering contributing to MetaGuildNet! This document provides guidelines for contributing to this fork while maintaining compatibility with upstream GuildNet.

## Philosophy

MetaGuildNet follows these core principles:

1. **Default-First**: Everything works out of the box
2. **Production-Ready**: No dev/prod split
3. **Modular**: Each component does one thing well
4. **Composable**: Components are replaceable
5. **Upstream-Compatible**: Clean merge path with GuildNet

> "Do or do not, there is no try."

## Getting Started

### Prerequisites

- Familiarity with Git and GitHub workflows
- Understanding of GuildNet architecture (see [../architecture.md](../../architecture.md))
- Local development environment set up (see [SETUP.md](SETUP.md))

### Development Setup

```bash
# 1. Fork the repository
# Click "Fork" on GitHub

# 2. Clone your fork
git clone https://github.com/<your-username>/GuildNet.git
cd GuildNet

# 3. Add upstream remote
git remote add upstream https://github.com/original-org/GuildNet.git

# 4. Create feature branch
git checkout -b feature/meta-your-feature

# 5. Make changes in MetaGuildNet/
# All fork-specific code goes in MetaGuildNet/

# 6. Test your changes
make meta-verify
make -C MetaGuildNet test-all

# 7. Commit with clear messages
git commit -m "feat(meta): add workspace validation"

# 8. Push and create PR
git push origin feature/meta-your-feature
```

## Contribution Guidelines

### Where to Put Code

**MetaGuildNet/** (fork-specific)
- Setup automation
- Verification scripts
- Convenience wrappers
- Integration tests
- Documentation enhancements
- Examples

**Outside MetaGuildNet/** (consider upstream PR)
- Bug fixes
- Core feature improvements
- Performance optimizations
- Security fixes

### Code Organization

```
MetaGuildNet/
├── docs/                  # Documentation only
├── scripts/
│   ├── setup/            # Setup automation
│   ├── verify/           # Verification scripts
│   └── utils/            # Utility functions
├── tests/
│   ├── integration/      # Integration tests
│   └── e2e/              # End-to-end tests
└── examples/             # Usage examples
    ├── basic/
    └── advanced/
```

**Rules:**
- No production code in `MetaGuildNet/` that modifies upstream behavior
- Wrappers and automation only
- Tests and documentation
- Configuration templates

### Code Style

#### Shell Scripts

```bash
#!/bin/bash
# Script description
# Usage: script.sh [options]

set -euo pipefail  # Fail fast

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

# Constants
readonly DEFAULT_TIMEOUT=300
readonly LOG_FILE="/var/log/guildnet.log"

# Functions
setup_environment() {
    local config_file="${1:-.env}"
    
    log_info "Loading configuration from ${config_file}"
    
    if [[ ! -f "${config_file}" ]]; then
        log_error "Config file not found: ${config_file}"
        return 1
    fi
    
    # shellcheck disable=SC1090
    source "${config_file}"
}

# Main
main() {
    log_info "Starting setup..."
    
    setup_environment "$@"
    
    log_success "Setup complete"
}

# Execute
main "$@"
```

**Shell Style Guide:**
- Use `bash` (not `sh`)
- Set `set -euo pipefail`
- Use `readonly` for constants
- Use `local` for function variables
- Quote variables: `"${var}"`
- Use `log_*` functions for output
- Provide usage information
- Include error handling
- Add shellcheck directives when needed

#### Go Code (if modifying upstream)

```go
package main

import (
    "context"
    "fmt"
    "time"
)

// Component represents a verifiable component.
type Component struct {
    Name        string
    Description string
    Timeout     time.Duration
}

// Verify checks if the component is healthy.
func (c *Component) Verify(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, c.Timeout)
    defer cancel()
    
    // Verification logic
    if err := c.checkHealth(ctx); err != nil {
        return fmt.Errorf("health check failed: %w", err)
    }
    
    return nil
}

func (c *Component) checkHealth(ctx context.Context) error {
    // Implementation
    return nil
}
```

**Go Style Guide:**
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` and `golangci-lint`
- Write tests for all new code
- Use context for cancellation
- Return errors, don't panic
- Document public APIs

#### Documentation

```markdown
# Title (imperative, capitalized)

**Brief description**

## Section

Content with:
- **Bold** for emphasis
- `code` for commands/variables
- Code blocks with language tags

### Subsection

Examples should be runnable:

```bash
# Comment explaining the command
make target OPTION=value

# Expected output:
# Output goes here
```

**Tips:**
- Use tables for comparisons
- Add diagrams where helpful
- Link to related docs
```

### Testing Requirements

All contributions must include appropriate tests:

#### 1. Unit Tests (if modifying Go code)

```go
func TestComponentVerify(t *testing.T) {
    tests := []struct {
        name    string
        timeout time.Duration
        wantErr bool
    }{
        {"success", 10 * time.Second, false},
        {"timeout", 1 * time.Millisecond, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c := &Component{
                Name:    "test",
                Timeout: tt.timeout,
            }
            
            err := c.Verify(context.Background())
            if (err != nil) != tt.wantErr {
                t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

#### 2. Integration Tests

```bash
#!/bin/bash
# tests/integration/example_test.sh

source "$(dirname "$0")/../lib/test_framework.sh"

test_suite "Example Integration Test"

test_case "component X integrates with component Y" {
    setup() {
        # Arrange
        start_component_x
        start_component_y
    }
    
    run() {
        # Act
        result=$(trigger_interaction)
    }
    
    assert() {
        # Assert
        assert_equals "$result" "expected"
    }
    
    teardown() {
        # Cleanup
        stop_component_x
        stop_component_y
    }
}

run_test_suite
```

#### 3. End-to-End Tests

```bash
#!/bin/bash
# tests/e2e/example_workflow.sh

source "$(dirname "$0")/../lib/e2e_framework.sh"

e2e_test "Full workflow" {
    # Create workspace
    workspace_id=$(create_workspace "test-image")
    
    # Wait for running
    wait_for_workspace_running "$workspace_id" 120
    
    # Test access
    response=$(curl -sk "https://127.0.0.1:8080/proxy/server/$workspace_id/")
    assert_contains "$response" "expected-content"
    
    # Cleanup
    delete_workspace "$workspace_id"
}
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Tests
- `refactor`: Code restructuring
- `perf`: Performance improvement
- `chore`: Maintenance

**Scopes:**
- `meta`: MetaGuildNet-specific
- `setup`: Setup scripts
- `verify`: Verification
- `test`: Testing
- `docs`: Documentation
- `examples`: Examples

**Examples:**

```bash
# Good commits
git commit -m "feat(meta): add automated route approval"
git commit -m "fix(verify): correct timeout handling in network checks"
git commit -m "docs(meta): add troubleshooting guide for Tailscale"
git commit -m "test(integration): add cluster-to-database connectivity test"

# Bad commits
git commit -m "fixed stuff"
git commit -m "WIP"
git commit -m "updates"
```

**Multi-line commits:**

```
feat(meta): add comprehensive verification framework

- Implemented multi-layer verification (L1-L4)
- Added JSON output format for automation
- Included diagnostic recommendations
- Created reusable verification library

Closes #123
```

## Pull Request Process

### 1. Before Submitting

```bash
# Ensure code quality
make lint
make test

# Ensure verification passes
make meta-verify

# Run integration tests
make -C MetaGuildNet test-integration

# Run E2E tests
make -C MetaGuildNet test-e2e

# Check for merge conflicts
git fetch upstream
git merge upstream/main
```

### 2. PR Template

```markdown
## Description

Brief description of changes.

## Type of Change

- [ ] Bug fix (non-breaking change fixing an issue)
- [ ] New feature (non-breaking change adding functionality)
- [ ] Breaking change (fix or feature that would break existing functionality)
- [ ] Documentation update

## MetaGuildNet Scope

- [ ] Setup automation
- [ ] Verification scripts
- [ ] Tests
- [ ] Documentation
- [ ] Examples
- [ ] Other: ___________

## How Has This Been Tested?

Describe testing performed:
- [ ] Integration tests pass
- [ ] E2E tests pass
- [ ] Manual testing performed

## Checklist

- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex code
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] All tests passing
- [ ] No breaking changes to upstream compatibility
- [ ] Commit messages follow convention
```

### 3. Review Process

1. **Automated Checks**: CI runs tests and linting
2. **Maintainer Review**: Code review by maintainers
3. **Community Feedback**: Optional community input
4. **Approval**: At least one maintainer approval required
5. **Merge**: Squash and merge to main

### 4. After Merge

- Update your local repository
- Close related issues
- Update documentation if needed
- Celebrate! 🎉

## Issue Reporting

### Bug Reports

Use the bug report template:

```markdown
**Describe the bug**
Clear description of what the bug is.

**To Reproduce**
Steps to reproduce:
1. Run '...'
2. Click on '...'
3. See error

**Expected behavior**
What you expected to happen.

**Actual behavior**
What actually happened.

**Environment**
- OS: [e.g., Ubuntu 22.04]
- GuildNet version: [e.g., v0.1.0]
- Deployment: [e.g., single-node, multi-node]

**Logs**
```
Paste relevant logs here
```

**Additional context**
Any other relevant information.
```

### Feature Requests

Use the feature request template:

```markdown
**Is your feature request related to a problem?**
Description of the problem.

**Describe the solution you'd like**
What you want to happen.

**Describe alternatives you've considered**
Alternative solutions or features.

**Additional context**
Any other relevant information.

**Scope**
- [ ] MetaGuildNet enhancement
- [ ] Consider for upstream GuildNet
```

## Development Workflow

### Branching Strategy

```mermaid
gitgraph
    commit id: "Initial commit"
    branch main
    checkout main
    commit id: "feat: add verification framework"
    branch feature/meta-verification
    checkout feature/meta-verification
    commit id: "test: add integration tests"
    branch test/meta-integration
    checkout test/meta-integration
    commit id: "docs: update setup guide"
    branch docs/meta-setup-guide
    checkout docs/meta-setup-guide
    commit id: "fix: resolve route approval issue"
    branch fix/meta-route-approval
    checkout fix/meta-route-approval
    commit id: "Merge fix to main"
    checkout main
    merge fix/meta-route-approval
    commit id: "Merge docs to main"
    merge docs/meta-setup-guide
    commit id: "Merge tests to main"
    merge test/meta-integration
    commit id: "Merge feature to main"
    merge feature/meta-verification
```

**Branch Naming:**
- `feature/meta-short-description` - New MetaGuildNet features
- `fix/meta-issue-123` - Bug fixes in MetaGuildNet components
- `docs/meta-setup-guide` - Documentation improvements
- `test/meta-integration-network` - Test additions

See [UPSTREAM_SYNC.md](UPSTREAM_SYNC.md) for upstream synchronization workflows.

### Release Process

MetaGuildNet follows semantic versioning:

```
vMAJOR.MINOR.PATCH-meta

Examples:
- v1.0.0-meta (initial release)
- v1.1.0-meta (new features)
- v1.1.1-meta (bug fixes)
```

**Release Checklist:**
1. Update CHANGELOG.md
2. Update version in README.md
3. Run full test suite
4. Create release branch
5. Tag release
6. Create GitHub release
7. Announce in discussions

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: Questions and general discussion
- **Pull Requests**: Code contributions
- **Documentation**: In-repo docs and wiki

### Code of Conduct

Be respectful, inclusive, and constructive:

- **Be respectful**: Treat everyone with respect
- **Be inclusive**: Welcome newcomers
- **Be constructive**: Provide helpful feedback
- **Be patient**: Remember everyone was a beginner once
- **Be professional**: Keep interactions professional

## Recognition

Contributors are recognized in:
- CONTRIBUTORS.md file
- GitHub contributors graph
- Release notes
- Project website (if applicable)

## Getting Help

- **Documentation**: Check MetaGuildNet/docs/
- **Examples**: Browse MetaGuildNet/examples/
- **Issues**: Search existing issues
- **Discussions**: Ask in GitHub Discussions
- **Maintainers**: Tag maintainers for guidance

## Advanced Topics

### Creating New Verification Layers

```bash
# 1. Create verification script
cat > MetaGuildNet/scripts/verify/custom_layer.sh << 'EOF'
#!/bin/bash
source "$(dirname "$0")/../../lib/verify_framework.sh"

verify_custom_layer() {
    log_section "Custom Layer"
    
    # Your checks here
    if custom_check; then
        log_pass "Custom check passed"
    else
        log_fail "Custom check failed"
        return 1
    fi
}

# Register layer
register_verification "L5" "verify_custom_layer"
EOF

# 2. Add to verification suite
# Edit MetaGuildNet/scripts/verify/verify_all.sh
# Add: source "${SCRIPT_DIR}/custom_layer.sh"

# 3. Test
make meta-verify-custom
```

### Adding New Examples

```bash
# 1. Create example directory
mkdir -p MetaGuildNet/examples/advanced/my-example

# 2. Add README
cat > MetaGuildNet/examples/advanced/my-example/README.md << 'EOF'
# My Example

Description of the example.

## Prerequisites

- Requirement 1
- Requirement 2

## Usage

```bash
bash run.sh
```

## Expected Output

What should happen.
EOF

# 3. Add runnable script
cat > MetaGuildNet/examples/advanced/my-example/run.sh << 'EOF'
#!/bin/bash
# Example implementation
EOF

chmod +x MetaGuildNet/examples/advanced/my-example/run.sh

# 4. Test the example
bash MetaGuildNet/examples/advanced/my-example/run.sh
```

### Extending Test Framework

```bash
# Add custom assertions
cat >> MetaGuildNet/tests/lib/test_framework.sh << 'EOF'

# Custom assertion
assert_workspace_ready() {
    local workspace_id="$1"
    local timeout="${2:-120}"
    
    if ! wait_for_workspace_ready "$workspace_id" "$timeout"; then
        test_fail "Workspace not ready within ${timeout}s"
        return 1
    fi
    
    test_pass "Workspace ready"
}
EOF
```

## FAQ

### Q: Should my change go in MetaGuildNet/ or upstream?

**A:** If it's:
- Setup automation → MetaGuildNet/
- Verification tooling → MetaGuildNet/
- Fork-specific feature → MetaGuildNet/
- Bug fix in GuildNet → Consider upstream PR
- Core feature → Consider upstream PR

### Q: How do I sync with upstream?

**A:** See [UPSTREAM_SYNC.md](UPSTREAM_SYNC.md)

### Q: Can I modify files outside MetaGuildNet/?

**A:** Only for:
- Updating Makefile to add MetaGuildNet targets
- Updating root AGENTS.md to reference MetaGuildNet
- Bug fixes that should go upstream

### Q: How do I test my changes?

**A:** See [VERIFICATION.md](VERIFICATION.md) for comprehensive testing procedures

### Q: Where can I find examples of MetaGuildNet usage?

**A:** Browse [MetaGuildNet/examples/](../examples/) for real-world usage patterns and [SETUP.md](SETUP.md) for complete setup workflows

### Q: How do I understand the overall architecture?

**A:** Read [ARCHITECTURE.md](ARCHITECTURE.md) for fork-specific design decisions and [../../architecture.md](../../architecture.md) for upstream GuildNet architecture

---

## References

### GuildNet Repository
- [Core Architecture](../../architecture.md) - Upstream design principles
- [Host App Implementation](../../cmd/hostapp/main.go) - Main application structure
- [Operator Code](../../internal/operator/) - Kubernetes operator implementation
- [Makefile](../../Makefile) - Build targets and automation
- [Go Module](../../go.mod) - Dependency management

### MetaGuildNet Extensions
- [Architecture Guide](ARCHITECTURE.md) - Fork-specific design decisions
- [Setup Guide](SETUP.md) - Installation and configuration procedures
- [Verification Guide](VERIFICATION.md) - Testing and health check procedures
- [Upstream Sync](UPSTREAM_SYNC.md) - Synchronization with upstream GuildNet

### External Resources
- [Conventional Commits](https://www.conventionalcommits.org/) - Commit message standards
- [Semantic Versioning](https://semver.org/) - Version numbering guidelines
- [GitHub Flow](https://docs.github.com/en/get-started/quickstart/github-flow) - Branching workflow
- [Go Testing](https://golang.org/pkg/testing/) - Go test framework documentation

**Thank you for contributing to MetaGuildNet! Your efforts help make GuildNet better for everyone.**

