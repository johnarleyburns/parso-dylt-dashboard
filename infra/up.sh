#!/usr/bin/env bash
# up.sh — Full cluster bring-up: provision → bootstrap → DNS → TLS → deploy
#
# Usage:
#   ./infra/up.sh
#
# Prerequisites:
#   - infra/.env populated (all tokens/keys)
#   - SSH key pair at SSH_PRIVATE_KEY_PATH / SSH_PUBLIC_KEY_PATH (from .env)
#   - ADMIN_EMAIL env var set, or export it before running:
#       ADMIN_EMAIL=you@example.com ./infra/up.sh
#   - wrangler installed: npm install -g wrangler
#   - jq, curl, dig, ssh, scp in PATH

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/.env"

ADMIN_EMAIL="${ADMIN_EMAIL:?Set ADMIN_EMAIL before running: export ADMIN_EMAIL=you@example.com}"

log()  { echo "" ; echo "━━━ $* ━━━" ; }
step() { echo "  → $*" ; }
die()  { echo "ERROR: $*" >&2 ; exit 1 ; }

# ─── Phase 1: Provision all four nodes ────────────────────────────────────────

log "PHASE 1 — Provisioning nodes (parallel)"

step "Hetzner  (N1 — runtime, Ashburn VA)..."
bash "$SCRIPT_DIR/provision/hetzner.sh" &
PID_N1=$!

step "Linode   (N2 — runtime, Los Angeles CA)..."
bash "$SCRIPT_DIR/provision/linode.sh" &
PID_N2=$!

step "Scaleway (N3 — runtime, Paris FR)..."
bash "$SCRIPT_DIR/provision/scaleway.sh" &
PID_N3=$!

step "UpCloud  (N4 — control, Chicago IL)..."
bash "$SCRIPT_DIR/provision/upcloud.sh" &
PID_N4=$!

# Wait for all four; abort if any fail
FAIL=0
wait $PID_N1 || { echo "  ✗ Hetzner provision failed"  >&2 ; FAIL=1 ; }
wait $PID_N2 || { echo "  ✗ Linode provision failed" >&2 ; FAIL=1 ; }
wait $PID_N3 || { echo "  ✗ Scaleway provision failed" >&2 ; FAIL=1 ; }
wait $PID_N4 || { echo "  ✗ UpCloud provision failed"  >&2 ; FAIL=1 ; }
[ "$FAIL" -eq 0 ] || die "One or more provision scripts failed — see output above"

N1_IP="$(cat "$SCRIPT_DIR/state/hetzner.ip")"
N2_IP="$(cat "$SCRIPT_DIR/state/linode.ip")"
N3_IP="$(cat "$SCRIPT_DIR/state/scaleway.ip")"
N4_IP="$(cat "$SCRIPT_DIR/state/upcloud.ip")"

echo ""
echo "  Node IPs:"
echo "    N1 (Hetzner)  $N1_IP"
echo "    N2 (Linode)   $N2_IP"
echo "    N3 (Scaleway) $N3_IP"
echo "    N4 (UpCloud)  $N4_IP"

# ─── Phase 2: Daylight bootstrap — etcd on N1, N2, N3 (parallel) ─────────────

log "PHASE 2 — Daylight bootstrap (etcd cluster, N1/N2/N3 in parallel)"

for NODE_IP in "$N1_IP" "$N2_IP" "$N3_IP"; do
  step "daylight.sh on $NODE_IP..."
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i "$SSH_PRIVATE_KEY_PATH" \
    "root@$NODE_IP" \
    "N1_IP=$N1_IP N2_IP=$N2_IP N3_IP=$N3_IP ETCD_CLUSTER_TOKEN=$ETCD_CLUSTER_TOKEN EIA_API_KEY=$EIA_API_KEY DOMAIN=$DOMAIN bash -s" \
    < "$SCRIPT_DIR/bootstrap/daylight.sh" &
done

FAIL=0
for NODE_IP in "$N1_IP" "$N2_IP" "$N3_IP"; do
  wait || { echo "  ✗ daylight.sh failed on $NODE_IP" >&2 ; FAIL=1 ; }
done
[ "$FAIL" -eq 0 ] || die "Daylight bootstrap failed on one or more nodes"

step "etcd cluster should have quorum — waiting 5s for leader election..."
sleep 5

# ─── Phase 3: DNS ─────────────────────────────────────────────────────────────

log "PHASE 3 — DNS (Cloudflare A records)"
bash "$SCRIPT_DIR/bootstrap/dns.sh"

step "Waiting 15s for DNS TTL to settle before TLS..."
sleep 15

# ─── Phase 4: TLS — Let's Encrypt on all four nodes (parallel) ───────────────

log "PHASE 4 — TLS (Let's Encrypt on N1/N2/N3/N4 in parallel)"

for NODE_IP in "$N1_IP" "$N2_IP" "$N3_IP" "$N4_IP"; do
  step "tls.sh on $NODE_IP..."
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i "$SSH_PRIVATE_KEY_PATH" \
    "root@$NODE_IP" \
    "ADMIN_EMAIL=$ADMIN_EMAIL bash -s" \
    < "$SCRIPT_DIR/bootstrap/tls.sh" &
done

FAIL=0
for NODE_IP in "$N1_IP" "$N2_IP" "$N3_IP" "$N4_IP"; do
  wait || { echo "  ✗ tls.sh failed on $NODE_IP" >&2 ; FAIL=1 ; }
done
[ "$FAIL" -eq 0 ] || die "TLS setup failed on one or more nodes"

# ─── Phase 5: Deploy app (N1/N2/N3) ──────────────────────────────────────────

log "PHASE 5 — Deploy oilfield-api + oilfield-scraper (N1/N2/N3)"
bash "$SCRIPT_DIR/deploy/deploy-app.sh" all

# ─── Phase 6: Deploy dashboard (N4 + Cloudflare Pages) ───────────────────────

log "PHASE 6 — Deploy dashboard (Go backend → N4, React frontend → CF Pages)"
bash "$SCRIPT_DIR/deploy/deploy-dash.sh"

# ─── Done ─────────────────────────────────────────────────────────────────────

log "CLUSTER UP"
echo ""
echo "  Endpoints:"
echo "    Price app:   https://$DOMAIN"
echo "    API (N1):    https://n1.$DOMAIN/api/v1/health"
echo "    API (N2):    https://n2.$DOMAIN/api/v1/health"
echo "    API (N3):    https://n3.$DOMAIN/api/v1/health"
echo "    Ctrl (N4):   https://ctrl.$DOMAIN/api/v1/health"
echo "    Dashboard:   https://dash.$DOMAIN"
echo ""
echo "  Quick verify:"
echo "    oilfield-dash status"
echo "    oilfield-dash prices crude"
echo "    oilfield-dash news"
echo ""
echo "  To tear down: ./infra/teardown/teardown-all.sh"
