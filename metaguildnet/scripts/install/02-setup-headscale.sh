#!/bin/bash
# Setup Headscale for GuildNet
# Wraps main GuildNet Headscale setup scripts

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
GUILDNET_SCRIPTS="$PROJECT_ROOT/scripts"

echo "Setting up Headscale..."

# Run main GuildNet Headscale setup
if [ -f "$GUILDNET_SCRIPTS/headscale-run.sh" ]; then
    bash "$GUILDNET_SCRIPTS/headscale-run.sh" up
    bash "$GUILDNET_SCRIPTS/headscale-bootstrap.sh"
else
    echo "⚠ GuildNet Headscale scripts not found, skipping..."
fi

echo "✓ Headscale setup complete"

