#!/usr/bin/env bash
set -euo pipefail

# cohost-bootstrap.sh — one-time setup to co-host oilfield-solo on the EXISTING
# IONOS box that already runs fast-insurance-portal (crm), WITHOUT disrupting crm.
#
# Idempotent. Run from your Mac:
#     bash deploy/scripts/cohost-bootstrap.sh root@217.76.158.90
#
# It:
#   1. verifies crm is healthy (before)
#   2. creates the oilfield user, /opt/oilfield tree, self-signed cert, swapfile
#   3. installs the systemd unit (with memory/cpu caps) — enabled, not started
#   4. installs nginx + the SNI stream config
#   5. CUTS OVER :443 from the iptables redirect to nginx (reversible)
#   6. verifies crm is STILL healthy (after)
#
# The oilfield binary itself is shipped separately by `make deploy`.

HOST="${1:?usage: cohost-bootstrap.sh root@HOST}"
SSH="ssh -o ConnectTimeout=15 $HOST"

echo "==> [1/6] pre-flight: crm health"
$SSH 'set -e
  systemctl is-active crm >/dev/null && echo "crm: active" || { echo "crm not active — aborting"; exit 1; }
  curl -skf https://127.0.0.1:8443/ >/dev/null && echo "crm :8443 responds" || echo "warn: crm :8443 no HTTP 2xx (may still be fine)"
'

echo "==> [2/6] oilfield user, dirs, cert, swap"
$SSH 'set -e
  id oilfield >/dev/null 2>&1 || useradd --system --home /opt/oilfield --shell /usr/sbin/nologin oilfield
  mkdir -p /opt/oilfield/bin /opt/oilfield/data/certs
  if [ ! -f /opt/oilfield/data/certs/cert.pem ]; then
    openssl req -x509 -newkey rsa:4096 -keyout /opt/oilfield/data/certs/key.pem \
      -out /opt/oilfield/data/certs/cert.pem -days 3650 -nodes -subj "/CN=oilfield-dash.parso.guru"
  fi
  chown -R oilfield:oilfield /opt/oilfield
  chmod 750 /opt/oilfield /opt/oilfield/data
  if [ ! -f /swapfile ]; then
    fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
    grep -q "/swapfile" /etc/fstab || echo "/swapfile none swap sw 0 0" >> /etc/fstab
    echo "2G swapfile added"
  else
    echo "swapfile present"
  fi
'

echo "==> [2b/6] /opt/oilfield/env (only if missing — never overwrites secrets)"
$SSH 'set -e
  if [ ! -f /opt/oilfield/env ]; then
    cat > /opt/oilfield/env <<EOF
NODE_NAME=solo
NODE_PROVIDER=ionos
ADDR=:8444
DB_PATH=/opt/oilfield/data/oilfield.db
TLS_CERT=/opt/oilfield/data/certs/cert.pem
TLS_KEY=/opt/oilfield/data/certs/key.pem
SCRAPE_INTERVAL=300
SCRAPE_CONCURRENCY=3
CORS_ORIGIN=https://oilfield-dash.parso.guru
EIA_API_KEY=CHANGE_ME
OILPRICE_API_KEY=
EOF
    chmod 640 /opt/oilfield/env && chown root:oilfield /opt/oilfield/env
    echo "WROTE /opt/oilfield/env — set EIA_API_KEY before starting!"
  else
    echo "/opt/oilfield/env exists — left untouched"
  fi
'

echo "==> [3/6] systemd unit"
$SSH 'cat > /etc/systemd/system/oilfield.service' < deploy/systemd/oilfield.service
$SSH 'systemctl daemon-reload && systemctl enable oilfield >/dev/null 2>&1 && echo "oilfield.service enabled"'

echo "==> [4/6] nginx + SNI stream config"
$SSH 'set -e
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  # nginx + the stream module (ssl_preread lives here; it is a separate package on Ubuntu)
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx libnginx-mod-stream >/dev/null
  mkdir -p /etc/nginx/stream.d
  ls /etc/nginx/modules-enabled/*stream* >/dev/null 2>&1 || echo "warn: stream module not in modules-enabled"
'
$SSH 'cat > /etc/nginx/stream.d/oilfield.conf' < deploy/nginx/stream-oilfield.conf
$SSH 'set -e
  # add the stream include to nginx.conf once (top level, outside http{})
  if ! grep -q "stream.d/oilfield.conf" /etc/nginx/nginx.conf; then
    printf "\nstream { include /etc/nginx/stream.d/oilfield.conf; }\n" >> /etc/nginx/nginx.conf
  fi
  nginx -t
'

echo "==> [5/6] :443 cutover (iptables redirect -> nginx). Reversible."
$SSH 'set -e
  systemctl restart nginx
  # remove the blunt 443->8443 redirect so nginx (SNI) receives :443
  if iptables -t nat -C PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443 2>/dev/null; then
    iptables -t nat -D PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443
    echo "removed iptables 443->8443 redirect"
  else
    echo "no 443->8443 redirect present (already cut over?)"
  fi
  command -v netfilter-persistent >/dev/null 2>&1 && netfilter-persistent save || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
'

echo "==> [6/6] post-flight: crm still healthy via nginx SNI"
$SSH 'set -e
  sleep 1
  curl -skf --resolve fast.arleyxu.com:443:127.0.0.1 https://fast.arleyxu.com/ >/dev/null \
    && echo "OK: crm reachable through nginx SNI (fast.arleyxu.com)" \
    || echo "WARN: crm SNI check failed — ROLLBACK: iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443 && systemctl stop nginx"
'

echo
echo "Bootstrap complete. Next:"
echo "  1) ssh $HOST 'nano /opt/oilfield/env'   # set EIA_API_KEY"
echo "  2) make deploy                          # ship the binary + start oilfield"
echo "  3) point oilfield-dash.parso.guru (proxied A) -> this box's IP in Cloudflare"
