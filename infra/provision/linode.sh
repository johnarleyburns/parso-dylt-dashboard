#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n2"
LABEL="oilfield-n2"
REGION="us-lax"           # Los Angeles — US West Coast
TYPE="g6-standard-1"      # 1 vCPU, 2GB RAM, 50GB SSD
IMAGE="linode/ubuntu24.04"

log() { echo "[linode] $*" >&2; }
die() { echo "[linode] ERROR: $*" >&2; exit 1; }

BASE_URL="https://api.linode.com/v4"
AUTH_HEADER="Authorization: Bearer $LINODE_TOKEN"

SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

# Create Linode instance
log "Creating instance '$LABEL' in $REGION..."
RESPONSE=$(curl -s -X POST "$BASE_URL/linode/instances" \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d "{
    \"region\": \"$REGION\",
    \"type\": \"$TYPE\",
    \"image\": \"$IMAGE\",
    \"label\": \"$LABEL\",
    \"root_pass\": \"$(openssl rand -base64 24)\",
    \"authorized_keys\": [\"$SSH_PUBLIC_KEY\"],
    \"booted\": true
  }")

LINODE_ID=$(echo "$RESPONSE" | jq -r '.id // empty')
[ -n "$LINODE_ID" ] || die "Failed to create instance: $(echo "$RESPONSE" | jq -r '.errors[0].reason // "unknown error"')"
log "Instance created (id=$LINODE_ID) — waiting for 'running' status..."

# Poll until running
for i in $(seq 1 72); do
  STATUS=$(curl -s \
    -H "$AUTH_HEADER" \
    "$BASE_URL/linode/instances/$LINODE_ID" \
    | jq -r '.status')
  if [ "$STATUS" = "running" ]; then
    break
  fi
  [ "$i" -lt 72 ] || die "Timed out waiting for instance to reach 'running' state"
  log "  status=$STATUS — retrying in 5s..."
  sleep 5
done

# Get public IP (first IPv4 address)
NODE_IP=$(curl -s \
  -H "$AUTH_HEADER" \
  "$BASE_URL/linode/instances/$LINODE_ID/ips" \
  | jq -r '.ipv4.public[0].address // empty')
[ -n "$NODE_IP" ] || die "Could not retrieve instance IP"

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/linode.ip"
log "Instance running at $NODE_IP — written to infra/state/linode.ip"

# Wait for SSH to be ready (Linode may take a moment after 'running' status)
log "Waiting for SSH on $NODE_IP..."
for i in $(seq 1 30); do
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -i "$SSH_PRIVATE_KEY_PATH" "root@$NODE_IP" true 2>/dev/null && break
  [ "$i" -lt 30 ] || die "SSH not ready on $NODE_IP after 150s"
  log "  SSH not ready — retrying in 5s ($i/30)..."
  sleep 5
done

# Run base bootstrap over SSH
log "Running base bootstrap on $NODE_IP..."
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i "$SSH_PRIVATE_KEY_PATH" \
  "root@$NODE_IP" "NODE_NAME=n2 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N2/Linode is up at $NODE_IP"
