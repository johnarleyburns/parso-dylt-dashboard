#!/usr/bin/env bash
set -euo pipefail

# verify-deploy.sh — post-deploy smoke test. Asserts oilfield is up AND that the
# co-hosted crm (fast-insurance-portal) is still healthy.
#
#   bash deploy/scripts/verify-deploy.sh root@217.76.158.90

HOST="${1:?usage: verify-deploy.sh root@HOST}"
SSH="ssh -o ConnectTimeout=15 $HOST"
fail=0

echo "== oilfield service =="
$SSH 'systemctl is-active oilfield' | grep -q active \
  && echo "  oilfield.service: active" || { echo "  oilfield.service: NOT active"; fail=1; }

echo "== oilfield API (:8444) =="
if $SSH 'curl -skf https://127.0.0.1:8444/api/v1/health' | grep -q '"status"'; then
  echo "  /api/v1/health: OK"
else
  echo "  /api/v1/health: FAIL"; fail=1
fi

echo "== oilfield via nginx SNI =="
$SSH 'curl -skf --resolve oilfield-dash.parso.guru:443:127.0.0.1 https://oilfield-dash.parso.guru/api/v1/health >/dev/null' \
  && echo "  SNI route oilfield-dash.parso.guru: OK" || { echo "  SNI route: FAIL"; fail=1; }

echo "== crm untouched (fast-insurance-portal) =="
$SSH 'systemctl is-active crm' | grep -q active \
  && echo "  crm.service: active" || { echo "  crm.service: NOT active"; fail=1; }
$SSH 'curl -skf --resolve fast.arleyxu.com:443:127.0.0.1 https://fast.arleyxu.com/ >/dev/null' \
  && echo "  crm via nginx SNI: OK" || { echo "  crm via nginx SNI: FAIL"; fail=1; }

echo "== memory headroom =="
$SSH "free -h | awk 'NR==1||NR==2'"

if [ "$fail" -ne 0 ]; then
  echo "VERIFY: FAILED"; exit 1
fi
echo "VERIFY: all checks passed"
