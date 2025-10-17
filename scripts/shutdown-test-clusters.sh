#!/usr/bin/env bash
set -euo pipefail

# shutdown-test-clusters.sh
# Find clusters that look like test/dev clusters (generated UUID names, names starting with 'test-' or '<nil>')
# and delete them via the Host App API at https://127.0.0.1:8090.
# Usage: ./scripts/shutdown-test-clusters.sh [--yes]

HOST=${HOST:-https://127.0.0.1:8090}
CONFIRM=${1:-}

need(){ command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; } }
need curl
need jq

echo "Fetching clusters from ${HOST}..."
CLUSTERS_JSON=$(curl -skS -X GET "${HOST}/api/deploy/clusters")

echo "Parsing candidate clusters..."
mapfile -t CANDIDS < <(echo "$CLUSTERS_JSON" | jq -r '.[] | select(.name=="<nil>" or (.name|test("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")) or (.name|test("^test-"))) | "\(.id)\t\(.name)\t\(.state)"')

if [ ${#CANDIDS[@]} -eq 0 ]; then
  echo "No test-like clusters found. Nothing to do."
  exit 0
fi

echo "Found ${#CANDIDS[@]} candidate clusters to delete:" 
for c in "${CANDIDS[@]}"; do
  echo "  $c"
done

if [ "$CONFIRM" != "--yes" ]; then
  echo
  echo "If you are sure, re-run with: $0 --yes"
  exit 0
fi

echo
echo "Deleting candidate clusters..."
for c in "${CANDIDS[@]}"; do
  ID=$(echo "$c" | awk -F"\t" '{print $1}')
  NAME=$(echo "$c" | awk -F"\t" '{print $2}')
  echo "Deleting cluster: $ID ($NAME) ..."
  # DELETE is the documented way to remove cluster record
  if curl -skS -X DELETE "${HOST}/api/deploy/clusters/${ID}" >/dev/null 2>&1; then
    echo "  Deleted: $ID"
  else
    echo "  Failed to delete: $ID (see server logs)"
  fi
done

echo "Done."
