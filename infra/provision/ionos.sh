#!/usr/bin/env bash
set -euo pipefail

# Provisions N3 (runtime node) on Ionos Cloud.
#
# Credentials — set in infra/.env:
#   IONOS_SECRET          — Bearer token from Ionos Cloud DCD Token Manager
#                           (Menu → Management → Token Manager → Generate Token)
#                           Regenerate if expired; tokens have a configurable TTL.
#
# The script:
#   1. Creates a Virtual Data Center (VDC) in de/txl (Berlin)
#   2. Creates an Ubuntu 24.04 server (2 vCPU, 2 GB RAM — VPS S equivalent)
#   3. Creates a boot volume (80 GB SSD)
#   4. Attaches a NIC with a reserved public IP
#   5. Polls until server is RUNNING
#   6. Extracts the public IP and writes it to infra/state/ionos.ip
#   7. Waits for SSH and runs base.sh bootstrap

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_NAME="n3"
SERVER_NAME="oilfield-n3"
DC_NAME="oilfield-dc"
LOCATION="de/txl"          # Berlin, DE
CORES=2
RAM_MB=2048                 # 2 GB RAM
DISK_GB=80

log() { echo "[ionos] $*" >&2; }
die() { echo "[ionos] ERROR: $*" >&2; exit 1; }

BASE_URL="https://api.ionos.com/cloudapi/v6"
AUTH_HEADER="Authorization: Bearer ${IONOS_SECRET}"

ionos_get()  { curl -sf -H "$AUTH_HEADER" -H "Accept: application/json" "$BASE_URL$1"; }
ionos_post() { curl -sf -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" -H "Accept: application/json" -d "$2" "$BASE_URL$1"; }
ionos_del()  { curl -sf -X DELETE -H "$AUTH_HEADER" "$BASE_URL$1" || true; }

# Verify auth
log "Verifying Ionos Cloud credentials..."
ionos_get "/locations?depth=0" > /dev/null \
  || die "Ionos API auth failed — regenerate IONOS_SECRET in the DCD Token Manager:
  https://dcd.ionos.com → Menu → Management → Token Manager → Generate Token
  Then update infra/.env with the new token value."
log "Auth OK"

# ── 1. Create (or reuse) a Virtual Data Center ────────────────────────────────

log "Checking for existing VDC '$DC_NAME'..."
EXISTING_DC_ID=$(ionos_get "/datacenters?depth=1" \
  | jq -r --arg n "$DC_NAME" '.items[] | select(.properties.name == $n) | .id // empty' \
  | head -1)

if [ -n "$EXISTING_DC_ID" ]; then
  DC_ID="$EXISTING_DC_ID"
  log "Reusing existing VDC id=$DC_ID"
else
  log "Creating VDC '$DC_NAME' in $LOCATION..."
  DC_RESP=$(ionos_post "/datacenters" \
    "{\"properties\":{\"name\":\"$DC_NAME\",\"location\":\"$LOCATION\",\"description\":\"oilfield cluster N3\"}}")
  DC_ID=$(echo "$DC_RESP" | jq -r '.id // empty')
  [ -n "$DC_ID" ] || die "Failed to create VDC: $DC_RESP"
  log "VDC created id=$DC_ID"

  # Wait for VDC provisioning
  for i in $(seq 1 20); do
    STATE=$(ionos_get "/datacenters/$DC_ID" | jq -r '.metadata.state // empty')
    [ "$STATE" = "AVAILABLE" ] && break
    [ "$i" -lt 20 ] || die "VDC did not become AVAILABLE after 60s"
    log "  VDC state=$STATE — waiting..."
    sleep 3
  done
fi

# ── 2. Reserve a public IP block (1 IP) ──────────────────────────────────────

log "Reserving public IP in $LOCATION..."
IP_RESP=$(ionos_post "/ipblocks" \
  "{\"properties\":{\"location\":\"$LOCATION\",\"size\":1}}")
IP_BLOCK_ID=$(echo "$IP_RESP" | jq -r '.id // empty')
[ -n "$IP_BLOCK_ID" ] || die "Failed to reserve IP block: $IP_RESP"

# Wait for IP block
for i in $(seq 1 20); do
  STATE=$(ionos_get "/ipblocks/$IP_BLOCK_ID" | jq -r '.metadata.state // empty')
  [ "$STATE" = "AVAILABLE" ] && break
  [ "$i" -lt 20 ] || die "IP block did not become AVAILABLE"
  sleep 3
done
NODE_IP=$(ionos_get "/ipblocks/$IP_BLOCK_ID" | jq -r '.properties.ips[0]')
log "Public IP: $NODE_IP (block id=$IP_BLOCK_ID)"

# ── 3. Create server ──────────────────────────────────────────────────────────

log "Creating server '$SERVER_NAME' ($CORES vCPU, ${RAM_MB}MB RAM)..."
SRV_RESP=$(ionos_post "/datacenters/$DC_ID/servers" \
  "{\"properties\":{\"name\":\"$SERVER_NAME\",\"cores\":$CORES,\"ram\":$RAM_MB,\"cpuFamily\":\"INTEL_SKYLAKE\"}}")
SRV_ID=$(echo "$SRV_RESP" | jq -r '.id // empty')
[ -n "$SRV_ID" ] || die "Failed to create server: $SRV_RESP"
log "Server id=$SRV_ID"

# ── 4. Create + attach boot volume ───────────────────────────────────────────

log "Finding Ubuntu 24.04 image in $LOCATION..."
IMG_ID=$(ionos_get "/images?depth=1" \
  | jq -r --arg loc "$LOCATION" \
    '[.items[] | select(.properties.imageType=="HDD" and .properties.location==$loc and (.properties.name | test("Ubuntu-24")) and .properties.public==true)] | sort_by(.properties.createdDate) | last | .id // empty')
[ -n "$IMG_ID" ] || die "Ubuntu 24.04 image not found in $LOCATION"
log "Ubuntu 24.04 image id=$IMG_ID"

SSH_PUBLIC_KEY="$(cat "$SSH_PUBLIC_KEY_PATH")"

log "Creating boot volume (${DISK_GB}GB SSD)..."
VOL_RESP=$(ionos_post "/datacenters/$DC_ID/volumes" \
  "{\"properties\":{
    \"name\":\"${SERVER_NAME}-boot\",
    \"type\":\"SSD Standard\",
    \"size\":$DISK_GB,
    \"image\":\"$IMG_ID\",
    \"imagePassword\":\"$(openssl rand -base64 18 | tr -dc 'A-Za-z0-9' | head -c 20)Aa1!\",
    \"sshKeys\":[\"$SSH_PUBLIC_KEY\"]
  }}")
VOL_ID=$(echo "$VOL_RESP" | jq -r '.id // empty')
[ -n "$VOL_ID" ] || die "Failed to create volume: $VOL_RESP"
log "Volume id=$VOL_ID"

# Wait for volume to be AVAILABLE
for i in $(seq 1 60); do
  STATE=$(ionos_get "/datacenters/$DC_ID/volumes/$VOL_ID" | jq -r '.metadata.state // empty')
  [ "$STATE" = "AVAILABLE" ] && break
  [ "$i" -lt 60 ] || die "Volume did not become AVAILABLE after 180s"
  log "  volume state=$STATE — waiting..."
  sleep 3
done
log "Volume AVAILABLE"

# Attach volume to server (as boot volume)
ionos_post "/datacenters/$DC_ID/servers/$SRV_ID/volumes" \
  "{\"id\":\"$VOL_ID\"}" > /dev/null
log "Volume attached to server"

# Set boot volume
ionos_post "/datacenters/$DC_ID/servers/$SRV_ID" \
  "{\"properties\":{\"bootVolume\":{\"id\":\"$VOL_ID\"}}}" > /dev/null 2>&1 || true

# ── 5. Create NIC with the reserved public IP ─────────────────────────────────

log "Creating NIC with public IP $NODE_IP..."
NIC_RESP=$(ionos_post "/datacenters/$DC_ID/servers/$SRV_ID/nics" \
  "{\"properties\":{\"name\":\"${SERVER_NAME}-nic\",\"dhcp\":false,\"ips\":[\"$NODE_IP\"],\"lan\":1}}")
NIC_ID=$(echo "$NIC_RESP" | jq -r '.id // empty')
[ -n "$NIC_ID" ] || die "Failed to create NIC: $NIC_RESP"
log "NIC id=$NIC_ID"

# ── 6. Start the server ───────────────────────────────────────────────────────

log "Starting server..."
ionos_post "/datacenters/$DC_ID/servers/$SRV_ID/start" "{}" > /dev/null 2>&1 || true

log "Waiting for server to reach RUNNING state..."
for i in $(seq 1 60); do
  STATE=$(ionos_get "/datacenters/$DC_ID/servers/$SRV_ID" | jq -r '.metadata.state // empty')
  [ "$STATE" = "RUNNING" ] && break
  [ "$i" -lt 60 ] || die "Server did not reach RUNNING state after 180s (last state: $STATE)"
  log "  state=$STATE — waiting..."
  sleep 3
done
log "Server RUNNING"

# ── 7. Save state ─────────────────────────────────────────────────────────────

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP"       > "$SCRIPT_DIR/../state/ionos.ip"
echo "$DC_ID"         > "$SCRIPT_DIR/../state/ionos_dc_id"
echo "$SRV_ID"        > "$SCRIPT_DIR/../state/ionos_server_id"
echo "$IP_BLOCK_ID"   > "$SCRIPT_DIR/../state/ionos_ipblock_id"

log "Node IP $NODE_IP written to infra/state/ionos.ip"

# ── 8. Wait for SSH ───────────────────────────────────────────────────────────

log "Waiting for SSH on $NODE_IP (cloud-init may take 60-90s)..."
for i in $(seq 1 36); do
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -o IdentitiesOnly=yes \
    -i "$SSH_PRIVATE_KEY_PATH" "root@$NODE_IP" true 2>/dev/null && break
  [ "$i" -lt 36 ] || die "SSH not ready on $NODE_IP after 180s"
  log "  SSH not ready ($i/36) — retrying in 5s..."
  sleep 5
done
log "SSH ready"

# ── 9. Run base bootstrap ─────────────────────────────────────────────────────

log "Running base bootstrap on $NODE_IP..."
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o IdentitiesOnly=yes \
  -i "$SSH_PRIVATE_KEY_PATH" \
  "root@$NODE_IP" \
  "NODE_NAME=n3 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh"

log "Done. N3/Ionos is up at $NODE_IP (DC=$DC_ID, server=$SRV_ID)"
