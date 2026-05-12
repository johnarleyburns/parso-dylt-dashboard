#!/usr/bin/env bash
set -euo pipefail

# Obtains a Let's Encrypt TLS cert for this node's FQDN and configures nginx.
# Run on each node (N1, N2, N3, N4) AFTER dns.sh has been run and DNS has propagated.
#
# NODE_NAME, NODE_ROLE, and DOMAIN are read from /etc/daylight/node.conf (written by base.sh).
# Required env vars:
#   ADMIN_EMAIL         — Let's Encrypt registration email
#   CLOUDFLARE_API_TOKEN — Required for runtime nodes (DNS-01 challenge for api.<DOMAIN> SAN)
#
# Runtime nodes (N1/N2/N3) use DNS-01 via Cloudflare because api.<DOMAIN> is a round-robin
# A record — webroot challenges would fail when LE's secondary validator hits a different node
# than the one that wrote the challenge file.
# Control node (N4) uses webroot (single A record, no round-robin issue).

source /etc/daylight/node.conf

ADMIN_EMAIL="${ADMIN_EMAIL:?ADMIN_EMAIL must be set}"

log() { echo "[tls] $*" >&2; }
die() { echo "[tls] ERROR: $*" >&2; exit 1; }

[ "$EUID" -eq 0 ] || die "Must run as root"

# Control node is publicly reachable as ctrl.<DOMAIN>, not n4.<DOMAIN>
if [ "${NODE_ROLE}" = "control" ]; then
  NODE_FQDN="ctrl.${DOMAIN}"
else
  NODE_FQDN="${NODE_NAME}.${DOMAIN}"
  # Runtime nodes also cover api.<DOMAIN> as a SAN so the CLI/browser TLS checks pass
  # when hitting https://api.<DOMAIN> and being routed to this node.
  CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN must be set for runtime nodes}"
fi
NGINX_CONF=/etc/nginx/sites-available/oilfield
CERT_DIR="/etc/letsencrypt/live/${NODE_FQDN}"

command -v certbot &>/dev/null || die "certbot not found — was base.sh run?"
command -v nginx   &>/dev/null || die "nginx not found — was base.sh run?"

# Write minimal HTTP config so nginx is up (needed for redirect + control webroot challenge)
log "Writing initial nginx config for ${NODE_FQDN}..."
mkdir -p /var/www/certbot

if [ "${NODE_ROLE}" = "runtime" ]; then
  # Runtime: nginx serves both nX.<DOMAIN> and api.<DOMAIN> on port 80
  printf 'server {\n    listen 80;\n    server_name %s api.%s;\n    location /.well-known/acme-challenge/ { root /var/www/certbot; }\n    location / { return 301 https://$host$request_uri; }\n}\n' \
    "${NODE_FQDN}" "${DOMAIN}" > "${NGINX_CONF}"
else
  printf 'server {\n    listen 80;\n    server_name %s;\n    location /.well-known/acme-challenge/ { root /var/www/certbot; }\n    location / { return 301 https://$host$request_uri; }\n}\n' \
    "${NODE_FQDN}" > "${NGINX_CONF}"
fi

ln -sf "${NGINX_CONF}" /etc/nginx/sites-enabled/oilfield
rm -f /etc/nginx/sites-enabled/default
nginx -t || die "nginx config test failed"
systemctl reload nginx || systemctl start nginx

# Obtain cert — method differs by role
if [ "${NODE_ROLE}" = "runtime" ]; then
  # Install Cloudflare DNS plugin (Ubuntu 24.04 package, no pip needed)
  log "Installing certbot-dns-cloudflare for DNS-01 challenge..."
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq python3-certbot-dns-cloudflare

  CF_CREDS=$(mktemp)
  chmod 600 "$CF_CREDS"
  printf 'dns_cloudflare_api_token = %s\n' "${CLOUDFLARE_API_TOKEN}" > "$CF_CREDS"

  log "Obtaining Let's Encrypt cert for ${NODE_FQDN} + api.${DOMAIN} (DNS-01)..."
  certbot certonly \
    --dns-cloudflare \
    --dns-cloudflare-credentials "$CF_CREDS" \
    --dns-cloudflare-propagation-seconds 30 \
    --non-interactive \
    --agree-tos \
    --email "${ADMIN_EMAIL}" \
    -d "${NODE_FQDN}" \
    -d "api.${DOMAIN}"

  rm -f "$CF_CREDS"
else
  log "Obtaining Let's Encrypt cert for ${NODE_FQDN} (webroot)..."
  certbot certonly \
    --webroot \
    --webroot-path /var/www/certbot \
    --non-interactive \
    --agree-tos \
    --email "${ADMIN_EMAIL}" \
    -d "${NODE_FQDN}"
fi

# Write full HTTPS config — proxy target differs by node role
log "Writing HTTPS nginx config (NODE_ROLE=${NODE_ROLE})..."

write_nginx_runtime() {
  cat > "${NGINX_CONF}" << EOF
server {
    listen 80;
    server_name ${NODE_FQDN} api.${DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen 443 ssl;
    server_name ${NODE_FQDN} api.${DOMAIN};

    ssl_certificate     ${CERT_DIR}/fullchain.pem;
    ssl_certificate_key ${CERT_DIR}/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    root /opt/oilfield/static;
    index index.html;
    location / { try_files \$uri \$uri/ /index.html; }

    location /api/ {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_set_header   X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
    }
}
EOF
}

write_nginx_control() {
  cat > "${NGINX_CONF}" << EOF
server {
    listen 80;
    server_name ${NODE_FQDN};
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen 443 ssl;
    server_name ${NODE_FQDN};

    ssl_certificate     ${CERT_DIR}/fullchain.pem;
    ssl_certificate_key ${CERT_DIR}/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    location /api/ {
        proxy_pass         http://127.0.0.1:8090;
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_set_header   X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        # Forward Origin so the Go backend can match it against DASH_ORIGIN and set ACAO.
        proxy_set_header   Origin \$http_origin;
    }
}
EOF
}

if [ "${NODE_ROLE}" = "runtime" ]; then
  write_nginx_runtime
else
  write_nginx_control
fi

nginx -t || die "HTTPS nginx config test failed"
systemctl reload nginx

# Auto-renewal via systemd timer (Daylight principle: no cron)
log "Writing certbot-renew systemd timer..."

cat > /etc/systemd/system/certbot-renew.service << 'EOF'
[Unit]
Description=Certbot renewal for oilfield TLS certs
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/bin/certbot renew --quiet --post-hook "systemctl reload nginx"
EOF

cat > /etc/systemd/system/certbot-renew.timer << 'EOF'
[Unit]
Description=Run certbot renewal twice daily

[Timer]
OnCalendar=*-*-* 03:00:00
OnCalendar=*-*-* 15:00:00
RandomizedDelaySec=1800
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable certbot-renew.timer
systemctl start certbot-renew.timer

log "TLS setup complete for ${NODE_FQDN}$([ "${NODE_ROLE}" = "runtime" ] && echo " (SAN: api.${DOMAIN})" || true)"
log "  cert:    ${CERT_DIR}/fullchain.pem"
log "  renewal: certbot-renew.timer (03:00 + 15:00 daily, +-30min jitter)"
