#!/usr/bin/env bash
set -euo pipefail

# dns.sh — point oilfield-dash.parso.guru at the co-host IONOS box via a
# Cloudflare-proxied A record. Overwrites whatever is there (e.g. the old
# CF Pages CNAME). Idempotent.
#
#   IP=217.76.158.90 bash deploy/scripts/dns.sh
#
# Credentials:
#   CLOUDFLARE_API_TOKEN — defaults to contents of ~/.cloudflare-api-token
#   ZONE                 — defaults to parso.guru
#   RECORD               — defaults to oilfield-dash.parso.guru

IP="${IP:?set IP=<ionos-public-ip>}"
ZONE="${ZONE:-parso.guru}"
RECORD="${RECORD:-oilfield-dash.parso.guru}"
TOKEN="${CLOUDFLARE_API_TOKEN:-$(cat ~/.cloudflare-api-token 2>/dev/null || true)}"
[ -n "$TOKEN" ] || { echo "no Cloudflare token (set CLOUDFLARE_API_TOKEN or ~/.cloudflare-api-token)"; exit 1; }

api() { curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" "$@"; }

# ZONE_ID may be provided directly (zone-scoped tokens can't list zones by name).
ZONE_ID="${ZONE_ID:-}"
if [ -z "$ZONE_ID" ]; then
  ZONE_ID=$(api "https://api.cloudflare.com/client/v4/zones?name=$ZONE" | jq -r '.result[0].id')
fi
[ -n "$ZONE_ID" ] && [ "$ZONE_ID" != "null" ] || { echo "zone $ZONE not found (try ZONE_ID=...)"; exit 1; }
echo "zone $ZONE -> $ZONE_ID"

# Delete any existing records (A or CNAME) for the name to avoid conflicts.
for rid in $(api "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records?name=$RECORD" | jq -r '.result[].id'); do
  echo "deleting existing record $rid for $RECORD"
  api -X DELETE "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records/$rid" >/dev/null
done

echo "creating proxied A $RECORD -> $IP"
api -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records" \
  --data "{\"type\":\"A\",\"name\":\"$RECORD\",\"content\":\"$IP\",\"ttl\":1,\"proxied\":true}" \
  | jq -r 'if .success then "  OK" else "  FAIL: \(.errors)" end'

echo
echo "Reminder: set parso.guru SSL/TLS mode to 'Full' and DELETE the old CF Pages"
echo "project 'oilfield-dash' (Cloudflare dash → Workers & Pages) to avoid confusion."
