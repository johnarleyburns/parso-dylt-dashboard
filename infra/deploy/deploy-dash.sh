#!/usr/bin/env bash
# deploy-dash.sh — Build and deploy the web dashboard.
#   Frontend → Cloudflare Pages (wrangler)
#   Backend  → N4/UpCloud control node (SSH)
# Run from repo root.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/infra/.env"

CTRL_HOST="deploy@ctrl.${DOMAIN}"
FRONTEND_DIR="$REPO_ROOT/dashboard/web/frontend"
BACKEND_DIR="$REPO_ROOT/dashboard/web/backend"

# ---- 1. Build and deploy Go backend to N4 ----
echo "==> Building oilfield-dash-web (Go backend)..."
(cd "$BACKEND_DIR" && GOARCH=amd64 GOOS=linux go build -o /tmp/oilfield-dash-web .)

echo "==> Deploying backend to ctrl ($CTRL_HOST)..."
scp -i "$SSH_PRIVATE_KEY_PATH" /tmp/oilfield-dash-web "${CTRL_HOST}:/tmp/"

ssh -i "$SSH_PRIVATE_KEY_PATH" "$CTRL_HOST" bash -s <<'REMOTE'
  set -euo pipefail
  sudo mv /tmp/oilfield-dash-web /opt/oilfield/bin/oilfield-dash-web
  sudo chmod +x /opt/oilfield/bin/oilfield-dash-web
  # Install service unit if not already present
  if [ ! -f /etc/systemd/system/oilfield-dash.service ]; then
    echo "  Installing systemd service..."
  fi
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
  npx wrangler pages deploy "$FRONTEND_DIR/dist" \
    --project-name "$CF_PAGES_PROJECT" \
    --commit-dirty=true

echo ""
echo "==> Dashboard deployed."
echo "    Frontend: https://dash.${DOMAIN}"
echo "    Backend:  https://ctrl.${DOMAIN}"
