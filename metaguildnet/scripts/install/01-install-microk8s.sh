#!/bin/bash
# Install and configure microk8s
# This wraps the main GuildNet microk8s setup script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
GUILDNET_SCRIPTS="$PROJECT_ROOT/scripts"

echo "Installing microk8s..."

# Check if microk8s is already installed
if command -v microk8s &> /dev/null; then
    echo "✓ microk8s is already installed"
    microk8s status --wait-ready
else
    # Call main GuildNet microk8s setup script
    if [ -f "$GUILDNET_SCRIPTS/microk8s-setup.sh" ]; then
        bash "$GUILDNET_SCRIPTS/microk8s-setup.sh"
    else
        echo "Installing microk8s via snap..."
        sudo snap install microk8s --classic --channel=1.28/stable
        
        # Add user to microk8s group
        sudo usermod -a -G microk8s "$USER"
        
        # Wait for microk8s to be ready
        sudo microk8s status --wait-ready
        
        # Enable addons
        sudo microk8s enable dns storage
        
        # Generate kubeconfig
        mkdir -p "$HOME/.guildnet"
        sudo microk8s config > "$HOME/.guildnet/kubeconfig"
        chmod 600 "$HOME/.guildnet/kubeconfig"
    fi
fi

echo "✓ microk8s installation complete"
echo "  Kubeconfig: $HOME/.guildnet/kubeconfig"

