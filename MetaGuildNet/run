#!/bin/bash
# MetaGuildNet Runner Wrapper
# Simple wrapper to run the Python script

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check if Python 3 is available
if ! command -v python3 &>/dev/null; then
    echo "Error: python3 is required but not installed"
    exit 1
fi

# Run the Python script with all arguments
exec python3 "$SCRIPT_DIR/run.py" "$@"
