#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n4"
SERVER_TITLE="oilfield-n4"
PLAN="DEV-1xCPU-2GB"
ZONE="$UPCLOUD_ZONE"   # us-chi1 (Chicago)
OS_TEMPLATE="01000000-0000-4000-8000-000030240200"  # Ubuntu 24.04 LTS

log() { echo "[upcloud] $*" >&2; }
die() { echo "[upcloud] ERROR: $*" >&2; exit 1; }

AUTH_HEADER="Authorization: Bearer $UPCLOUD_API_TOKEN"
BASE_URL="https://api.upcloud.com/1.3"

# Verify Ubuntu 24.04 template UUID for this zone (look it up dynamically)
log "Resolving Ubuntu 24.04 OS template for zone $ZONE..."
TEMPLATE_UUID=$(curl -s \
  -H "$AUTH_HEADER" \
  -H "Accept: application/json" \
  "$BASE_URL/storage/template" \
  | jq -r '[.storages.storage[] | select(.title | test("Ubuntu Server 24.04"))] | first | .uuid // empty')
[ -n "$TEMPLATE_UUID" ] || {
  log "Dynamic lookup failed — using pinned Ubuntu 24.04 UUID"
  TEMPLATE_UUID="$OS_TEMPLATE"
}
log "Template UUID: $TEMPLATE_UUID"

SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

# Create server
log "Creating server '$SERVER_TITLE' in $ZONE..."
RESPONSE=$(curl -s -X POST \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d "{
    \"server\": {
      \"zone\": \"$ZONE\",
      \"title\": \"$SERVER_TITLE\",
      \"hostname\": \"$SERVER_TITLE\",
      \"plan\": \"$PLAN\",
      \"metadata\": \"yes\",
      \"storage_devices\": {
        \"storage_device\": [{
          \"action\": \"clone\",
          \"storage\": \"$TEMPLATE_UUID\",
          \"title\": \"$SERVER_TITLE-disk\",
          \"size\": 30
        }]
      },
      \"login_user\": {
        \"username\": \"root\",
        \"ssh_keys\": {
          \"ssh_key\": [\"$SSH_PUBLIC_KEY\"]
        }
      }
    }
  }" \
  "$BASE_URL/server")

SERVER_UUID=$(echo "$RESPONSE" | jq -r '.server.uuid // empty')
[ -n "$SERVER_UUID" ] || die "Failed to create server: $(echo "$RESPONSE" | jq -r '.errors.error[0].error_message // "unknown error"')"
log "Server created (uuid=$SERVER_UUID) — waiting for 'started' state..."

# Poll until started
for i in $(seq 1 72); do
  STATE=$(curl -s \
    -H "$AUTH_HEADER" \
    -H "Accept: application/json" \
    "$BASE_URL/server/$SERVER_UUID" \
    | jq -r '.server.state')
  if [ "$STATE" = "started" ]; then
    break
  fi
  [ "$i" -lt 72 ] || die "Timed out waiting for server to reach 'started' state"
  log "  state=$STATE — retrying in 5s..."
  sleep 5
done

# Get public IP
NODE_IP=$(curl -s \
  -H "$AUTH_HEADER" \
  -H "Accept: application/json" \
  "$BASE_URL/server/$SERVER_UUID" \
  | jq -r '.server.ip_addresses.ip_address[] | select(.access == "public" and .family == "IPv4") | .address' \
  | head -1)
[ -n "$NODE_IP" ] || die "Could not retrieve server IP"

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/upcloud.ip"
log "Server running at $NODE_IP — written to infra/state/upcloud.ip"

# Wait for SSH to be ready
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
  "root@$NODE_IP" "NODE_NAME=n4 NODE_ROLE=control DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N4/UpCloud is up at $NODE_IP"
