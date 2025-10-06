#!/usr/bin/env sh
# Generate a single self-signed TLS cert for both backend and Vite dev server.
# Files are written to ./certs and can be committed.
# Usage:
#   scripts/generate-certs.sh [-d output_dir] [-H hostnames] [-f]
# Options:
#   -d DIR       Output directory (default: "./certs")
#   -H HOSTS     Comma-separated SANs (default: "localhost,127.0.0.1,::1")
#   -f           Overwrite existing certs/keys
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT/certs"
HOSTS="localhost,127.0.0.1,::1"
FORCE=0

while getopts ":d:H:f" opt; do
  case "$opt" in
    d) OUT_DIR="$OPTARG" ;;
    H) HOSTS="$OPTARG" ;;
    f) FORCE=1 ;;
    *) echo "Usage: $0 [-d output_dir] [-H hostnames] [-f]" >&2; exit 2 ;;
  esac
done

log() { printf "%s | %s\n" "$(date -Iseconds)" "$*"; }
err() { printf "Error: %s\n" "$*" >&2; }

need() { command -v "$1" >/dev/null 2>&1 || { err "$1 is required"; exit 1; }; }

need openssl

mkdir -p "$OUT_DIR"
chmod 700 "$OUT_DIR"

CRT="$OUT_DIR/dev.crt"
KEY="$OUT_DIR/dev.key"
CFG="$OUT_DIR/dev-sans.cnf"

if [ -f "$CRT" ] && [ -f "$KEY" ] && [ $FORCE -eq 0 ]; then
  log "Dev cert already exists: $CRT"
  echo "Use -f to overwrite."
  exit 0
fi

# Build SAN config
i_dns=0; i_ip=0; ALT_NAMES=""
OLDIFS="$IFS"; IFS=','
for h in $HOSTS; do
  h=$(printf "%s" "$h" | tr -d ' ') || true
  [ -z "$h" ] && continue
  case "$h" in
    *:*) i_ip=$((i_ip+1)); ALT_NAMES="${ALT_NAMES}
IP.$i_ip = $h" ;;
    *[!0-9.]* ) i_dns=$((i_dns+1)); ALT_NAMES="${ALT_NAMES}
DNS.$i_dns = $h" ;;
    *) i_ip=$((i_ip+1)); ALT_NAMES="${ALT_NAMES}
IP.$i_ip = $h" ;;
  esac
done
IFS="$OLDIFS"
cat > "$CFG" <<EOF
[ req ]
distinguished_name = dn
req_extensions = v3_req
prompt = no

[ dn ]
CN = localhost

[ v3_req ]
subjectAltName = @alt_names

[ alt_names ]
# Auto-generated SANs$ALT_NAMES
EOF

log "Generating self-signed dev cert with SANs: $HOSTS"
openssl genrsa -out "$KEY" 2048 >/dev/null 2>&1
openssl req -new -key "$KEY" -out "$OUT_DIR/dev.csr" -config "$CFG" >/dev/null 2>&1
openssl x509 -req -in "$OUT_DIR/dev.csr" -signkey "$KEY" -out "$CRT" -days 825 -sha256 -extensions v3_req -extfile "$CFG" >/dev/null 2>&1
rm -f "$OUT_DIR/dev.csr" "$CFG"
chmod 600 "$KEY"
chmod 644 "$CRT"

log "Dev cert written: $CRT"
echo "Use this for both backend and UI dev."
