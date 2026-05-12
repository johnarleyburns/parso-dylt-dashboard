#!/usr/bin/env bash
set -euo pipefail

# ionos-bootstrap.sh — Bootstrap N3 from a manually created Ionos VPS.
#
# This script is called by up.sh in place of a provisioning API call, because
# the Ionos consumer VPS product (ionos.com/servers/vps) has no REST provisioning
# API — VMs must be created manually at my.ionos.com.
#
# Prerequisites (set in infra/.env before running up.sh):
#   IONOS_VPS_IP       — public IP of the VPS (shown in my.ionos.com dashboard)
#   IONOS_VPS_PASSWORD — root password set during VPS creation (used once to
#                        install the oilfield SSH key; key-only auth after that)
#
# What this script does:
#   1. Installs the oilfield SSH public key onto the VPS root account
#   2. Writes IONOS_VPS_IP to infra/state/ionos.ip (so dns.sh and deploy-app.sh can read it)
#   3. Verifies SSH key auth works
#   4. Runs base.sh bootstrap over SSH (packages, deploy user, UFW, /etc/daylight/node.conf)
#
# The remaining phases (daylight.sh, dns.sh, tls.sh, deploy-app.sh) are run by
# up.sh in the same way as all other nodes.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env"

NODE_IP="${IONOS_VPS_IP:?IONOS_VPS_IP must be set in infra/.env}"
NODE_PASSWORD="${IONOS_VPS_PASSWORD:-}"

log() { echo "[ionos-bootstrap] $*" >&2; }
die() { echo "[ionos-bootstrap] ERROR: $*" >&2; exit 1; }

SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=15 -o IdentitiesOnly=yes -i $SSH_PRIVATE_KEY_PATH"

log "N3 Ionos VPS at $NODE_IP"

# ── 1. Install SSH key (using password if key not yet installed) ───────────────

if ssh $SSH_OPTS -o BatchMode=yes "root@$NODE_IP" true 2>/dev/null; then
  log "SSH key already accepted — skipping password install"
else
  [ -n "$NODE_PASSWORD" ] || die "SSH key auth failed and IONOS_VPS_PASSWORD is not set.
  Either add IONOS_VPS_PASSWORD to infra/.env or manually install the SSH key on the VPS."

  log "Installing SSH public key via password auth..."

  # Write SSH_ASKPASS helper
  ASKPASS=$(mktemp)
  chmod 700 "$ASKPASS"
  printf '#!/bin/sh\necho "%s"\n' "$NODE_PASSWORD" > "$ASKPASS"

  PUBKEY="$(cat "$SSH_PUBLIC_KEY_PATH")"
  SSH_ASKPASS="$ASKPASS" DISPLAY=:0 setsid ssh \
    -o StrictHostKeyChecking=no -o ConnectTimeout=15 \
    -o NumberOfPasswordPrompts=1 \
    "root@$NODE_IP" \
    "mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '$PUBKEY' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys" \
    2>/dev/null || die "Failed to install SSH key — check IONOS_VPS_PASSWORD"

  rm -f "$ASKPASS"
  log "SSH key installed"
fi

# Verify key auth now works
ssh $SSH_OPTS -o BatchMode=yes "root@$NODE_IP" true \
  || die "SSH key auth still failing after install attempt"
log "SSH key auth confirmed"

# ── 2. Write state file ────────────────────────────────────────────────────────

mkdir -p "$SCRIPT_DIR/../state"
echo "$NODE_IP" > "$SCRIPT_DIR/../state/ionos.ip"
log "Wrote $NODE_IP to infra/state/ionos.ip"

# ── 3. Run base.sh bootstrap ──────────────────────────────────────────────────

log "Running base.sh on $NODE_IP..."
ssh $SSH_OPTS "root@$NODE_IP" \
  "NODE_NAME=n3 NODE_ROLE=runtime DOMAIN=$DOMAIN bash -s" \
  < "$SCRIPT_DIR/../bootstrap/base.sh" \
  || die "base.sh failed on $NODE_IP"

log "Done — N3/Ionos bootstrapped at $NODE_IP"
