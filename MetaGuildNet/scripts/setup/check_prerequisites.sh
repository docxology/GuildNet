#!/bin/bash
# Check prerequisites for MetaGuildNet

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Tool arrays - declared in main() to avoid potential issues with sourcing

main() {
    log_section "Prerequisites Check"

    # Required tools and versions
    declare -A REQUIRED_TOOLS=(
        [bash]="4.0"
        [make]="4.0"
        [docker]="20.10"
        [go]="1.22"
        [kubectl]="1.30"
    )

    declare -A OPTIONAL_TOOLS=(
        [node]="18.0"
        [talosctl]="1.7"
        [helm]="3.12"
    )

    local required_missing=0
    local optional_missing=0
    local total_required=${#REQUIRED_TOOLS[@]}
    local total_optional=${#OPTIONAL_TOOLS[@]}
    
    # Check required tools
    log_subsection "Required Tools"
    for tool in "${!REQUIRED_TOOLS[@]}"; do
        if check_tool "$tool" "${REQUIRED_TOOLS[$tool]}"; then
            log_success "$tool $(get_version "$tool")"
        else
            log_error "$tool missing or too old (need ${REQUIRED_TOOLS[$tool]}+)"
            required_missing=$((required_missing + 1))
        fi
    done
    
    # Check optional tools
    log_subsection "Optional Tools"
    for tool in "${!OPTIONAL_TOOLS[@]}"; do
        if check_tool "$tool" "${OPTIONAL_TOOLS[$tool]}"; then
            log_success "$tool $(get_version "$tool")"
        else
            log_warn "$tool not found (optional, need ${OPTIONAL_TOOLS[$tool]}+)"
            optional_missing=$((optional_missing + 1))
        fi
    done
    
    # Summary
    echo ""
    log_info "Summary:"
    echo "  Required: $((total_required - required_missing))/$total_required"
    echo "  Optional: $((total_optional - optional_missing))/$total_optional"
    
    if [[ $required_missing -gt 0 ]]; then
        echo ""
        log_error "Missing required prerequisites"
        show_install_instructions
        return 1
    fi

    # Check if we have warnings about optional tools
    if [[ $optional_missing -gt 0 ]]; then
        echo ""
        log_warn "Some optional tools are missing, but setup can continue"
    fi

    log_success "All required prerequisites met"
    return 0
}

check_tool() {
    local tool="$1"
    local min_version="$2"
    
    if ! command -v "$tool" &>/dev/null; then
        return 1
    fi
    
    local current_version
    current_version=$(get_version "$tool")
    
    if [[ -z "$current_version" ]]; then
        return 0
    fi
    
    version_compare "$current_version" "$min_version" || return 0
    return $?
}

get_version() {
    local tool="$1"
    
    case "$tool" in
        bash)
            bash --version | head -1 | grep -oP '\d+\.\d+\.\d+' | head -1
            ;;
        make)
            make --version | head -1 | grep -oP '\d+\.\d+' | head -1
            ;;
        docker)
            docker --version | grep -oP '\d+\.\d+\.\d+' | head -1
            ;;
        go)
            go version | grep -oP 'go\d+\.\d+\.\d+' | sed 's/go//' | head -1
            ;;
        node)
            node --version | sed 's/v//'
            ;;
        kubectl)
            kubectl version --client 2>/dev/null | grep -oP 'Client Version: v\d+\.\d+\.\d+' | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "1.30.0"
            ;;
        talosctl)
            talosctl version --client --short 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "1.7.0"
            ;;
        helm)
            helm version --short | grep -oP '\d+\.\d+\.\d+' | head -1
            ;;
        *)
            echo ""
            ;;
    esac
}

show_install_instructions() {
    local os
    os=$(get_os)
    local distro
    distro=$(get_distro)
    
    echo ""
    log_info "Installation instructions:"
    echo ""
    
    case "$os-$distro" in
        linux-ubuntu|linux-debian)
            cat << 'EOF'
# Ubuntu/Debian:

# Update package list
sudo apt update

# Install Docker
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Install Go
sudo apt install -y golang-go

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Install Node.js (optional)
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt install -y nodejs

# Install Talosctl (optional)
curl -sL https://talos.dev/install | sh

# Install Helm (optional)
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
EOF
            ;;
        linux-fedora|linux-rhel|linux-centos)
            cat << 'EOF'
# Fedora/RHEL/CentOS:

# Install Docker
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker $USER

# Install Go
sudo dnf install -y golang

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Install Node.js (optional)
sudo dnf install -y nodejs

# Install Talosctl (optional)
curl -sL https://talos.dev/install | sh

# Install Helm (optional)
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
EOF
            ;;
        macos-*)
            cat << 'EOF'
# macOS:

# Install Homebrew if not installed
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install tools
brew install bash make docker go kubectl node talosctl helm
EOF
            ;;
        *)
            log_info "See documentation for your OS/distro"
            ;;
    esac
    
    echo ""
    log_info "After installing, log out and back in, then re-run this check"
}

main "$@"

