#!/usr/bin/env bash
set -euo pipefail

# Obtains a Let's Encrypt TLS cert for this node's FQDN and configures nginx.
# Run on each node (N1, N2, N3, N4) AFTER dns.sh has been run and DNS has propagated.
#
# NODE_NAME, NODE_ROLE, and DOMAIN are read from /etc/daylight/node.conf (written by base.sh).
# ADMIN_EMAIL is the Let's Encrypt registration email — pass as an env var:
#   ADMIN_EMAIL=you@example.com bash -s < tls.sh

source /etc/daylight/node.conf

ADMIN_EMAIL="${ADMIN_EMAIL:?ADMIN_EMAIL must be set}"

log() { echo "[tls] $*" >&2; }
die() { echo "[tls] ERROR: $*" >&2; exit 1; }

[ "$EUID" -eq 0 ] || die "Must run as root"

NODE_FQDN="${NODE_NAME}.${DOMAIN}"
NGINX_CONF=/etc/nginx/sites-available/oilfield
CERT_DIR="/etc/letsencrypt/live/${NODE_FQDN}"

command -v certbot &>/dev/null || die "certbot not found — was base.sh run?"
command -v nginx   &>/dev/null || die "nginx not found — was base.sh run?"

# Write minimal HTTP config for webroot challenge
log "Writing initial nginx config for ${NODE_FQDN}..."
mkdir -p /var/www/certbot

printf 'server {\n    listen 80;\n    server_name %s;\n    location /.well-known/acme-challenge/ { root /var/www/certbot; }\n    location / { return 301 https://$host$request_uri; }\n}\n' \
  "${NODE_FQDN}" > "${NGINX_CONF}"

ln -sf "${NGINX_CONF}" /etc/nginx/sites-enabled/oilfield
rm -f /etc/nginx/sites-enabled/default
nginx -t || die "nginx config test failed"
systemctl reload nginx || systemctl start nginx

# Obtain cert
log "Obtaining Let's Encrypt cert for ${NODE_FQDN}..."
certbot certonly \
  --webroot \
  --webroot-path /var/www/certbot \
  --non-interactive \
  --agree-tos \
  --email "${ADMIN_EMAIL}" \
  -d "${NODE_FQDN}"

# Write full HTTPS config — proxy target differs by node role
log "Writing HTTPS nginx config (NODE_ROLE=${NODE_ROLE})..."

write_nginx_runtime() {
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
        add_header         Access-Control-Allow-Origin "https://dash.${DOMAIN}";
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

log "TLS setup complete for ${NODE_FQDN}"
log "  cert:    ${CERT_DIR}/fullchain.pem"
log "  renewal: certbot-renew.timer (03:00 + 15:00 daily, +-30min jitter)"
