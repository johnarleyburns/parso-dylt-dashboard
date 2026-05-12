#!/usr/bin/env bash
# teardown-all.sh — Delete all four cluster VMs and clean up state files.
#
# Usage:
#   ./infra/teardown/teardown-all.sh           # prompts for confirmation
#   ./infra/teardown/teardown-all.sh --yes     # skip confirmation (CI/script use)
#
# What this does:
#   1. Deletes oilfield-n1 from Hetzner Cloud
#   2. Deletes oilfield-n2 from Linode/Akamai
#   3. Deletes oilfield-n3 from Ionos Cloud (server + VDC + IP block)
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
  echo "    Hetzner  oilfield-n1  (API)"
  echo "    Linode   oilfield-n2  (API)"
  echo "    Ionos    oilfield-n3  (MANUAL — you must delete at my.ionos.com)"
  echo "    UpCloud  oilfield-n4  (API)"
  echo ""
  echo "  NOTE: N3 (Ionos VPS) has no API — this script will remove it from DNS"
  echo "  and clean up state files, but you must manually delete the VPS at"
  echo "  my.ionos.com to stop billing."
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

# ─── Ionos VPS: oilfield-n3 ───────────────────────────────────────────────────
#
# NOTE: Ionos consumer VPS has no provisioning API — it cannot be deleted
# programmatically. This section cleans up DNS and local state files only.
# YOU MUST manually delete the VPS at https://my.ionos.com to stop billing.

log "Ionos VPS — oilfield-n3 (manual deletion required)"

N3_IP="${IONOS_VPS_IP:-$(cat "$INFRA_DIR/state/ionos.ip" 2>/dev/null || true)}"
if [ -n "$N3_IP" ]; then
  warn "Cannot delete Ionos VPS via API — please delete it manually at my.ionos.com"
  warn "VPS IP was: $N3_IP"
  ok "State files will be removed below; DNS will be cleaned up if REMOVE_DNS=1"
else
  warn "No Ionos VPS IP found in state — nothing to do for N3"
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
  step "Found server uuid=$SERVER_UUID — querying storage, then stopping and deleting..."

  # Query attached storage BEFORE stopping/deleting the server (server detail API
  # returns empty storage_devices once the server is deleted).
  STORAGE_UUIDS=$(curl -s \
    -H "$AUTH_HEADER_UC" \
    -H "Accept: application/json" \
    "$BASE_URL_UC/server/$SERVER_UUID" \
    | jq -r '.server.storage_devices.storage_device[].uuid // empty')

  # Stop server first (required before delete on UpCloud)
  curl -s -X POST \
    -H "$AUTH_HEADER_UC" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d '{"stop_server":{"stop_type":"hard","timeout":10}}' \
    "$BASE_URL_UC/server/$SERVER_UUID/stop" > /dev/null || true

  step "Waiting 20s for server to stop..."
  sleep 20

  # Delete server
  curl -s -X DELETE \
    -H "$AUTH_HEADER_UC" \
    -H "Accept: application/json" \
    "$BASE_URL_UC/server/$SERVER_UUID" > /dev/null || true
  ok "oilfield-n4 deleted"

  # Delete attached storage volumes (must happen after server is deleted)
  sleep 3
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
       "$INFRA_DIR/state/ionos.ip" \
       "$INFRA_DIR/state/upcloud.ip" \
       "$INFRA_DIR/state/cluster.env"
ok "infra/state/*.ip and cluster.env removed"

# Remove stale SSH known_hosts entries for all cluster hostnames and any IPs
# recorded in cluster.env before we deleted it.  Silently skips if not present.
log "Purging stale SSH known_hosts entries"
source "$INFRA_DIR/.env"
for host in \
    "$DOMAIN" \
    "n1.$DOMAIN" "n2.$DOMAIN" "n3.$DOMAIN" \
    "ctrl.$DOMAIN" "etcd.$DOMAIN" "api.$DOMAIN"; do
  ssh-keygen -R "$host" 2>/dev/null || true
done
ok "known_hosts entries removed (IPs purged by up.sh on next run)"

# ─── Done ─────────────────────────────────────────────────────────────────────

log "TEARDOWN COMPLETE"
echo ""
echo "  All VMs deleted. To rebuild: ./infra/up.sh"
echo ""
