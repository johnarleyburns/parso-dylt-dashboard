#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n3"
SERVER_NAME="oilfield-n3"
COMMERCIAL_TYPE="PLAY2-MICRO"
IMAGE_LABEL="ubuntu_noble"   # Ubuntu 24.04 LTS

log() { echo "[scaleway] $*" >&2; }
die() { echo "[scaleway] ERROR: $*" >&2; exit 1; }

BASE_URL="https://api.scaleway.com/instance/v1/zones/$SCW_ZONE"
AUTH_HEADER="X-Auth-Token: $SCW_SECRET_KEY"

# Resolve Ubuntu 24.04 image ID for this zone
log "Resolving Ubuntu 24.04 image ID..."
IMAGE_ID=$(curl -s \
  -H "$AUTH_HEADER" \
  "$BASE_URL/images?arch=x86_64&per_page=100" \
  | jq -r '[.images[] | select(.name | startswith("Ubuntu 24.04"))] | sort_by(.creation_date) | last | .id // empty')
[ -n "$IMAGE_ID" ] || die "Could not find Ubuntu 24.04 image in zone $SCW_ZONE"
log "Image ID: $IMAGE_ID"

# Upload SSH public key (idempotent)
SSH_KEY_NAME="oilfield-deploy"
SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

log "Checking for existing SSH key..."
EXISTING_KEY_ID=$(curl -s \
  -H "$AUTH_HEADER" \
  "https://api.scaleway.com/iam/v1alpha1/ssh-keys?name=$SSH_KEY_NAME" \
  | jq -r '.ssh_keys[0].id // empty')

if [ -n "$EXISTING_KEY_ID" ]; then
  log "SSH key already exists (id=$EXISTING_KEY_ID)"
  SSH_KEY_ID="$EXISTING_KEY_ID"
else
  log "Uploading SSH key..."
  SSH_KEY_ID=$(curl -s -X POST \
    -H "$AUTH_HEADER" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$SSH_KEY_NAME\",\"public_key\":\"$SSH_PUBLIC_KEY\",\"project_id\":\"$SCW_PROJECT_ID\"}" \
    "https://api.scaleway.com/iam/v1alpha1/ssh-keys" \
    | jq -r '.id // empty')
  [ -n "$SSH_KEY_ID" ] || die "Failed to upload SSH key"
  log "SSH key uploaded (id=$SSH_KEY_ID)"
fi

# Create server
log "Creating server '$SERVER_NAME'..."
RESPONSE=$(curl -s -X POST \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\":\"$SERVER_NAME\",
    \"commercial_type\":\"$COMMERCIAL_TYPE\",
    \"image\":\"$IMAGE_ID\",
    \"project\":\"$SCW_PROJECT_ID\"
  }" \
  "$BASE_URL/servers")

SERVER_ID=$(echo "$RESPONSE" | jq -r '.server.id // empty')
[ -n "$SERVER_ID" ] || die "Failed to create server: $(echo "$RESPONSE" | jq -r '.message // "unknown error"')"
log "Server created (id=$SERVER_ID)"

# Power on
curl -s -X POST \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{"action":"poweron"}' \
  "$BASE_URL/servers/$SERVER_ID/action" > /dev/null

log "Power-on action sent — waiting for 'running' state..."

# Poll until running
for i in $(seq 1 60); do
  STATE=$(curl -s \
    -H "$AUTH_HEADER" \
    "$BASE_URL/servers/$SERVER_ID" \
    | jq -r '.server.state')
  if [ "$STATE" = "running" ]; then
    break
  fi
  [ "$i" -lt 60 ] || die "Timed out waiting for server to reach 'running' state"
  log "  state=$STATE — retrying in 5s..."
  sleep 5
done

# Get public IP
NODE_IP=$(curl -s \
  -H "$AUTH_HEADER" \
  "$BASE_URL/servers/$SERVER_ID" \
  | jq -r '.server.public_ip.address // empty')
[ -n "$NODE_IP" ] || die "Could not retrieve server IP"

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/scaleway.ip"
log "Server running at $NODE_IP — written to infra/state/scaleway.ip"

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
  "root@$NODE_IP" "NODE_NAME=n3 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N3/Scaleway is up at $NODE_IP"
