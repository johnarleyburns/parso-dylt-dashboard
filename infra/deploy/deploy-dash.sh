#!/usr/bin/env bash
# deploy-dash.sh — Build and deploy the web dashboard.
#   Frontend → Cloudflare Pages (wrangler)
#   Backend  → N4/UpCloud control node (SSH)
# Run from repo root.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/infra/.env"

# Use IP directly to avoid DNS propagation race (ctrl hostname may still resolve to old IP)
N4_IP="$(cat "$REPO_ROOT/infra/state/upcloud.ip" 2>/dev/null || echo "")"
[ -n "$N4_IP" ] || { echo "ERROR: infra/state/upcloud.ip missing" >&2; exit 1; }
CTRL_HOST="deploy@$N4_IP"
FRONTEND_DIR="$REPO_ROOT/dashboard/web/frontend"
BACKEND_DIR="$REPO_ROOT/dashboard/web/backend"

# ---- 1. Build and deploy Go backend to N4 ----
echo "==> Building oilfield-dash-web (Go backend)..."
(cd "$BACKEND_DIR" && GOARCH=amd64 GOOS=linux go build -o /tmp/oilfield-dash-web .)

echo "==> Deploying backend to ctrl ($CTRL_HOST)..."
# Copy binary and the systemd unit file together
scp -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" \
  /tmp/oilfield-dash-web \
  "$BACKEND_DIR/systemd/oilfield-dash.service" \
  "${CTRL_HOST}:/tmp/"

ssh -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -i "$SSH_PRIVATE_KEY_PATH" "$CTRL_HOST" bash -s <<'REMOTE'
  set -euo pipefail
  sudo mv /tmp/oilfield-dash-web /opt/oilfield/bin/oilfield-dash-web
  sudo chmod +x /opt/oilfield/bin/oilfield-dash-web
  # Always (re)install service unit so config updates are picked up
  sudo mv /tmp/oilfield-dash.service /etc/systemd/system/oilfield-dash.service
  sudo chmod 644 /etc/systemd/system/oilfield-dash.service
  sudo systemctl daemon-reload
  sudo systemctl enable oilfield-dash
  sudo systemctl restart oilfield-dash
  echo "  oilfield-dash-web restarted on $(hostname)"
REMOTE

rm -f /tmp/oilfield-dash-web
echo "  Backend deployed."

# ---- 2. Build and deploy frontend to Cloudflare Pages ----
echo ""
echo "==> Building React frontend..."
(cd "$FRONTEND_DIR" && npm ci && npm run build)

echo "==> Deploying frontend to Cloudflare Pages (project: ${CF_PAGES_PROJECT})..."
CLOUDFLARE_API_TOKEN="$CLOUDFLARE_API_TOKEN" \
CLOUDFLARE_ACCOUNT_ID="$CF_ACCOUNT_ID" \
  npx wrangler pages deploy "$FRONTEND_DIR/dist" \
    --project-name "$CF_PAGES_PROJECT" \
    --commit-dirty=true

echo ""
echo "==> Dashboard deployed."
echo "    Frontend: https://oilfield-dash.pages.dev  (or custom domain if configured in CF Pages)"
echo "    Backend:  https://ctrl.${DOMAIN}"
