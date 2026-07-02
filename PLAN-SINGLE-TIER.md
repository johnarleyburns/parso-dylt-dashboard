# PLAN-SINGLE-TIER.md — Oilfield "Solo" Single-Server Redesign (Co-hosted)

> **Codename:** `oilfield-solo`
> **Goal:** Collapse the 4-node, 4-provider, etcd-clustered `oilfield` platform into a
> single Go binary that runs 100% locally and **co-hosts on the EXISTING IONOS server
> alongside `fast-insurance-portal`, without impacting its operation.**
> **Baseline tag:** `multi-tier-v1.0.0` (current HEAD — the multi-tiered release).
> **Status:** DRAFT FOR REVIEW. No code/server changed yet except the git tag.

---

## 0. Decisions locked in (from review)

| Decision | Choice |
|---|---|
| Data store | **SQLite (WAL)**, single file. Replaces the 3-node etcd cluster. |
| Backups | **None.** Prices are re-pullable; on data loss the scraper repopulates within 5 min. No Litestream, no R2. |
| Domain | **Reuse `oilfield-dash.parso.guru`** — overwrite the existing record to point at the IONOS server. |
| Frontend hosting | **Served by the Go binary** (Go `embed` of the built React bundle). |
| Cloudflare Pages | **Removed** — delete the `oilfield-dash` Pages project + its managed CNAME. |
| Server | **CO-HOST on the existing IONOS VPS `217.76.158.90`** (already runs `fast-insurance-portal`). **No new server.** |
| 443 routing | **LOCKED: nginx `stream` + `ssl_preread` (SNI-based, no TLS termination)** — see §6.2. |
| Deploy vehicle | **Colima** (x86_64 Linux VM on the Mac) to build/verify the linux/amd64 artifact, then SSH deploy + systemd. |

---

## 1. Existing server — verified live state (read-only inspection)

| Property | Value |
|---|---|
| Host | `217.76.158.90` (IONOS VPS), Ubuntu 26.04, **x86_64** |
| CPU / RAM / Disk | **1 vCPU / 821 MB RAM / 8.6 GB disk** |
| RAM in use | ~497 MB used, **~324 MB available** |
| Disk free | 5.1 GB |
| Running app | `crm.service` (Fast Insurance Portal), listens **`:8443`** |
| Ingress | `iptables` NAT `REDIRECT :443 → :8443` (all hosts). **Nothing binds `:80`/`:443` directly.** |
| UFW | allow `22, 443, 8443` |

**Implication:** the box is small. Co-hosting is fine for light load, but oilfield **must**
be memory/CPU-capped so it can never starve crm (see §7). The only structural change to
the host is replacing the blunt `:443→:8443` redirect with hostname-aware routing (§6).

---

## 2. Goals & Non-Goals

**Goals**
- One binary: REST API + static React frontend + in-process scraper, backed by one SQLite file.
- `make dev` brings the whole thing up locally on macOS with zero external services.
- `make deploy` (Colima-built linux artifact) ships to the existing IONOS VPS.
- **Zero impact on `fast-insurance-portal`** — separate port, user, path, DB, systemd unit, resource caps.
- Cheaper than the ~$15.40/mo multi-tier design (co-host adds **$0** compute).

**Non-Goals**
- No multi-region HA, no quorum, no failover. Single box, single point of failure — by design.
- No etcd, no distributed lock, no per-provider provisioning.
- No R2/Litestream backups. No Cloudflare Pages.

---

## 3. Isolation matrix (oilfield vs. crm on the same box)

| Resource | crm (fast portal) | oilfield-solo | Conflict? |
|---|---|---|---|
| Internal port | 8443 | **8444** | No |
| systemd unit | `crm.service` (www-data) | `oilfield.service` (**`oilfield`** user) | No |
| Install path | `/opt/crm` | **`/opt/oilfield`** | No |
| SQLite file | `/opt/crm/data/crm.db` | **`/opt/oilfield/data/oilfield.db`** | No |
| TLS cert | `/opt/crm/data/certs` | **`/opt/oilfield/data/certs`** (own self-signed) | No |
| Domain (both CF-proxied) | fast.arleyxu.com | oilfield-dash.parso.guru | No |
| External `:443` | iptables → 8443 (all hosts) | **routed by SNI (§6)** | **Fixed by §6** |
| RAM | ~497 MB used | **capped ≤ 220 MB (§7)** | Guarded |
| CPU (1 core) | default weight | **`CPUWeight=50` (§7)** | Guarded |
| Disk | 3.5 GB used | +~60 MB | No (5.1 GB free) |

---

## 4. Target architecture (co-hosted)

```
                 Cloudflare (proxied)
   fast.arleyxu.com ─┐            ┌─ oilfield-dash.parso.guru
                     ▼            ▼
        ┌───────────────────────────────────────────┐
        │  IONOS VPS 217.76.158.90 (Ubuntu, x86_64)  │
        │                                            │
        │   nginx  :443  (stream, ssl_preread)       │  ← replaces iptables 443→8443
        │     ├─ SNI fast.arleyxu.com   → 127.0.0.1:8443 (crm, UNCHANGED)
        │     └─ SNI oilfield-dash...   → 127.0.0.1:8444 (oilfield)
        │                                            │
        │   crm.service      :8443  (untouched)      │
        │   oilfield.service :8444                   │  systemd, user=oilfield
        │     • REST API /api/v1/*                   │  MemoryMax=220M CPUWeight=50
        │     • static React (Go embed)             │
        │     • scraper goroutine (5-min ticker)    │
        │            │                               │
        │            ▼                               │
        │   SQLite /opt/oilfield/data/oilfield.db   │
        └───────────────────────────────────────────┘
```

TLS is **passed through** by nginx (no re-termination); each app keeps its own self-signed
cert and Cloudflare stays on **Full**. crm's process and config are never modified.

---

## 5. Data store: etcd → SQLite  *(unchanged from prior draft)*

### 5.1 Drop-in interface
New `internal/store` mirrors the `etcdstore` method names (`Get/Put/PutJSON/GetJSON/Delete/GetWithPrefix/IsHealthy`) so `internal/api` and the scraper compile with minimal edits.
`AcquireLock`/`RevokeLease` become **no-ops** (single writer). `GetWithPrefix` → `WHERE key LIKE ?||'%'`. `IsHealthy` → `SELECT 1`.

### 5.2 Schema (`internal/store/schema.sql`)
Keep a generic KV table so the JSON payloads and handlers are unchanged:
```sql
CREATE TABLE IF NOT EXISTS kv (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_kv_prefix ON kv(key);
```
WAL mode, `busy_timeout=5000`. `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`) → static binary. `Put` = `INSERT ... ON CONFLICT(key) DO UPDATE`.

### 5.3 go.mod
Remove `go.etcd.io/etcd/client/v3` (+ grpc/zap/protobuf tree). Add `modernc.org/sqlite`.

---

## 6. Port-443 routing (the ONE host change) — nginx SNI passthrough

### 6.1 Why the current rule can't stay
`iptables … REDIRECT dpt:443 redir ports 8443` sends **every** :443 packet to crm before any
socket sees it — it cannot distinguish hostnames. Two CF-proxied apps on one IP both arrive
on :443, so 443 must be terminated or SNI-inspected.

### 6.2 Recommended: nginx `stream` + `ssl_preread`
Install nginx (ships with `ngx_stream_ssl_preread_module`). It inspects the TLS ClientHello
SNI **without decrypting**, and forwards the raw TLS stream to the right backend.

`/etc/nginx/nginx.conf` (stream block):
```nginx
stream {
    map $ssl_preread_server_name $oilfield_backend {
        oilfield-dash.parso.guru   127.0.0.1:8444;
        default                    127.0.0.1:8443;   # crm — safe fallback
    }
    server {
        listen 443;
        listen [::]:443;
        ssl_preread on;
        proxy_pass $oilfield_backend;
    }
}
```
- crm keeps listening on 8443 with its own cert — **config untouched**.
- oilfield listens on 8444 with its own self-signed cert.
- Default backend = crm, so any SNI mishap still lands on crm (fail-safe).

**Cutover (fast, reversible):**
```
nginx -t                                  # validate
systemctl start nginx                      # binds :443 but gets nothing yet (NAT still active)
iptables -t nat -D PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443  # flip traffic to nginx
netfilter-persistent save
# ROLLBACK if needed: re-add the iptables rule, systemctl stop nginx
```
Because the NAT rule rewrites the port in PREROUTING, nginx receives no 443 traffic until the
rule is removed — so start nginx first, then flip. Rollback is a one-liner.

### 6.3 Alternative: Cloudflare Origin Rule (zero host-side change to crm)
If you'd rather not touch crm's front door at all:
- Leave crm's `443→8443` iptables rule as-is.
- oilfield: `iptables … REDIRECT dpt:2053 → :8444`; `ufw allow 2053`.
- Cloudflare → **Rules → Origin Rules**: for host `oilfield-dash.parso.guru`, rewrite
  **Destination Port → 2053** (2053 is a CF-supported HTTPS origin port; Origin Rules are on Free).
- crm networking is **never touched**. Trade-off: the port indirection lives in Cloudflare
  (less visible on the box), and direct-origin curl needs the alt port.

> **Open decision:** §6.2 (recommended, self-contained, standard) vs §6.3 (zero-touch to crm).
> See the elaboration in the chat for the full trade-off.

---

## 7. Resource guardrails (protect crm on the 1 vCPU / 821 MB box)

`deploy/systemd/oilfield.service` includes cgroup limits so oilfield can **never** starve crm:
```ini
[Service]
User=oilfield
Group=oilfield
EnvironmentFile=/opt/oilfield/env
ExecStart=/opt/oilfield/bin/oilfield
Restart=always
RestartSec=5
# --- resource fencing (crm gets priority) ---
MemoryHigh=150M
MemoryMax=220M
CPUWeight=50
Nice=5
# --- hardening (same posture as crm.service) ---
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/oilfield/data
```
Additional guardrails:
- **Scraper concurrency bounded** to ~3 sources at a time (not all 9) to cap the every-5-min RAM/CPU spike.
- **Add a 2 GB swapfile** as cheap insurance against transient spikes (harmless to crm):
  `fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile` + `/etc/fstab`.
- oilfield idle RSS ≈ 40 MB; nginx ≈ 10 MB. With the 220 MB cap + 324 MB free + swap, crm stays safe.

---

## 8. Backend changes

### 8.1 New `cmd/oilfield/main.go` (the one binary)
- Env: `ADDR=:8444`, `DB_PATH`, `TLS_CERT`, `TLS_KEY`, `SCRAPE_INTERVAL`, `SCRAPE_CONCURRENCY=3`, `EIA_API_KEY`, `DEEPSEEK_API_KEY` (if UI chat used).
- Opens SQLite, runs `schema.sql`.
- Mounts `/api/v1/*` (existing handlers) + `/` (embedded React, SPA fallback to `index.html`).
- Scraper **goroutine**: `time.NewTicker(SCRAPE_INTERVAL)`, bounded concurrency; first run at boot. `--scrape-once` flag for local testing.
- HTTPS on `:8444`; graceful SIGTERM shutdown.

### 8.2 `internal/api/handlers.go` (minor edits)
- `/api/v1/health` → `{node:"solo", status, db_healthy, last_scrape, time}`.
- `/api/v1/nodes` → single-element array (keeps NodeHealthGrid/AdminConsole map working).
- `/api/v1/prices/{sector}`, `/api/v1/news` read from SQLite. Sectors unchanged.
- `/api/v1/etcd/health` → rename `/api/v1/db/health`. Drop/soften `/api/v1/status`.

### 8.3 Delete `dashboard/web/backend` (the N4 aggregator) — frontend calls its own origin.

---

## 9. Frontend changes
- Source unchanged in `dashboard/web/frontend`. API base → same-origin `/api`.
- `NodeHealthGrid`/`AdminConsole` render one node (cosmetic).
- `npm run build` → embedded into the Go binary via `//go:embed` (single self-contained artifact).
- **Remove Cloudflare Pages** `oilfield-dash` project + its custom-domain CNAME (§11). No `wrangler` in deploy.

---

## 10. Local dev workflow (100% local)
```bash
make dev          # go run ./cmd/oilfield  → https://localhost:8444
make dev-ui       # cd dashboard/web/frontend && npm run dev → http://localhost:5173
make build        # npm run build + go build ./cmd/oilfield → bin/oilfield
make scrape-once  # populate local DB once
make test         # go test ./...
```
`.data/oilfield.db`, `.data/certs/*` gitignored. Requires only Go 1.23+, Node 20+. No etcd, no Docker for local dev.

---

## 11. Colima build & deploy (to existing IONOS)

### 11.1 One-time
```bash
brew install colima docker
colima start --arch x86_64 --cpu 2 --memory 4
```
### 11.2 Build linux/amd64 artifact (verified in x86_64 parity)
```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23 \
  bash -lc 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/oilfield-linux ./app/backend/cmd/oilfield'
( cd dashboard/web/frontend && npm ci && npm run build )   # embedded before the go build in practice
```
### 11.3 `make deploy` (co-host aware)
```
1. build bin/oilfield-linux (Colima) with embedded frontend
2. ssh root@217.76.158.90 "systemctl stop oilfield" (ignore on first run)
3. rsync bin/oilfield-linux → /opt/oilfield/bin/oilfield
4. ssh ... "systemctl start oilfield && systemctl is-active oilfield"
5. deploy/scripts/verify-deploy.sh  → curl both apps' health, assert crm still 200
```
> Deploy **never** touches `/opt/crm`, `crm.service`, or crm's port. A post-deploy check
> asserts `https://fast.arleyxu.com` (via origin :8443) still returns 200.

---

## 12. One-time host bootstrap (`deploy/scripts/cohost-bootstrap.sh`)
Run once against `217.76.158.90`:
1. `apt-get install -y nginx sqlite3` (nginx ships ssl_preread).
2. Create `oilfield` system user; `mkdir -p /opt/oilfield/{bin,data/certs}`.
3. Self-signed origin cert → `/opt/oilfield/data/certs/`.
4. Write `/opt/oilfield/env` (see `.env.example`), `chmod 600`.
5. Install `deploy/systemd/oilfield.service` (with resource caps, §7); `systemctl enable oilfield`.
6. Add 2 GB swapfile (§7).
7. `ufw allow 8444` (internal only; not strictly needed if loopback), keep 22/443.
8. **Routing cutover (§6.2):** install nginx.conf stream block, `nginx -t`, start nginx, remove the `443→8443` iptables rule, `netfilter-persistent save`. Verify crm still serves.

Each numbered step is idempotent and prints a crm-health check before/after the routing cutover.

---

## 13. DNS cutover (Cloudflare, zone `parso.guru`)
Token in `~/.cloudflare-api-token`; zone id in `infra/.env`.
1. **Delete the CF Pages project** `oilfield-dash` + its managed CNAME (frees the name).
2. Create/overwrite **`oilfield-dash.parso.guru`** → **proxied A record → `217.76.158.90`** (orange cloud).
3. Cloudflare **SSL/TLS → Full** (already set for arleyxu; parso.guru zone: confirm Full).
4. (If using §6.3 instead of §6.2) add the Origin Rule for the alt port.
5. Optionally clean up stale multi-tier records (`n1/n2/n3`, `api`, `etcd`, `ctrl`).

---

## 14. Decommission the multi-tier cluster (after solo verified)
Only after `oilfield-dash.parso.guru` serves correctly from the IONOS box **and** crm is confirmed healthy:
- `infra/teardown/teardown-all.sh` → delete Hetzner/Linode/UpCloud VMs.
- Manually delete the **old multi-tier IONOS N3** VM at my.ionos.com (this is a *different* VM from `217.76.158.90`).
- Remove stale DNS records. Recoverable via `multi-tier-v1.0.0`.

---

## 15. Cost comparison
| Item | Multi-tier | Single-tier (co-host) |
|---|---|---|
| Compute | Hetzner $4.20 + Linode $6.00 + Ionos $2.00 + UpCloud $3.20 | **$0 extra** (shares existing IONOS box) |
| Frontend | CF Pages $0 | $0 (served by Go) |
| DNS / TLS | CF / Let's Encrypt $0 | CF / self-signed $0 |
| Backups | — | — |
| **Total (incremental)** | **~$15.40/mo** | **~$0/mo** |

Co-hosting is the cheapest possible outcome; the trade-off is shared fate with crm on one small box (mitigated by §7).

---

## 16. Phased implementation checklist
| Phase | Work | Verify | Status |
|---|---|---|---|
| P0 | Tag `multi-tier-v1.0.0` + this plan | `git tag` | ✅ done |
| P1 | `internal/store` (SQLite) + tests | `go test ./internal/store/...` | ✅ done |
| P2 | `cmd/oilfield`: api + embedded static + bounded in-process scraper; single-node handler edits | curl `:8444/api/v1/health` | ✅ done (smoke-tested; real scrape wrote 302 prices / 141 news) |
| P3 | Frontend API base → same-origin; single-node grid; `npm run build` + embed | UI builds; binary serves it | ✅ done |
| P4 | Makefile, `.env.example`, `deploy/systemd/oilfield.service` (caps), `deploy/nginx/`, scripts | `make build-linux` | ✅ done (static ELF x86-64 verified) |
| P5 | Colima parity build target (`make parity`) | container serves `/api/v1/health` | ✅ target ready (run when colima up) |
| P6 | **Host bootstrap** on `217.76.158.90` (nginx SNI cutover + oilfield user/paths/swap) | crm still 200; `systemctl status oilfield` | ✅ done (installed `libnginx-mod-stream`; cutover clean; crm verified before+after) |
| P7 | `make deploy`; DNS cutover (`deploy/scripts/dns.sh`); delete CF Pages | `https://oilfield-dash.parso.guru` live **and** crm live | ✅ done (LIVE; CF Pages project deleted) |
| P8 | Decommission multi-tier cluster | old VMs gone | ✅ done — stale DNS removed; N1/N2/N4 confirmed down by owner; old "Ionos N3" was the kept box (217.76.158.90) |
| P9 | Removed `etcdstore`, `dashboard/web/backend`, old cmds; go.mod cleaned | `go build ./...` clean | ✅ done |

### Deployment status — LIVE ✅ (2026-07-02)
`https://oilfield-dash.parso.guru` is live from the co-hosted IONOS box, serving the
React frontend + API from one Go binary + SQLite. Scraper populated the DB
(302 prices / 141 news). `fast-insurance-portal` (crm) verified healthy before, during,
and after the :443 SNI cutover — HTTP/2 200 externally. Memory: ~355 MB free with both
apps running (oilfield capped at 220 MB). CF Pages `oilfield-dash` project deleted.

**P8 complete (2026-07-02):** stale old-cluster DNS records removed (only
`oilfield-dash.parso.guru` remains). The old "Ionos N3" node was the very box we kept
(`217.76.158.90`), so nothing to delete there. N1 (Hetzner) / N2 (Linode) / N4 (UpCloud)
confirmed down by the owner. Migration finished — single-server is the only live stack.

---

## 17. Open issues / confirm
1. **Routing choice:** §6.2 nginx-SNI (recommended) vs §6.3 CF Origin Rule (zero-touch to crm). *(Elaborated in chat.)*
2. **Memory reality:** box has ~324 MB free. Caps + 2 GB swap make it safe for light load; confirm you're OK sharing.
3. **DeepSeek chat:** reuse `~/.deepseek-api-key` if the dash uses LLM; confirm whether it does.
4. **parso.guru SSL mode** must be **Full** (not Flexible) for the self-signed origin. Confirm zone setting.

*End PLAN-SINGLE-TIER.md*
