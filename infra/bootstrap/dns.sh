#!/usr/bin/env bash
set -euo pipefail

# Creates/updates all Cloudflare DNS A records for the oilfield cluster.
# Run LOCALLY after all four provision scripts have completed.
# Reads IPs from infra/state/*.ip and secrets from infra/.env.
#
# Records managed:
#   oilfield.<DOMAIN>   A  N1, N2, N3  TTL=60   (round-robin, user-facing)
#   n1.<DOMAIN>         A  N1          TTL=300
#   n2.<DOMAIN>         A  N2          TTL=300
#   n3.<DOMAIN>         A  N3          TTL=300
#   ctrl.<DOMAIN>       A  N4          TTL=300
#   etcd.<DOMAIN>       A  N1, N2, N3  TTL=60   (peer discovery)
#   api.<DOMAIN>        A  N1, N2, N3  TTL=60   (Go REST API)
#
# NOT managed: dash.<DOMAIN> — Cloudflare Pages manages that CNAME automatically.
#
# Strategy: for each managed FQDN, delete ALL existing A records then create fresh
# ones. This prevents stale IPs from accumulating when nodes are rebuilt with new IPs.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

log() { echo "[dns] $*" >&2; }
die() { echo "[dns] ERROR: $*" >&2; exit 1; }

STATE_DIR="$SCRIPT_DIR/../state"
N1_IP="$(cat "$STATE_DIR/hetzner.ip"  2>/dev/null)" || die "infra/state/hetzner.ip missing — run hetzner.sh first"
N2_IP="$(cat "$STATE_DIR/linode.ip"   2>/dev/null)" || die "infra/state/linode.ip missing — run linode.sh first"
N3_IP="$(cat "$STATE_DIR/ionos.ip"    2>/dev/null)" || die "infra/state/ionos.ip missing — run ionos.sh first"
N4_IP="$(cat "$STATE_DIR/upcloud.ip"  2>/dev/null)" || die "infra/state/upcloud.ip missing — run upcloud.sh first"

log "IPs: N1=$N1_IP  N2=$N2_IP  N3=$N3_IP  N4=$N4_IP"

CF_API="https://api.cloudflare.com/client/v4"
AUTH_HEADER="Authorization: Bearer $CLOUDFLARE_API_TOKEN"
ZONE="$CLOUDFLARE_ZONE_ID"

# delete_all_a <fqdn>
# Deletes all A records for the given FQDN. Idempotent — no error if none exist.
delete_all_a() {
  local fqdn="$1"
  local ids
  ids=$(curl -s \
    -H "$AUTH_HEADER" \
    "$CF_API/zones/$ZONE/dns_records?type=A&name=${fqdn}&per_page=100" \
    | jq -r '.result[].id // empty')
  for id in $ids; do
    log "  DELETE $fqdn (id=$id)"
    curl -s -X DELETE \
      -H "$AUTH_HEADER" \
      "$CF_API/zones/$ZONE/dns_records/$id" > /dev/null \
      || log "  WARN: failed to delete $fqdn id=$id (continuing)"
  done
}

# create_a <fqdn> <ip> <ttl>
# Creates a single A record. Fails hard on error.
create_a() {
  local fqdn="$1" ip="$2" ttl="$3"
  log "  CREATE $fqdn → $ip (ttl=$ttl)"
  curl -s -X POST \
    -H "$AUTH_HEADER" \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"A\",\"name\":\"${fqdn}\",\"content\":\"${ip}\",\"ttl\":${ttl},\"proxied\":false}" \
    "$CF_API/zones/$ZONE/dns_records" | jq -r '.success' | grep -q true \
    || die "Failed to create $fqdn → $ip"
}

# set_a <name> <ip> <ttl>
# For single-IP records (n1, n2, n3, ctrl): replace all existing A records with one.
set_a() {
  local name="$1" ip="$2" ttl="$3"
  local fqdn="${name}.${DOMAIN}"
  delete_all_a "$fqdn"
  create_a "$fqdn" "$ip" "$ttl"
}

# set_apex_a <ip> <ttl>
# One of the three round-robin A records for the apex domain (oilfield.parso.guru).
# The caller is responsible for calling delete_all_a on the apex before the first call.
set_apex_a() {
  local ip="$1" ttl="$2"
  create_a "$DOMAIN" "$ip" "$ttl"
}

log "Creating/updating DNS records for $DOMAIN..."

# oilfield.<DOMAIN> — 3 A records (round-robin to N1, N2, N3)
log "oilfield.$DOMAIN round-robin records:"
delete_all_a "$DOMAIN"
set_apex_a "$N1_IP" 60
set_apex_a "$N2_IP" 60
set_apex_a "$N3_IP" 60

# Per-node records (stable, 300s TTL)
log "Per-node records:"
set_a "n1"   "$N1_IP" 300
set_a "n2"   "$N2_IP" 300
set_a "n3"   "$N3_IP" 300
set_a "ctrl" "$N4_IP" 300

# Cluster records — also round-robin to N1, N2, N3 (60s TTL for fast failover)
log "Cluster records (etcd, api):"
delete_all_a "etcd.${DOMAIN}"
create_a "etcd.${DOMAIN}" "$N1_IP" 60
create_a "etcd.${DOMAIN}" "$N2_IP" 60
create_a "etcd.${DOMAIN}" "$N3_IP" 60

delete_all_a "api.${DOMAIN}"
create_a "api.${DOMAIN}" "$N1_IP" 60
create_a "api.${DOMAIN}" "$N2_IP" 60
create_a "api.${DOMAIN}" "$N3_IP" 60

log ""
log "DNS records set. Verifying with dig (may take up to 60s to propagate)..."
sleep 5

for host in "$DOMAIN" "n1.$DOMAIN" "n2.$DOMAIN" "n3.$DOMAIN" "ctrl.$DOMAIN" "etcd.$DOMAIN" "api.$DOMAIN"; do
  result=$(dig +short A "$host" @1.1.1.1 2>/dev/null | tr '\n' ' ')
  printf "  %-35s → %s\n" "$host" "${result:-<no answer yet>}"
done

log ""
log "DNS bootstrap complete."
log "Note: dash.$DOMAIN CNAME is managed by Cloudflare Pages — do not create it here."
