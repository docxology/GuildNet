Any new features or configurations must work by default, but can be customized via environment variables or other configurations as appropriate. There is no dev flow for this project, only a production flow as this is a developer tool. Everything should work out of the box with sensible defaults.

Prefer modularity and composability over monoliths. Each component should do one thing well and be replaceable.

Always ensure everything the user asks to be done is actually done, even if it requires multiple steps, complex logic or terminal commands. If the user asks for a file to be created, create it with the correct content. If the user asks for code to be modified, modify it correctly. If a file needs to be deleted, delete it. Never leave the user with an incomplete task or half-finished code.

Do or do not, there is no try.

## MetaGuildNet Extensions

This repository includes **MetaGuildNet**, a comprehensive enhancement layer providing:

- **Automated Setup**: One-command setup wizard for the full stack
- **Verification Framework**: Multi-layer health checks and diagnostics
- **Testing Infrastructure**: Integration and end-to-end test suites
- **Documentation**: Comprehensive guides for setup, verification, and contribution
- **Examples**: Real-world usage patterns and scenarios

All fork-specific enhancements live in `MetaGuildNet/` to maintain clean separation from upstream GuildNet.

### Quick Start with MetaGuildNet

```bash
# Automated full setup
make meta-setup

# Comprehensive verification
make meta-verify

# Create example workspace
bash MetaGuildNet/examples/basic/create-workspace.sh
```

### MetaGuildNet Resources

- **Main Documentation**: [MetaGuildNet/README.md](MetaGuildNet/README.md)
- **Setup Guide**: [MetaGuildNet/docs/SETUP.md](MetaGuildNet/docs/SETUP.md)
- **Verification Guide**: [MetaGuildNet/docs/VERIFICATION.md](MetaGuildNet/docs/VERIFICATION.md)
- **Architecture**: [MetaGuildNet/docs/ARCHITECTURE.md](MetaGuildNet/docs/ARCHITECTURE.md)
- **Contributing**: [MetaGuildNet/docs/CONTRIBUTING.md](MetaGuildNet/docs/CONTRIBUTING.md)

### MetaGuildNet Principles

- **Default-First**: Everything works without configuration
- **Production-Ready**: No dev/prod distinction
- **Composable**: Each component is independent and replaceable
- **Upstream-Compatible**: Clean merge path with GuildNet
- **Well-Tested**: Comprehensive test coverage

See [MetaGuildNet/README.md](MetaGuildNet/README.md) for complete documentation.