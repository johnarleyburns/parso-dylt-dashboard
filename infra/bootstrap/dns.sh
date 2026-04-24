#!/usr/bin/env bash
set -euo pipefail

# Creates/updates all Cloudflare DNS A records for the oilfield cluster.
# Run LOCALLY after all four provision scripts have completed.
# Reads IPs from infra/state/*.ip and secrets from infra/.env.
#
# Records created:
#   oilfield.<DOMAIN>   A  N1, N2, N3  TTL=60   (round-robin, user-facing)
#   n1.<DOMAIN>         A  N1          TTL=300
#   n2.<DOMAIN>         A  N2          TTL=300
#   n3.<DOMAIN>         A  N3          TTL=300
#   ctrl.<DOMAIN>       A  N4          TTL=300
#   etcd.<DOMAIN>       A  N1, N2, N3  TTL=60   (peer discovery)
#   api.<DOMAIN>        A  N1, N2, N3  TTL=60   (Go REST API)
#
# NOT created: dash.<DOMAIN> — Cloudflare Pages manages that CNAME automatically.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

log() { echo "[dns] $*" >&2; }
die() { echo "[dns] ERROR: $*" >&2; exit 1; }

STATE_DIR="$SCRIPT_DIR/../state"
N1_IP="$(cat "$STATE_DIR/hetzner.ip"  2>/dev/null)" || die "infra/state/hetzner.ip missing — run hetzner.sh first"
N2_IP="$(cat "$STATE_DIR/kamatera.ip" 2>/dev/null)" || die "infra/state/kamatera.ip missing — run kamatera.sh first"
N3_IP="$(cat "$STATE_DIR/scaleway.ip" 2>/dev/null)" || die "infra/state/scaleway.ip missing — run scaleway.sh first"
N4_IP="$(cat "$STATE_DIR/upcloud.ip"  2>/dev/null)" || die "infra/state/upcloud.ip missing — run upcloud.sh first"

log "IPs: N1=$N1_IP  N2=$N2_IP  N3=$N3_IP  N4=$N4_IP"

CF_API="https://api.cloudflare.com/client/v4"
AUTH_HEADER="Authorization: Bearer $CLOUDFLARE_API_TOKEN"
ZONE="$CLOUDFLARE_ZONE_ID"

# Extract the base zone name for constructing record names.
# DOMAIN is e.g. "oilfield.parso.guru" — the CF zone covers "parso.guru".
# Record name in CF must be fully qualified.
BASE_DOMAIN="${DOMAIN}"   # each record below uses $DOMAIN directly as prefix

# upsert_a <name> <ip> <ttl>
# Creates the record if it doesn't exist, updates if it does.
upsert_a() {
  local name="$1" ip="$2" ttl="$3"
  local fqdn="${name}.${DOMAIN}"

  # Look for an existing record matching this name + IP
  local existing_id
  existing_id=$(curl -s \
    -H "$AUTH_HEADER" \
    "$CF_API/zones/$ZONE/dns_records?type=A&name=${fqdn}&content=${ip}" \
    | jq -r '.result[0].id // empty')

  if [ -n "$existing_id" ]; then
    log "  UPDATE $fqdn → $ip (id=$existing_id)"
    curl -s -X PUT \
      -H "$AUTH_HEADER" \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"A\",\"name\":\"${fqdn}\",\"content\":\"${ip}\",\"ttl\":${ttl},\"proxied\":false}" \
      "$CF_API/zones/$ZONE/dns_records/$existing_id" | jq -r '.success' | grep -q true \
      || die "Failed to update $fqdn → $ip"
  else
    log "  CREATE $fqdn → $ip (ttl=$ttl)"
    curl -s -X POST \
      -H "$AUTH_HEADER" \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"A\",\"name\":\"${fqdn}\",\"content\":\"${ip}\",\"ttl\":${ttl},\"proxied\":false}" \
      "$CF_API/zones/$ZONE/dns_records" | jq -r '.success' | grep -q true \
      || die "Failed to create $fqdn → $ip"
  fi
}

# Special case: oilfield.<DOMAIN> is the apex (3 A records, round-robin)
upsert_apex_a() {
  local ip="$1" ttl="$2"
  local fqdn="$DOMAIN"   # oilfield.parso.guru IS the apex record

  local existing_id
  existing_id=$(curl -s \
    -H "$AUTH_HEADER" \
    "$CF_API/zones/$ZONE/dns_records?type=A&name=${fqdn}&content=${ip}" \
    | jq -r '.result[0].id // empty')

  if [ -n "$existing_id" ]; then
    log "  UPDATE $fqdn → $ip (id=$existing_id)"
    curl -s -X PUT \
      -H "$AUTH_HEADER" \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"A\",\"name\":\"${fqdn}\",\"content\":\"${ip}\",\"ttl\":${ttl},\"proxied\":false}" \
      "$CF_API/zones/$ZONE/dns_records/$existing_id" | jq -r '.success' | grep -q true \
      || die "Failed to update $fqdn → $ip"
  else
    log "  CREATE $fqdn → $ip (ttl=$ttl)"
    curl -s -X POST \
      -H "$AUTH_HEADER" \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"A\",\"name\":\"${fqdn}\",\"content\":\"${ip}\",\"ttl\":${ttl},\"proxied\":false}" \
      "$CF_API/zones/$ZONE/dns_records" | jq -r '.success' | grep -q true \
      || die "Failed to create $fqdn → $ip"
  fi
}

log "Creating/updating DNS records for $DOMAIN..."

# oilfield.<DOMAIN> — 3 A records (round-robin to N1, N2, N3)
log "oilfield.$DOMAIN round-robin records:"
upsert_apex_a "$N1_IP" 60
upsert_apex_a "$N2_IP" 60
upsert_apex_a "$N3_IP" 60

# Per-node records (stable, 300s TTL)
log "Per-node records:"
upsert_a "n1"   "$N1_IP" 300
upsert_a "n2"   "$N2_IP" 300
upsert_a "n3"   "$N3_IP" 300
upsert_a "ctrl" "$N4_IP" 300

# Cluster records — also round-robin to N1, N2, N3 (60s TTL for fast failover)
log "Cluster records (etcd, api):"
upsert_a "etcd" "$N1_IP" 60
upsert_a "etcd" "$N2_IP" 60
upsert_a "etcd" "$N3_IP" 60
upsert_a "api"  "$N1_IP" 60
upsert_a "api"  "$N2_IP" 60
upsert_a "api"  "$N3_IP" 60

log ""
log "DNS records created. Verifying with dig (may take up to 60s to propagate)..."
sleep 5

for host in "$DOMAIN" "n1.$DOMAIN" "n2.$DOMAIN" "n3.$DOMAIN" "ctrl.$DOMAIN" "etcd.$DOMAIN" "api.$DOMAIN"; do
  result=$(dig +short A "$host" @1.1.1.1 2>/dev/null | tr '\n' ' ')
  printf "  %-35s → %s\n" "$host" "${result:-<no answer yet>}"
done

log ""
log "DNS bootstrap complete."
log "Note: dash.$DOMAIN CNAME is managed by Cloudflare Pages — do not create it here."
