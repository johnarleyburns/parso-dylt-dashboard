#!/usr/bin/env bash
set -euo pipefail

# Installs etcd (pinned binary), writes cluster config, and drops systemd unit stubs.
# Run on N1, N2, N3 ONLY (runtime nodes) after ALL THREE nodes are provisioned.
#
# Required env vars (pass before bash -s):
#   N1_IP, N2_IP, N3_IP     — public IPs from infra/state/*.ip
#   ETCD_CLUSTER_TOKEN       — from infra/.env
#   DOMAIN                   — from infra/.env
#
# NODE_NAME and NODE_ROLE are read from /etc/daylight/node.conf (written by base.sh).

source /etc/daylight/node.conf

N1_IP="${N1_IP:?N1_IP must be set}"
N2_IP="${N2_IP:?N2_IP must be set}"
N3_IP="${N3_IP:?N3_IP must be set}"
ETCD_CLUSTER_TOKEN="${ETCD_CLUSTER_TOKEN:?ETCD_CLUSTER_TOKEN must be set}"
DOMAIN="${DOMAIN:-$DOMAIN}"

log() { echo "[daylight] $*" >&2; }
die() { echo "[daylight] ERROR: $*" >&2; exit 1; }

[ "$EUID" -eq 0 ] || die "Must run as root"
[ "$NODE_ROLE" = "runtime" ] || die "daylight.sh is for runtime nodes only (NODE_ROLE=runtime)"

ETCD_VERSION="3.5.18"
ETCD_ARCH="amd64"
ETCD_TARBALL="etcd-v${ETCD_VERSION}-linux-${ETCD_ARCH}.tar.gz"
ETCD_URL="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/${ETCD_TARBALL}"

# Resolve this node's IP from the cluster map
case "$NODE_NAME" in
  n1) NODE_IP="$N1_IP" ;;
  n2) NODE_IP="$N2_IP" ;;
  n3) NODE_IP="$N3_IP" ;;
  *)  die "Unknown NODE_NAME: $NODE_NAME" ;;
esac

# Install etcd pinned binary (never use apt — version skew risk)
log "Downloading etcd v$ETCD_VERSION..."
cd /tmp
curl -fsSL "$ETCD_URL" -o "$ETCD_TARBALL"
tar xzf "$ETCD_TARBALL"
install -m 755 "etcd-v${ETCD_VERSION}-linux-${ETCD_ARCH}/etcd"    /usr/local/bin/etcd
install -m 755 "etcd-v${ETCD_VERSION}-linux-${ETCD_ARCH}/etcdctl" /usr/local/bin/etcdctl
rm -rf "etcd-v${ETCD_VERSION}-linux-${ETCD_ARCH}" "$ETCD_TARBALL"
log "etcd $(etcd --version | head -1) installed"

# etcd user and directories
if ! id etcd &>/dev/null; then
  useradd -r -s /sbin/nologin -d /var/lib/etcd etcd
fi
mkdir -p /var/lib/etcd /etc/etcd
chown etcd:etcd /var/lib/etcd
chmod 700 /var/lib/etcd

# etcd cluster config
log "Writing /etc/etcd/etcd.conf.yml for $NODE_NAME..."
mkdir -p /etc/etcd
cat > /etc/etcd/etcd.conf.yml << EOF
name: '$NODE_NAME'
data-dir: /var/lib/etcd

listen-peer-urls: 'http://${NODE_IP}:2380'
listen-client-urls: 'http://${NODE_IP}:2379,http://127.0.0.1:2379'

advertise-client-urls: 'http://${NODE_IP}:2379'
initial-advertise-peer-urls: 'http://${NODE_IP}:2380'

initial-cluster: 'n1=http://${N1_IP}:2380,n2=http://${N2_IP}:2380,n3=http://${N3_IP}:2380'
initial-cluster-token: '$ETCD_CLUSTER_TOKEN'
initial-cluster-state: 'new'

# Heartbeat and election tuning for WAN peers (US-EU latency)
heartbeat-interval: 250
election-timeout: 2500

# Auto-compaction: keep only last 1 hour of revisions
auto-compaction-mode: 'periodic'
auto-compaction-retention: '1h'

log-level: 'warn'
EOF
chmod 640 /etc/etcd/etcd.conf.yml
chown root:etcd /etc/etcd/etcd.conf.yml

# etcd systemd unit
log "Writing etcd.service..."
cat > /etc/systemd/system/etcd.service << 'EOF'
[Unit]
Description=etcd — oilfield cluster K/V store
Documentation=https://etcd.io/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
User=etcd
Group=etcd
ExecStart=/usr/local/bin/etcd --config-file /etc/etcd/etcd.conf.yml
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
TimeoutStartSec=120

[Install]
WantedBy=multi-user.target
EOF

# oilfield-api stub — binary deployed in Phase 3
log "Writing oilfield-api.service stub..."
cat > /etc/systemd/system/oilfield-api.service << 'EOF'
[Unit]
Description=oilfield REST API
After=network-online.target etcd.service
Requires=etcd.service

[Service]
Type=simple
User=deploy
EnvironmentFile=/etc/daylight/node.conf
ExecStart=/opt/oilfield/bin/oilfield-api
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# oilfield-scraper stub — oneshot; driven by timer below
log "Writing oilfield-scraper.service + timer stubs..."
cat > /etc/systemd/system/oilfield-scraper.service << 'EOF'
[Unit]
Description=oilfield market data scraper (oneshot)
After=network-online.target etcd.service
Requires=etcd.service

[Service]
Type=oneshot
User=deploy
EnvironmentFile=/etc/daylight/node.conf
ExecStart=/opt/oilfield/bin/oilfield-scraper
EOF

cat > /etc/systemd/system/oilfield-scraper.timer << 'EOF'
[Unit]
Description=Run oilfield-scraper every 5 minutes
Requires=etcd.service

[Timer]
OnBootSec=60s
OnUnitActiveSec=5min
AccuracySec=10s

[Install]
WantedBy=timers.target
EOF

# Enable etcd — stubs are enabled after Phase 3 deploys the binaries
systemctl daemon-reload
systemctl enable etcd.service
systemctl start etcd.service

log "Waiting for etcd to start on $NODE_NAME..."
for i in $(seq 1 30); do
  if etcdctl --endpoints="http://127.0.0.1:2379" endpoint health &>/dev/null; then
    log "etcd is healthy"
    break
  fi
  [ "$i" -lt 30 ] || die "etcd did not start within 150s — check: journalctl -u etcd"
  sleep 5
done

log "daylight.sh complete — $NODE_NAME is an etcd cluster member"
log "Run the other nodes' daylight.sh before etcd reaches quorum."
