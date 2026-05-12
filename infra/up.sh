#!/usr/bin/env bash
# up.sh — Full cluster bring-up: provision → bootstrap → DNS → TLS → deploy
#
# Usage:
#   ./infra/up.sh
#
# Prerequisites:
#   - infra/.env populated (all tokens/keys, including ADMIN_EMAIL)
#   - SSH key pair at SSH_PRIVATE_KEY_PATH / SSH_PUBLIC_KEY_PATH (from .env)
#   - wrangler installed: npm install -g wrangler
#   - jq, curl, dig, ssh, scp in PATH
#
# N3 NOTE — Ionos VPS has no provisioning API.
#   Before running this script, manually create a VPS at my.ionos.com, then set:
#     IONOS_VPS_IP=<ip>            in infra/.env
#     IONOS_VPS_PASSWORD=<pass>    in infra/.env  (root password, used only to install SSH key)
#   up.sh will install the SSH key and run all bootstrap phases automatically.
#   If IONOS_VPS_IP is already in .env and ionos.ip state file doesn't exist,
#   this script writes it to infra/state/ionos.ip and proceeds.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/.env"

# ADMIN_EMAIL may come from .env or the environment; both are acceptable.
ADMIN_EMAIL="${ADMIN_EMAIL:?Set ADMIN_EMAIL in infra/.env or export before running}"

log()  { echo "" ; echo "━━━ $* ━━━" ; }
step() { echo "  → $*" ; }
die()  { echo "ERROR: $*" >&2 ; exit 1 ; }

# Guard: abort if any .ip state files already exist (stale prior run).
FORCE="${1:-}"
if [ "$FORCE" != "--force" ]; then
  for f in hetzner linode ionos upcloud; do
    if [ -f "$SCRIPT_DIR/state/${f}.ip" ]; then
      die "infra/state/${f}.ip already exists — cluster may already be up.
  Run: ./infra/teardown/teardown-all.sh
  Or:  ./infra/up.sh --force  (skips this guard)"
    fi
  done
fi

# ─── Phase 1: Provision all four nodes ────────────────────────────────────────

log "PHASE 1 — Provisioning nodes"

step "Hetzner  (N1 — runtime, Ashburn VA)..."
bash "$SCRIPT_DIR/provision/hetzner.sh" &
PID_N1=$!

step "Linode   (N2 — runtime, Los Angeles CA)..."
bash "$SCRIPT_DIR/provision/linode.sh" &
PID_N2=$!

# N3 — Ionos VPS: no provisioning API; use IONOS_VPS_IP set in .env.
step "Ionos    (N3 — runtime, Berlin DE) — manual VPS, bootstrapping from IONOS_VPS_IP..."
IONOS_VPS_IP="${IONOS_VPS_IP:?N3 requires IONOS_VPS_IP in infra/.env — create VPS at my.ionos.com first}"
IONOS_VPS_PASSWORD="${IONOS_VPS_PASSWORD:-}"
bash "$SCRIPT_DIR/provision/ionos-bootstrap.sh" &
PID_N3=$!

step "UpCloud  (N4 — control, Chicago IL)..."
bash "$SCRIPT_DIR/provision/upcloud.sh" &
PID_N4=$!

# Wait for all four using captured PIDs; abort if any fail.
FAIL=0
wait $PID_N1 || { echo "  ✗ Hetzner provision failed"  >&2 ; FAIL=1 ; }
wait $PID_N2 || { echo "  ✗ Linode provision failed"   >&2 ; FAIL=1 ; }
wait $PID_N3 || { echo "  ✗ Ionos provision failed"    >&2 ; FAIL=1 ; }
wait $PID_N4 || { echo "  ✗ UpCloud provision failed"  >&2 ; FAIL=1 ; }
[ "$FAIL" -eq 0 ] || die "One or more provision scripts failed — see output above"

N1_IP="$(cat "$SCRIPT_DIR/state/hetzner.ip")"
N2_IP="$(cat "$SCRIPT_DIR/state/linode.ip")"
N3_IP="$(cat "$SCRIPT_DIR/state/ionos.ip")"
N4_IP="$(cat "$SCRIPT_DIR/state/upcloud.ip")"

# Persist IPs to cluster.env so other scripts and operators can source them easily.
mkdir -p "$SCRIPT_DIR/state"
cat > "$SCRIPT_DIR/state/cluster.env" << EOF
export N1_IP="$N1_IP"
export N2_IP="$N2_IP"
export N3_IP="$N3_IP"
export N4_IP="$N4_IP"
EOF

echo ""
  echo "  Node IPs:"
  echo "    N1 (Hetzner)  $N1_IP"
  echo "    N2 (Linode)   $N2_IP"
  echo "    N3 (Ionos)    $N3_IP"
  echo "    N4 (UpCloud)  $N4_IP"

# Purge any stale known_hosts entries for the new IPs so later SSH/scp by IP
# (in deploy-app.sh and deploy-dash.sh) don't fail on host key mismatch.
step "Purging stale SSH known_hosts entries for new IPs..."
for ip in "$N1_IP" "$N2_IP" "$N3_IP" "$N4_IP"; do
  ssh-keygen -R "$ip" 2>/dev/null || true
done

# ─── Phase 2: Daylight bootstrap — etcd on N1, N2, N3 (parallel) ─────────────

log "PHASE 2 — Daylight bootstrap (etcd cluster, N1/N2/N3 in parallel)"

# Capture PIDs so we can wait on them individually (bare `wait` in a loop
# consumes any background job, not the specific ones we launched).
PIDS_P2=()
NODES_P2=("$N1_IP" "$N2_IP" "$N3_IP")

for NODE_IP in "${NODES_P2[@]}"; do
  step "daylight.sh on $NODE_IP..."
  # base.sh disables root login; use deploy + sudo for all post-provision SSH.
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o IdentitiesOnly=yes \
    -i "$SSH_PRIVATE_KEY_PATH" \
    "deploy@$NODE_IP" \
    "N1_IP=$N1_IP N2_IP=$N2_IP N3_IP=$N3_IP ETCD_CLUSTER_TOKEN=$ETCD_CLUSTER_TOKEN EIA_API_KEY=$EIA_API_KEY DOMAIN=$DOMAIN sudo -E bash -s" \
    < "$SCRIPT_DIR/bootstrap/daylight.sh" &
  PIDS_P2+=($!)
done

FAIL=0
for i in "${!PIDS_P2[@]}"; do
  wait "${PIDS_P2[$i]}" || { echo "  ✗ daylight.sh failed on ${NODES_P2[$i]}" >&2 ; FAIL=1 ; }
done
[ "$FAIL" -eq 0 ] || die "Daylight bootstrap failed on one or more nodes"

step "etcd cluster should have quorum — waiting 10s for leader election..."
sleep 10

# ─── Phase 3: DNS ─────────────────────────────────────────────────────────────

log "PHASE 3 — DNS (Cloudflare A records)"
bash "$SCRIPT_DIR/bootstrap/dns.sh"

# Wait for DNS to propagate before running TLS.  Let's Encrypt validates by
# fetching from the FQDN; if its resolver still returns the old IP the challenge
# will fail.  Poll until dig @1.1.1.1 returns the expected IPs (max 2 minutes).
step "Waiting for DNS propagation (checking @1.1.1.1, max 120s)..."
declare -A EXPECTED_IPS=(
  ["n1.$DOMAIN"]="$N1_IP"
  ["n2.$DOMAIN"]="$N2_IP"
  ["n3.$DOMAIN"]="$N3_IP"
  ["ctrl.$DOMAIN"]="$N4_IP"
)
for FQDN in "${!EXPECTED_IPS[@]}"; do
  WANT="${EXPECTED_IPS[$FQDN]}"
  for i in $(seq 1 24); do
    GOT=$(dig +short A "$FQDN" @1.1.1.1 2>/dev/null | head -1)
    if [ "$GOT" = "$WANT" ]; then
      step "  $FQDN → $GOT ✓"
      break
    fi
    [ "$i" -lt 24 ] || die "DNS did not propagate for $FQDN after 120s (got '$GOT', want '$WANT')"
    sleep 5
  done
done

# ─── Phase 4: TLS — Let's Encrypt on all four nodes (parallel) ───────────────

log "PHASE 4 — TLS (Let's Encrypt on N1/N2/N3/N4 in parallel)"

PIDS_P4=()
NODES_P4=("$N1_IP" "$N2_IP" "$N3_IP" "$N4_IP")

for NODE_IP in "${NODES_P4[@]}"; do
  step "tls.sh on $NODE_IP..."
  # Use deploy + sudo (root SSH disabled by base.sh hardening)
  # Runtime nodes need CLOUDFLARE_API_TOKEN for the DNS-01 challenge (api.<DOMAIN> SAN).
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o IdentitiesOnly=yes \
    -i "$SSH_PRIVATE_KEY_PATH" \
    "deploy@$NODE_IP" \
    "ADMIN_EMAIL=$ADMIN_EMAIL CLOUDFLARE_API_TOKEN=$CLOUDFLARE_API_TOKEN sudo -E bash -s" \
    < "$SCRIPT_DIR/bootstrap/tls.sh" &
  PIDS_P4+=($!)
done

FAIL=0
for i in "${!PIDS_P4[@]}"; do
  wait "${PIDS_P4[$i]}" || { echo "  ✗ tls.sh failed on ${NODES_P4[$i]}" >&2 ; FAIL=1 ; }
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
echo "    Dashboard:   https://oilfield-dash.pages.dev"
echo ""
echo "  Quick verify:"
echo "    oilfield-dash status"
echo "    oilfield-dash prices crude"
echo "    oilfield-dash news"
echo ""
echo "  To tear down: ./infra/teardown/teardown-all.sh"
