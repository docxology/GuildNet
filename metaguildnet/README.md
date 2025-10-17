# MetaGuildNet

**Fork-Specific Enhancements and Documentation for GuildNet**

## Overview

MetaGuildNet is a structured enhancement layer for the GuildNet project, providing:

- **Comprehensive Documentation**: Technical guides, architecture notes, and best practices
- **Convenience Tooling**: Wrappers, utilities, and automation scripts
- **Verification Suite**: Integration tests, end-to-end tests, and health checks
- **Examples**: Real-world usage patterns and scenarios

All fork-specific modifications, enhancements, and tooling live in this directory, maintaining clean separation from upstream GuildNet while enabling seamless synchronization.

## Philosophy

> "Prefer modularity and composability over monoliths. Each component should do one thing well and be replaceable."

MetaGuildNet follows these principles:

- **Default-First**: Everything works out of the box with sensible defaults
- **Environment-Driven**: Customize via environment variables when needed
- **Production-Ready**: No dev/prod split—this is a developer tool
- **Upstream-Compatible**: Designed to merge cleanly with upstream changes

## Directory Structure

```
MetaGuildNet/
├── README.md                    # This file
├── docs/                        # Technical documentation
│   ├── ARCHITECTURE.md          # Fork-specific architecture decisions
│   ├── CONTRIBUTING.md          # Contribution guidelines
│   ├── SETUP.md                 # Detailed setup procedures
│   ├── VERIFICATION.md          # Verification and testing guide
│   └── UPSTREAM_SYNC.md         # Syncing with upstream GuildNet
├── scripts/                     # Convenience scripts and wrappers
│   ├── setup/                   # Automated setup scripts
│   ├── verify/                  # Verification and health check scripts
│   └── utils/                   # General utilities
├── tests/                       # Fork-specific test suites
│   ├── integration/             # Integration tests
│   └── e2e/                     # End-to-end tests
└── examples/                    # Usage examples and templates
    ├── basic/                   # Basic usage patterns
    └── advanced/                # Advanced scenarios
```

## Quick Start

### First-Time Setup

```bash
# 1. Run the unified setup wizard
make -C MetaGuildNet setup-wizard

# 2. Verify installation
make -C MetaGuildNet verify-all

# 3. Run a basic example
make -C MetaGuildNet example-basic
```

### Daily Workflow

```bash
# Check health of all components
make -C MetaGuildNet health-check

# Run verification suite
make -C MetaGuildNet verify

# Sync with upstream (when needed)
make -C MetaGuildNet sync-upstream
```

## Integration with GuildNet

MetaGuildNet seamlessly integrates with the base GuildNet Makefile:

```bash
# Standard GuildNet commands work as usual
make setup              # Base setup
make run                # Run hostapp

# MetaGuildNet enhancements
make meta-setup         # Enhanced setup with verification
make meta-verify        # Comprehensive health checks
make meta-docs          # Generate/serve documentation
```

## Key Features

### 1. Automated Setup Orchestration

The setup wizard automates the entire stack:

- **Network Layer**: Headscale + Tailscale with automatic route configuration
- **Cluster Layer**: Talos Kubernetes with add-ons (MetalLB, CRDs, RethinkDB)
- **Application Layer**: Host app with embedded operator
- **Verification**: End-to-end health checks and connectivity tests

### 2. Comprehensive Verification

Multi-layer verification ensures everything works:

- **Network**: Tailnet connectivity, route propagation
- **Cluster**: Kubernetes API, node health, storage
- **Database**: RethinkDB connectivity and schema
- **Application**: Host app APIs, workspace creation, proxy

### 3. Developer Experience

Convenience wrappers reduce friction:

- **One-Command Setup**: `make meta-setup` orchestrates everything
- **Smart Defaults**: Environment variables with sensible fallbacks
- **Error Recovery**: Automatic retry with exponential backoff
- **Diagnostic Tools**: Structured troubleshooting commands

### 4. Documentation-Driven

Living documentation that stays current:

- **Architecture Decision Records** (ADRs)
- **Troubleshooting Playbooks**
- **Integration Examples**
- **API Documentation**

## Environment Configuration

MetaGuildNet respects all GuildNet environment variables and adds:

```bash
# MetaGuildNet-specific configuration
METAGN_SETUP_MODE=auto           # auto|interactive|minimal
METAGN_VERIFY_TIMEOUT=300        # Verification timeout (seconds)
METAGN_AUTO_APPROVE_ROUTES=true  # Auto-approve Tailscale routes
METAGN_LOG_LEVEL=info            # debug|info|warn|error
```

## Documentation

- **[Setup Guide](docs/SETUP.md)**: Detailed setup procedures and troubleshooting
- **[Architecture](docs/ARCHITECTURE.md)**: Fork-specific design decisions
- **[Verification](docs/VERIFICATION.md)**: Testing and health check procedures
- **[Contributing](docs/CONTRIBUTING.md)**: How to contribute to this fork
- **[Upstream Sync](docs/UPSTREAM_SYNC.md)**: Maintaining sync with upstream GuildNet

## Testing

```bash
# Run all tests
make -C MetaGuildNet test-all

# Run specific test suites
make -C MetaGuildNet test-integration
make -C MetaGuildNet test-e2e

# Run with coverage
make -C MetaGuildNet test-coverage
```

## Examples

Browse the `examples/` directory for:

- **Basic**: Single-node setup, workspace creation, proxy access
- **Advanced**: Multi-node clusters, custom images, persistent storage

Run examples directly:

```bash
# Basic workspace example
bash MetaGuildNet/examples/basic/create-workspace.sh

# Advanced multi-user scenario
bash MetaGuildNet/examples/advanced/multi-user-setup.sh
```

## Contributing

We welcome contributions! See [CONTRIBUTING.md](docs/CONTRIBUTING.md) for:

- Code style and standards
- Testing requirements
- Pull request process
- Development workflow

## Upstream Relationship

This fork maintains compatibility with upstream GuildNet:

- **Sync Cadence**: Weekly upstream pulls (or as needed)
- **Merge Strategy**: Rebase onto upstream main
- **Feature Isolation**: Fork features in MetaGuildNet/ only
- **Upstream Contributions**: Extract generic improvements for PRs

See [UPSTREAM_SYNC.md](docs/UPSTREAM_SYNC.md) for procedures.

## Troubleshooting

Common issues and solutions:

### Setup Fails

```bash
# Check prerequisites
make -C MetaGuildNet check-prereqs

# View detailed logs
METAGN_LOG_LEVEL=debug make -C MetaGuildNet setup-wizard
```

### Connectivity Issues

```bash
# Diagnose network
make -C MetaGuildNet diag-network

# Diagnose cluster
make -C MetaGuildNet diag-cluster
```

### Verification Failures

```bash
# Run incremental verification
make -C MetaGuildNet verify-step-by-step

# Export diagnostic bundle
make -C MetaGuildNet export-diagnostics
```

See [docs/VERIFICATION.md](docs/VERIFICATION.md) for comprehensive troubleshooting.

## License

This fork maintains the same license as upstream GuildNet. See [../LICENSE](../LICENSE).

## Support and Community

- **Issues**: Open issues in this fork's repository
- **Discussions**: Use GitHub Discussions for questions
- **Upstream Issues**: File bugs affecting base GuildNet upstream

## Changelog

### v1.0.0 (Current)

- Initial MetaGuildNet structure
- Automated setup wizard
- Comprehensive verification suite
- Integration test framework
- Documentation overhaul
- Example scenarios

---

**Philosophy**: Do or do not, there is no try. Everything here works by default, is production-ready, and can be customized as needed.

