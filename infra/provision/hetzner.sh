#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n1"
SERVER_NAME="oilfield-n1"
SERVER_TYPE="cpx11"
LOCATION="ash"   # Ashburn, VA
IMAGE="ubuntu-24.04"

log() { echo "[hetzner] $*" >&2; }
die() { echo "[hetzner] ERROR: $*" >&2; exit 1; }

# Upload SSH public key (idempotent — skip if already exists)
SSH_KEY_NAME="oilfield-deploy"
SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

log "Checking for existing SSH key '$SSH_KEY_NAME'..."
EXISTING_KEY_ID=$(curl -s \
  -H "Authorization: Bearer $HETZNER_API_TOKEN" \
  "https://api.hetzner.cloud/v1/ssh_keys?name=$SSH_KEY_NAME" \
  | jq -r '.ssh_keys[0].id // empty')

if [ -n "$EXISTING_KEY_ID" ]; then
  log "SSH key already exists (id=$EXISTING_KEY_ID)"
  SSH_KEY_ID="$EXISTING_KEY_ID"
else
  log "Uploading SSH key..."
  SSH_KEY_ID=$(curl -s -X POST \
    -H "Authorization: Bearer $HETZNER_API_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$SSH_KEY_NAME\",\"public_key\":\"$SSH_PUBLIC_KEY\"}" \
    "https://api.hetzner.cloud/v1/ssh_keys" \
    | jq -r '.ssh_key.id // empty')
  [ -n "$SSH_KEY_ID" ] || die "Failed to upload SSH key"
  log "SSH key uploaded (id=$SSH_KEY_ID)"
fi

# Create server
log "Creating server '$SERVER_NAME'..."
RESPONSE=$(curl -s -X POST \
  -H "Authorization: Bearer $HETZNER_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\":\"$SERVER_NAME\",
    \"server_type\":\"$SERVER_TYPE\",
    \"location\":\"$LOCATION\",
    \"image\":\"$IMAGE\",
    \"ssh_keys\":[$SSH_KEY_ID],
    \"user_data\":\"\"
  }" \
  "https://api.hetzner.cloud/v1/servers")

SERVER_ID=$(echo "$RESPONSE" | jq -r '.server.id // empty')
[ -n "$SERVER_ID" ] || die "Failed to create server: $(echo "$RESPONSE" | jq -r '.error.message // "unknown error"')"
log "Server created (id=$SERVER_ID) — waiting for 'running' status..."

# Poll until running
for i in $(seq 1 60); do
  STATUS=$(curl -s \
    -H "Authorization: Bearer $HETZNER_API_TOKEN" \
    "https://api.hetzner.cloud/v1/servers/$SERVER_ID" \
    | jq -r '.server.status')
  if [ "$STATUS" = "running" ]; then
    break
  fi
  [ "$i" -lt 60 ] || die "Timed out waiting for server to reach 'running' state"
  log "  status=$STATUS — retrying in 5s..."
  sleep 5
done

# Get public IP
NODE_IP=$(curl -s \
  -H "Authorization: Bearer $HETZNER_API_TOKEN" \
  "https://api.hetzner.cloud/v1/servers/$SERVER_ID" \
  | jq -r '.server.public_net.ipv4.ip')
[ -n "$NODE_IP" ] || die "Could not retrieve server IP"

# Write IP to state file
mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/hetzner.ip"
log "Server running at $NODE_IP — written to infra/state/hetzner.ip"

# Wait for SSH to be ready (server reports 'running' before sshd starts)
log "Waiting for SSH on $NODE_IP..."
for i in $(seq 1 30); do
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" "root@$NODE_IP" true 2>/dev/null && break
  [ "$i" -lt 30 ] || die "SSH not ready on $NODE_IP after 150s"
  log "  SSH not ready — retrying in 5s ($i/30)..."
  sleep 5
done

# Run base bootstrap over SSH (root is still permitted at this stage;
# base.sh disables root login as its final step).
log "Running base bootstrap on $NODE_IP..."
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" \
  "root@$NODE_IP" "NODE_NAME=n1 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N1/Hetzner is up at $NODE_IP"
