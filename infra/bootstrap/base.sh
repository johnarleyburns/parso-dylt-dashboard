#!/usr/bin/env bash
set -euo pipefail

# Called by provision scripts via:
#   NODE_NAME=<n1|n2|n3|n4> NODE_ROLE=<runtime|control> DOMAIN=<domain> bash -s

NODE_NAME="${NODE_NAME:?NODE_NAME must be set}"
NODE_ROLE="${NODE_ROLE:?NODE_ROLE must be set}"
DOMAIN="${DOMAIN:?DOMAIN must be set}"

log() { echo "[base] $*" >&2; }
die() { echo "[base] ERROR: $*" >&2; exit 1; }

[ "$EUID" -eq 0 ] || die "Must run as root"

# Update and install base packages
log "Updating packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq
apt-get install -y -qq \
  curl jq git wget unzip \
  nginx fail2ban ufw \
  certbot python3-certbot-nginx \
  ca-certificates gnupg

# Set hostname
log "Setting hostname to $NODE_NAME.$DOMAIN..."
hostnamectl set-hostname "$NODE_NAME.$DOMAIN"
echo "127.0.1.1 $NODE_NAME.$DOMAIN $NODE_NAME" >> /etc/hosts

# Create deploy user
log "Creating deploy user..."
if ! id deploy &>/dev/null; then
  useradd -m -s /bin/bash deploy
fi
echo "deploy ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/deploy
chmod 440 /etc/sudoers.d/deploy

# Copy authorized_keys from root so deploy user can SSH in with the same key
mkdir -p /home/deploy/.ssh
if [ -f /root/.ssh/authorized_keys ]; then
  cp /root/.ssh/authorized_keys /home/deploy/.ssh/authorized_keys
  chown -R deploy:deploy /home/deploy/.ssh
  chmod 700 /home/deploy/.ssh
  chmod 600 /home/deploy/.ssh/authorized_keys
fi

# SSH hardening — drop root login after deploy user is ready
log "Hardening SSH..."
cat > /etc/ssh/sshd_config.d/99-oilfield.conf << 'EOF'
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
AllowUsers deploy
ChallengeResponseAuthentication no
MaxAuthTries 3
LoginGraceTime 30
EOF
systemctl try-reload-or-restart ssh 2>/dev/null || systemctl try-reload-or-restart sshd 2>/dev/null || true

# UFW — deny all in, allow all out, then open per-role ports
log "Configuring UFW..."
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp  comment 'SSH'
ufw allow 80/tcp  comment 'HTTP (certbot + redirect)'
ufw allow 443/tcp comment 'HTTPS'

if [ "$NODE_ROLE" = "runtime" ]; then
  ufw allow 2379/tcp comment 'etcd client'
  ufw allow 2380/tcp comment 'etcd peer'
  ufw allow 8080/tcp comment 'oilfield-api'
fi

if [ "$NODE_ROLE" = "control" ]; then
  ufw allow 8090/tcp comment 'oilfield-dashboard backend'
fi

ufw --force enable

# fail2ban — default SSH jail is sufficient
systemctl enable fail2ban
systemctl start fail2ban

# Write node config — read by all subsequent bootstrap scripts and systemd units
log "Writing /etc/daylight/node.conf..."
mkdir -p /etc/daylight
cat > /etc/daylight/node.conf << EOF
NODE_NAME=$NODE_NAME
NODE_ROLE=$NODE_ROLE
DOMAIN=$DOMAIN
EOF
chmod 644 /etc/daylight/node.conf

# Create binary home for oilfield binaries
mkdir -p /opt/oilfield/bin

log "Base bootstrap complete — $NODE_NAME ($NODE_ROLE) at $DOMAIN"
