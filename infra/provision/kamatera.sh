#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n2"
SERVER_NAME="oilfield-n2"
DATACENTER="US-LA"   # Los Angeles
CPU_TYPE="B"         # Standard (burstable)
CPU_CORES=1
RAM_MB=2048
DISK_GB=20
DISK_TYPE="SSD"
IMAGE="ubuntu_noble_server_x86_64_optimized"   # Ubuntu 24.04 LTS

log() { echo "[kamatera] $*" >&2; }
die() { echo "[kamatera] ERROR: $*" >&2; exit 1; }

BASE_URL="https://console.kamatera.com"

# Acquire auth token (valid 1 hour)
log "Authenticating..."
AUTH_TOKEN=$(curl -s -X POST "$BASE_URL/service/authenticate" \
  -d "clientId=$KAMATERA_CLIENT_ID&secret=$KAMATERA_CLIENT_SECRET" \
  | jq -r '.authentication // empty')
[ -n "$AUTH_TOKEN" ] || die "Authentication failed — check KAMATERA_CLIENT_ID and KAMATERA_CLIENT_SECRET"
log "Authenticated"

AUTH_HEADER="AuthClientId: $KAMATERA_CLIENT_ID"
AUTH_KEY_HEADER="AuthToken: $AUTH_TOKEN"

SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

# Create server — Kamatera returns a task ID, not an immediate server object
log "Creating server '$SERVER_NAME' in $DATACENTER..."
RESPONSE=$(curl -s -X POST "$BASE_URL/service/server" \
  -H "$AUTH_HEADER" \
  -H "$AUTH_KEY_HEADER" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"$SERVER_NAME\",
    \"datacenter\": \"$DATACENTER\",
    \"image\": \"$IMAGE\",
    \"cpu\": \"${CPU_CORES}${CPU_TYPE}\",
    \"ram\": $RAM_MB,
    \"disk\": \"size=${DISK_GB}type=${DISK_TYPE}\",
    \"dailybackup\": false,
    \"managed\": false,
    \"networks\": [{\"name\":\"wan\",\"ip\":\"auto\"}],
    \"quantity\": 1,
    \"billingcycle\": \"hourly\",
    \"monthlypackage\": \"\",
    \"poweronaftercreate\": true,
    \"sshKey\": \"$SSH_PUBLIC_KEY\"
  }")

TASK_ID=$(echo "$RESPONSE" | jq -r '.[0] // empty')
[ -n "$TASK_ID" ] || die "Failed to create server: $RESPONSE"
log "Task queued (id=$TASK_ID) — waiting for completion..."

# Poll task until complete
for i in $(seq 1 120); do
  TASK_RESPONSE=$(curl -s "$BASE_URL/service/queue/$TASK_ID" \
    -H "$AUTH_HEADER" \
    -H "$AUTH_KEY_HEADER")
  STATUS=$(echo "$TASK_RESPONSE" | jq -r '.status // empty')
  case "$STATUS" in
    complete)
      break
      ;;
    error)
      die "Task failed: $(echo "$TASK_RESPONSE" | jq -r '.log // "unknown error"')"
      ;;
    *)
      [ "$i" -lt 120 ] || die "Timed out waiting for task to complete"
      log "  status=$STATUS — retrying in 5s..."
      sleep 5
      ;;
  esac
done

# Extract server ID from completed task log
SERVER_ID=$(curl -s "$BASE_URL/service/queue/$TASK_ID" \
  -H "$AUTH_HEADER" \
  -H "$AUTH_KEY_HEADER" \
  | jq -r '.log | split("\n") | map(select(test("server id|serverid|created server"; "i"))) | last | gsub(".*[^0-9]([0-9]+)$"; "\1") // empty')

# Get server IP — list servers and match by name
log "Retrieving server IP..."
NODE_IP=$(curl -s "$BASE_URL/service/servers" \
  -H "$AUTH_HEADER" \
  -H "$AUTH_KEY_HEADER" \
  | jq -r --arg name "$SERVER_NAME" '.[] | select(.name == $name) | .networks[0].ips[0].ip // empty' \
  | head -1)

# Fallback: parse IP from task log
if [ -z "$NODE_IP" ]; then
  NODE_IP=$(curl -s "$BASE_URL/service/queue/$TASK_ID" \
    -H "$AUTH_HEADER" \
    -H "$AUTH_KEY_HEADER" \
    | jq -r '.log' \
    | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' \
    | grep -v '^10\.' | grep -v '^192\.168\.' | grep -v '^172\.' \
    | head -1)
fi
[ -n "$NODE_IP" ] || die "Could not retrieve server IP — check Kamatera console manually"

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/kamatera.ip"
log "Server running at $NODE_IP — written to infra/state/kamatera.ip"

# Run base bootstrap over SSH
log "Running base bootstrap on $NODE_IP..."
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i "$SSH_PRIVATE_KEY_PATH" \
  "root@$NODE_IP" "NODE_NAME=n2 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N2/Kamatera is up at $NODE_IP"
