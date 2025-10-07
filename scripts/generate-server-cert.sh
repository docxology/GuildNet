#!/usr/bin/env sh
# Generate a server certificate signed by certs/ca.crt using certs/server.key and certs/server-san.cnf
# Output: certs/server.crt (replaces existing)
# Usage: scripts/generate-server-cert.sh [-H "localhost,127.0.0.1,::1"] [-f]
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT/certs"
HOSTS=""
FORCE=0
while getopts ":H:f" opt; do
  case "$opt" in
    H) HOSTS="$OPTARG" ;;
    f) FORCE=1 ;;
    *) echo "Usage: $0 [-H hostnames] [-f]" >&2; exit 2 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "Error: $1 is required" >&2; exit 1; }; }
need openssl

CA_CRT="$OUT_DIR/ca.crt"
CA_KEY="$OUT_DIR/ca.key"
SRV_KEY="$OUT_DIR/server.key"
SRV_CRT="$OUT_DIR/server.crt"
CFG="$OUT_DIR/server-san.cnf"

[ -f "$CA_CRT" ] || { echo "Missing $CA_CRT" >&2; exit 1; }
[ -f "$CA_KEY" ] || { echo "Missing $CA_KEY" >&2; exit 1; }
[ -f "$SRV_KEY" ] || { echo "Missing $SRV_KEY" >&2; exit 1; }

if [ -n "$HOSTS" ]; then
  # rewrite SAN config with provided hosts
  i_dns=0; i_ip=0; ALT_NAMES=""
  OLDIFS="$IFS"; IFS=','
  for h in $HOSTS; do
    h=$(printf "%s" "$h" | tr -d ' ')
    [ -z "$h" ] && continue
    case "$h" in
      *:*) i_ip=$((i_ip+1)); ALT_NAMES="$ALT_NAMES\nIP.$i_ip = $h" ;;
      *[!0-9.]* ) i_dns=$((i_dns+1)); ALT_NAMES="$ALT_NAMES\nDNS.$i_dns = $h" ;;
      *) i_ip=$((i_ip+1)); ALT_NAMES="$ALT_NAMES\nIP.$i_ip = $h" ;;
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
fi

if [ -f "$SRV_CRT" ] && [ $FORCE -eq 0 ]; then
  echo "Server cert already exists: $SRV_CRT (use -f to overwrite)" >&2
  exit 0
fi

TMP_CSR="$OUT_DIR/server.csr"
openssl req -new -key "$SRV_KEY" -out "$TMP_CSR" -config "$CFG" >/dev/null 2>&1
openssl x509 -req -in "$TMP_CSR" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial -out "$SRV_CRT" -days 825 -sha256 -extensions v3_req -extfile "$CFG" >/dev/null 2>&1
rm -f "$TMP_CSR" "$OUT_DIR/ca.srl"
chmod 644 "$SRV_CRT"
echo "Server cert written: $SRV_CRT"
