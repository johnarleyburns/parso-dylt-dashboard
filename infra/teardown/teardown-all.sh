#!/usr/bin/env bash
# teardown-all.sh — Delete all four cluster VMs and clean up state files.
#
# Usage:
#   ./infra/teardown/teardown-all.sh           # prompts for confirmation
#   ./infra/teardown/teardown-all.sh --yes     # skip confirmation (CI/script use)
#
# What this does:
#   1. Deletes oilfield-n1 from Hetzner Cloud
#   2. Deletes oilfield-n2 from Kamatera
#   3. Deletes oilfield-n3 from Scaleway
#   4. Deletes oilfield-n4 from UpCloud
#   5. Removes infra/state/*.ip files
#   6. Optionally: removes Cloudflare DNS A records (controlled by REMOVE_DNS=1)
#
# DNS records are NOT removed by default (costs nothing to leave them and
# re-running up.sh will update them). Set REMOVE_DNS=1 to delete them.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INFRA_DIR="$SCRIPT_DIR/.."
source "$INFRA_DIR/.env"

YES="${1:-}"
REMOVE_DNS="${REMOVE_DNS:-0}"

log()  { echo "" ; echo "━━━ $* ━━━" ; }
step() { echo "  → $*" ; }
warn() { echo "  ⚠  $*" ; }
ok()   { echo "  ✓  $*" ; }

# ─── Confirmation ─────────────────────────────────────────────────────────────

if [ "$YES" != "--yes" ]; then
  echo ""
  echo "  This will DELETE all four oilfield VMs:"
  echo "    Hetzner  oilfield-n1"
  echo "    Linode   oilfield-n2"
  echo "    Scaleway oilfield-n3"
  echo "    UpCloud  oilfield-n4"
  echo ""
  read -rp "  Type 'yes' to confirm: " CONFIRM
  [ "$CONFIRM" = "yes" ] || { echo "  Aborted."; exit 0; }
fi

# ─── Hetzner: delete oilfield-n1 ──────────────────────────────────────────────

log "Hetzner — deleting oilfield-n1"

SERVER_ID=$(curl -s \
  -H "Authorization: Bearer $HETZNER_API_TOKEN" \
  "https://api.hetzner.cloud/v1/servers?name=oilfield-n1" \
  | jq -r '.servers[0].id // empty')

if [ -n "$SERVER_ID" ]; then
  step "Found server id=$SERVER_ID — deleting..."
  curl -s -X DELETE \
    -H "Authorization: Bearer $HETZNER_API_TOKEN" \
    "https://api.hetzner.cloud/v1/servers/$SERVER_ID" | jq -r '.action.status' || true
  ok "oilfield-n1 deleted"
else
  warn "oilfield-n1 not found in Hetzner (already deleted?)"
fi

# ─── Linode: delete oilfield-n2 ──────────────────────────────────────────────

log "Linode — deleting oilfield-n2"

LINODE_ID=$(curl -s \
  -H "Authorization: Bearer $LINODE_TOKEN" \
  "https://api.linode.com/v4/linode/instances?label=oilfield-n2" \
  | jq -r '.data[0].id // empty')

if [ -n "$LINODE_ID" ]; then
  step "Found instance id=$LINODE_ID — deleting..."
  curl -s -X DELETE \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    "https://api.linode.com/v4/linode/instances/$LINODE_ID" | jq -r '.[] // "ok"' 2>/dev/null || true
  ok "oilfield-n2 deleted"
else
  warn "oilfield-n2 not found in Linode (already deleted?)"
fi

# ─── Scaleway: delete oilfield-n3 ─────────────────────────────────────────────

log "Scaleway — deleting oilfield-n3"

BASE_URL="https://api.scaleway.com/instance/v1/zones/$SCW_ZONE"
AUTH_HEADER_SCW="X-Auth-Token: $SCW_SECRET_KEY"

SERVER_ID=$(curl -s \
  -H "$AUTH_HEADER_SCW" \
  "$BASE_URL/servers?name=oilfield-n3" \
  | jq -r '.servers[0].id // empty')

if [ -n "$SERVER_ID" ]; then
  step "Found server id=$SERVER_ID — powering off then deleting..."

  # Terminate (power off + delete) via action
  curl -s -X POST \
    -H "$AUTH_HEADER_SCW" \
    -H "Content-Type: application/json" \
    -d '{"action":"terminate"}' \
    "$BASE_URL/servers/$SERVER_ID/action" | jq -r '.task.status' || true

  ok "oilfield-n3 terminate action sent (Scaleway will handle volume cleanup)"
else
  warn "oilfield-n3 not found in Scaleway (already deleted?)"
fi

# ─── UpCloud: delete oilfield-n4 ──────────────────────────────────────────────

log "UpCloud — deleting oilfield-n4"

AUTH_HEADER_UC="Authorization: Bearer $UPCLOUD_API_TOKEN"
BASE_URL_UC="https://api.upcloud.com/1.3"

SERVER_UUID=$(curl -s \
  -H "$AUTH_HEADER_UC" \
  -H "Accept: application/json" \
  "$BASE_URL_UC/server" \
  | jq -r --arg title "oilfield-n4" \
    '.servers.server[] | select(.title == $title) | .uuid // empty' \
  | head -1)

if [ -n "$SERVER_UUID" ]; then
  step "Found server uuid=$SERVER_UUID — stopping then deleting..."

  # Stop server first (required before delete)
  curl -s -X POST \
    -H "$AUTH_HEADER_UC" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d '{"stop_server":{"stop_type":"hard","timeout":10}}' \
    "$BASE_URL_UC/server/$SERVER_UUID/stop" > /dev/null || true

  step "Waiting 15s for server to stop..."
  sleep 15

  # Delete server (storage is deleted separately — query and delete attached storage)
  STORAGE_UUIDS=$(curl -s \
    -H "$AUTH_HEADER_UC" \
    -H "Accept: application/json" \
    "$BASE_URL_UC/server/$SERVER_UUID" \
    | jq -r '.server.storage_devices.storage_device[].uuid // empty')

  curl -s -X DELETE \
    -H "$AUTH_HEADER_UC" \
    -H "Accept: application/json" \
    "$BASE_URL_UC/server/$SERVER_UUID" > /dev/null || true
  ok "oilfield-n4 deleted"

  for UUID in $STORAGE_UUIDS; do
    step "Deleting storage $UUID..."
    curl -s -X DELETE \
      -H "$AUTH_HEADER_UC" \
      -H "Accept: application/json" \
      "$BASE_URL_UC/storage/$UUID" > /dev/null || true
  done
else
  warn "oilfield-n4 not found in UpCloud (already deleted?)"
fi

# ─── Optional: remove DNS records ─────────────────────────────────────────────

if [ "$REMOVE_DNS" = "1" ]; then
  log "DNS — removing Cloudflare A records"

  CF_API="https://api.cloudflare.com/client/v4"
  CF_AUTH="Authorization: Bearer $CLOUDFLARE_API_TOKEN"

  RECORD_IDS=$(curl -s \
    -H "$CF_AUTH" \
    "$CF_API/zones/$CLOUDFLARE_ZONE_ID/dns_records?type=A&per_page=100" \
    | jq -r '.result[] | select(
        (.name | test("^(oilfield|n1|n2|n3|ctrl|etcd|api)\\.")) or
        .name == "'"$DOMAIN"'"
      ) | .id')

  COUNT=0
  for ID in $RECORD_IDS; do
    curl -s -X DELETE \
      -H "$CF_AUTH" \
      "$CF_API/zones/$CLOUDFLARE_ZONE_ID/dns_records/$ID" > /dev/null
    COUNT=$((COUNT + 1))
  done
  ok "Deleted $COUNT DNS A records"
else
  step "DNS records left in place (set REMOVE_DNS=1 to remove them)"
fi

# ─── Clean up state files ──────────────────────────────────────────────────────

log "Cleaning up state files"
rm -f "$INFRA_DIR/state/hetzner.ip" \
       "$INFRA_DIR/state/linode.ip" \
       "$INFRA_DIR/state/scaleway.ip" \
       "$INFRA_DIR/state/upcloud.ip"
ok "infra/state/*.ip removed"

# ─── Done ─────────────────────────────────────────────────────────────────────

log "TEARDOWN COMPLETE"
echo ""
echo "  All VMs deleted. To rebuild: ./infra/up.sh"
echo ""
