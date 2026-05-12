#!/usr/bin/env bash
# deploy-app.sh — Cross-compile oilfield Go binaries and deploy to runtime nodes.
# Run from repo root on the control node (N4) or local machine with SSH access.
# Usage: ./infra/deploy/deploy-app.sh [n1|n2|n3|all]  (default: all)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/infra/.env"

TARGET="${1:-all}"
NODES=()
case "$TARGET" in
  all) NODES=(n1 n2 n3) ;;
  n1|n2|n3) NODES=("$TARGET") ;;
  *) echo "Usage: $0 [n1|n2|n3|all]" >&2; exit 1 ;;
esac

GOARCH=amd64 GOOS=linux

echo "==> Building oilfield-api..."
(cd "$REPO_ROOT/app/backend" && GOARCH=amd64 GOOS=linux go build -o /tmp/oilfield-api ./cmd/api)

echo "==> Building oilfield-scraper..."
(cd "$REPO_ROOT/app/backend" && GOARCH=amd64 GOOS=linux go build -o /tmp/oilfield-scraper ./cmd/scraper)

# Load IPs from state files — use IPs directly to avoid DNS propagation race.
declare -A NODE_IPS
NODE_IPS[n1]="$(cat "$REPO_ROOT/infra/state/hetzner.ip"  2>/dev/null || echo "")"
NODE_IPS[n2]="$(cat "$REPO_ROOT/infra/state/linode.ip"   2>/dev/null || echo "")"
NODE_IPS[n3]="$(cat "$REPO_ROOT/infra/state/ionos.ip"    2>/dev/null || echo "")"

for NODE in "${NODES[@]}"; do
  NODE_IP="${NODE_IPS[$NODE]}"
  [ -n "$NODE_IP" ] || { echo "  WARN: No IP found for $NODE — skipping" >&2; continue; }
  HOST="deploy@$NODE_IP"
  echo ""
  echo "==> Deploying to $NODE ($HOST)..."

  scp -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" \
    /tmp/oilfield-api \
    /tmp/oilfield-scraper \
    "${HOST}:/tmp/"

  ssh -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" "$HOST" bash -s <<'REMOTE'
    set -euo pipefail
    sudo mv /tmp/oilfield-api /opt/oilfield/bin/oilfield-api
    sudo mv /tmp/oilfield-scraper /opt/oilfield/bin/oilfield-scraper
    sudo chmod +x /opt/oilfield/bin/oilfield-api /opt/oilfield/bin/oilfield-scraper
    sudo systemctl daemon-reload
    sudo systemctl enable --now oilfield-api
    sudo systemctl restart oilfield-api
    # Enable + start the scraper timer (idempotent)
    sudo systemctl enable --now oilfield-scraper.timer
    # Run the scraper immediately to populate etcd without waiting 60s for the timer
    sudo systemctl start oilfield-scraper.service || true
    echo "  oilfield-api restarted and scraper timer enabled on $(hostname)"
REMOTE

  echo "  $NODE done."
done

rm -f /tmp/oilfield-api /tmp/oilfield-scraper
echo ""
echo "==> Deploy complete: ${NODES[*]}"
