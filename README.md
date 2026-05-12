# Oilfield — Sovereign Multi-Cloud Energy Market Dashboard

A real-time global energy futures market visualization platform, built as a demonstration of the [Daylight](https://github.com/anomalyco/daylight) architecture: no cloud lock-in, no managed databases, no proprietary orchestration. Four nodes across four independent cloud providers, coordinated by etcd, served behind nginx with Let's Encrypt TLS, for **$17.40/month**.

**Live dashboard:** [https://oilfield-dash.parso.guru](https://oilfield-dash.parso.guru) (Cloudflare Pages)  
**Public API:** [https://oilfield.parso.guru/api/v1/health](https://oilfield.parso.guru/api/v1/health) (round-robin: N1/N2/N3)  
**Whitepaper:** [WHITEPAPER.md](WHITEPAPER.md) — *The Hidden Cost of Conventional Cloud*  
**Technical plan:** [PLAN.md](PLAN.md)

---

## Table of Contents

1. [What It Does](#what-it-does)
2. [Cluster Architecture](#cluster-architecture)
3. [How Daylight Fits In](#how-daylight-fits-in)
4. [Directory Layout](#directory-layout)
5. [Administrator Setup — Start Here](#administrator-setup--start-here)
   - [Prerequisites](#1-prerequisites)
   - [Clone the repo](#2-clone-the-repo)
   - [Receive and place the .env file](#3-receive-and-place-the-env-file)
   - [Generate or install the SSH key](#4-generate-or-install-the-ssh-key)
   - [Bring up the cluster](#5-bring-up-the-cluster)
   - [Verify the cluster is healthy](#6-verify-the-cluster-is-healthy)
   - [Tear down](#7-tear-down)
6. [Runtime Status — CLI, curl, and Web](#runtime-status--cli-curl-and-web)
7. [Redeploy Without Reprovisioning](#redeploy-without-reprovisioning)
8. [Top-Level Code Design](#top-level-code-design)
9. [Public Endpoints](#public-endpoints)
10. [References](#references)
11. [Cost Summary](#cost-summary)

---

## What It Does

The platform scrapes global energy futures pricing from eight independent data sources every five minutes and stores results in an etcd key-value cluster spanning three continents. A React dashboard visualizes the data as a 3D forward-curve surface (Three.js), a 2D multi-series line chart (Recharts), a tabular view, and a live news feed from ten global energy publications.

Data covers nine sectors: crude oil, natural gas, LNG, LPG, NGLs, electricity, refined products, coal, and carbon credits. Sources include EIA API v2, Yahoo Finance, FRED/IMF, ENTSO-E, Eurostat SDMX, AEMO (Australia), Investing.com, and OilPriceAPI.

---

## Cluster Architecture

```
                ┌──────────────────────────────────────────────────┐
                │  Cloudflare DNS — parso.guru zone                 │
                │  oilfield.parso.guru  →  N1, N2, N3 (round-robin)│
                │  n1/n2/n3.oilfield.parso.guru  →  per-node        │
                │  ctrl.oilfield.parso.guru  →  N4                  │
                │  oilfield-dash.parso.guru  →  CF Pages (CNAME)    │
                └──────────────────────────────────────────────────┘

  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
  │ N1 — Hetzner Cloud   │  │ N2 — Linode/Akamai  │  │ N3 — Scaleway       │
  │ Ashburn, VA (US East)│  │ Los Angeles (US West)│  │ Paris, FR (Europe)  │
  │ cpx11: 2vCPU 4GB RAM │  │ g6-std-1: 1vCPU 2GB │  │ PLAY2-MICRO: 1vCPU  │
  │ ~$4.20/mo            │  │ ~$6.00/mo            │  │ ~$4.00/mo           │
  │                      │  │                      │  │                     │
  │  etcd (peer member)  │  │  etcd (peer member)  │  │  etcd (peer member) │
  │  oilfield-api :8080  │  │  oilfield-api :8080  │  │  oilfield-api :8080 │
  │  oilfield-scraper    │  │  oilfield-scraper    │  │  oilfield-scraper   │
  │  nginx + TLS (443)   │  │  nginx + TLS (443)   │  │  nginx + TLS (443)  │
  └──────────┬───────────┘  └──────────┬───────────┘  └──────────┬──────────┘
             │  etcd cluster (2379 client / 2380 peer, WAN-tuned)  │
             └──────────────────────────┬──────────────────────────┘

                          ┌─────────────────────┐
                          │ N4 — UpCloud          │
                          │ Chicago, IL (US Ctrl) │
                          │ DEV-1xCPU-2GB         │
                          │ ~$3.20/mo             │
                          │                       │
                          │  oilfield-dash-web    │
                          │  :8090 (Go backend)   │
                          │  nginx + TLS (443)    │
                          └─────────────────────┘

  ┌─────────────────────────────────────┐
  │  Cloudflare Pages (free tier)        │
  │  React frontend → oilfield-dash-web  │
  │  CORS-gated to ctrl.oilfield...guru  │
  └─────────────────────────────────────┘
```

### Node roles

| Node | Provider | Region | Role | Cost |
|------|----------|--------|------|------|
| N1 | Hetzner Cloud | Ashburn, VA | Runtime: etcd + API + scraper | ~$4.20/mo |
| N2 | Linode/Akamai | Los Angeles, CA | Runtime: etcd + API + scraper | ~$6.00/mo |
| N3 | Scaleway | Paris, FR | Runtime: etcd + API + scraper | ~$4.00/mo |
| N4 | UpCloud | Chicago, IL | Control: dashboard backend | ~$3.20/mo |
| CF | Cloudflare Pages | Global CDN | Frontend (React build) | Free |
| **Total** | | | | **~$17.40/mo** |

---

## How Daylight Fits In

[Daylight](https://github.com/anomalyco/daylight) is an architecture constraint system that enforces: *nothing may be used that is not replaceable without a full rewrite*. This project is a live demonstration of Daylight principles applied to production infrastructure:

- **No cloud lock-in**: Four different providers, one coherent system. Any node can be moved to a fifth provider by copying a binary and running two bootstrap scripts.
- **No managed databases**: etcd is the sole state store. It runs as a three-peer WAN cluster across N1/N2/N3. No RDS, no DynamoDB, no connection pools, no schema migrations.
- **No proprietary orchestration**: No Kubernetes, no ECS, no Lambda. Every service is a systemd unit on a plain Ubuntu VPS.
- **DNS as the load balancer**: `oilfield.parso.guru` has three A records (N1, N2, N3). DNS round-robin is the only load balancing mechanism. No ALB, no NGINX upstream list required.
- **Unix process as the unit of work**: `oilfield-api`, `oilfield-scraper`, and `oilfield-dash-web` are systemd services — observable (`journalctl -u`), restartable (`systemctl restart`), replaceable (copy binary + restart).
- **etcd as the nervous system**: All cluster state — price snapshots, news items, scrape locks, node heartbeats — lives in etcd under a consistent `/oilfield/` prefix scheme.

For more on the Daylight philosophy, see the [Daylight GitHub](https://github.com/anomalyco/daylight) and the project [whitepaper](WHITEPAPER.md).

---

## Directory Layout

```
parso-dylt-dashboard/
├── app/
│   └── backend/              # Go backend — API server + scraper
│       ├── cmd/
│       │   ├── api/          # oilfield-api: REST API server (port 8080)
│       │   └── scraper/      # oilfield-scraper: oneshot scrape process
│       ├── internal/
│       │   ├── api/          # HTTP handlers, routing, aggregation
│       │   ├── etcdstore/    # etcd client wrapper (read/write/lease)
│       │   └── scraper/      # per-source scrapers (EIA, Yahoo, FRED, etc.)
│       ├── go.mod
│       └── systemd/          # Service unit templates (installed by deploy)
│           ├── oilfield-api.service
│           ├── oilfield-scraper.service
│           └── oilfield-scraper.timer
│
├── dashboard/
│   ├── cli/                  # oilfield-dash CLI tool
│   │   ├── main.go
│   │   ├── cmd/              # Cobra subcommands: status, nodes, prices, news, watch
│   │   └── internal/
│   │       ├── client/       # HTTP client for the cluster API
│   │       └── render/       # Terminal rendering (tables, colour)
│   └── web/
│       ├── backend/          # oilfield-dash-web: Go proxy/aggregator (port 8090)
│       │   ├── main.go       # HTTP server, node aggregation, CORS middleware
│       │   ├── systemd/
│       │   │   └── oilfield-dash.service
│       │   └── go.mod
│       └── frontend/         # React + TypeScript + Three.js dashboard
│           ├── src/
│           │   ├── App.tsx          # Root component, data polling, layout
│           │   ├── components/
│           │   │   ├── DailyPricesBoard.tsx
│           │   │   ├── EnergyCurve3D.tsx
│           │   │   ├── PriceChart2D.tsx
│           │   │   ├── PriceTable.tsx
│           │   │   ├── NewsPanel.tsx
│           │   │   ├── NodeHealthGrid.tsx
│           │   │   ├── AdminPanel.tsx
│           │   │   └── AdminConsole.tsx
│           │   └── types.ts
│           ├── package.json
│           └── vite.config.ts
│
├── infra/
│   ├── .env                  # ← secrets file (GPG-encrypted at rest; see setup below)
│   ├── up.sh                 # Master bring-up: provision → bootstrap → DNS → TLS → deploy
│   ├── state/
│   │   ├── hetzner.ip        # Written by provision/hetzner.sh
│   │   ├── linode.ip         # Written by provision/linode.sh
│   │   ├── scaleway.ip       # Written by provision/scaleway.sh
│   │   ├── upcloud.ip        # Written by provision/upcloud.sh
│   │   └── cluster.env       # Exports N1_IP/N2_IP/N3_IP/N4_IP
│   ├── provision/            # Create VMs via each provider's REST API
│   │   ├── hetzner.sh
│   │   ├── linode.sh
│   │   ├── scaleway.sh
│   │   └── upcloud.sh
│   ├── bootstrap/            # Remote scripts piped via SSH
│   │   ├── base.sh           # apt packages, SSH hardening, UFW, deploy user
│   │   ├── daylight.sh       # etcd install + cluster config + systemd stubs
│   │   ├── dns.sh            # Cloudflare DNS records (delete-all then create)
│   │   └── tls.sh            # Let's Encrypt via certbot webroot + nginx HTTPS
│   ├── deploy/
│   │   ├── deploy-app.sh     # Cross-compile Go → scp → systemctl restart N1/N2/N3
│   │   └── deploy-dash.sh    # Build backend + React → N4 + CF Pages
│   └── teardown/
│       └── teardown-all.sh   # Delete all four VMs + clean state + purge known_hosts
│
├── WHITEPAPER.md
├── PLAN.md
└── README.md
```

---

## Administrator Setup — Start Here

This section is the complete procedure for a new admin to go from zero to a running cluster. The sender will provide the `infra/.env` file encrypted with GPG.

### 1. Prerequisites

Install the following on your local machine before doing anything else.

**macOS (Homebrew):**
```bash
brew install git go node gpg jq curl bind   # bind provides dig
# node installs npm automatically
```

**Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install -y git golang-go nodejs npm gpg jq curl dnsutils
```

**Verify versions (minimums):**
```bash
go version          # need 1.21+
node --version      # need 18+
npm --version       # need 9+
gpg --version       # any modern version
jq --version
dig -v
```

**Install wrangler** (Cloudflare Pages CLI — used by deploy-dash.sh):
```bash
npm install -g wrangler
```

### 2. Clone the repo

```bash
git clone https://github.com/anomalyco/parso-dylt-dashboard.git
cd parso-dylt-dashboard
```

### 3. Receive and place the .env file

The sender will provide `infra/.env` encrypted as `infra.env.gpg`. Decrypt it and place it at exactly `infra/.env`:

```bash
# Decrypt (you will be prompted for the passphrase the sender gave you)
gpg --decrypt infra.env.gpg > infra/.env

# Confirm the file looks right — should show ~20 key=value lines
head -5 infra/.env

# Lock down permissions so other users on the machine cannot read it
chmod 600 infra/.env
```

The decrypted file contains all cloud API tokens, the Cloudflare credentials, the EIA API key, the etcd cluster token, and the ADMIN_EMAIL. It should look like this (values redacted here):

```
CF_ACCOUNT_ID=...
CF_PAGES_PROJECT=oilfield-dash
CLOUDFLARE_API_TOKEN=...
CLOUDFLARE_ZONE_ID=...
DOMAIN=oilfield.parso.guru
EIA_API_KEY=...
ETCD_CLUSTER_TOKEN=...
HETZNER_API_TOKEN=...
LINODE_TOKEN=...
SCW_ACCESS_KEY=...
SCW_PROJECT_ID=...
SCW_SECRET_KEY=...
SCW_ZONE=fr-par-1
UPCLOUD_API_TOKEN=...
UPCLOUD_ZONE=us-chi1
SSH_PRIVATE_KEY_PATH=~/.ssh/oilfield_ed25519
SSH_PUBLIC_KEY_PATH=~/.ssh/oilfield_ed25519.pub
ADMIN_EMAIL=you@example.com
```

> The file is listed in `.gitignore` and will never be committed. Do not move it or rename it — all infra scripts source it from `infra/.env`.

### 4. Generate or install the SSH key

All four VMs are accessed with a dedicated ED25519 key. The `.env` file points to `~/.ssh/oilfield_ed25519`.

**If the sender also provided the private key** (sent separately, also GPG-encrypted):
```bash
gpg --decrypt oilfield_ed25519.gpg > ~/.ssh/oilfield_ed25519
chmod 600 ~/.ssh/oilfield_ed25519

# Derive the public key from it
ssh-keygen -y -f ~/.ssh/oilfield_ed25519 > ~/.ssh/oilfield_ed25519.pub
chmod 644 ~/.ssh/oilfield_ed25519.pub
```

**If you are generating a fresh key pair** (new admin, first time):
```bash
ssh-keygen -t ed25519 -f ~/.ssh/oilfield_ed25519 -N "" -C "oilfield-deploy"
# No passphrase (-N "") so up.sh can use it non-interactively.
```

Then send `~/.ssh/oilfield_ed25519.pub` to the sender so they can upload it to all four cloud provider SSH key stores before you run `up.sh`. If this is a fresh teardown/rebuild cycle on existing provider accounts, the key is already registered — no action needed.

Confirm the key is present and has the right permissions:
```bash
ls -la ~/.ssh/oilfield_ed25519 ~/.ssh/oilfield_ed25519.pub
# Should show: -rw------- for the private key, -rw-r--r-- for the public key
```

### 5. Bring up the cluster

With the repo cloned, `.env` in place, and SSH key ready:

```bash
./infra/up.sh
```

The script runs unattended for approximately **8–12 minutes** and prints progress as it goes. It will not prompt for input. What it does:

| Phase | What happens | Time |
|-------|-------------|------|
| 1 — Provision | Creates four VMs in parallel across Hetzner, Linode, Scaleway, UpCloud | ~2–4 min |
| 2 — Bootstrap | Installs etcd on N1/N2/N3, forms cluster, drops systemd unit stubs | ~1–2 min |
| 3 — DNS | Deletes stale Cloudflare A records, creates fresh ones for all FQDNs; polls until propagated | ~1 min |
| 4 — TLS | Obtains Let's Encrypt certs for n1/n2/n3/ctrl in parallel; configures nginx HTTPS | ~1–2 min |
| 5 — App deploy | Cross-compiles Go binaries locally, deploys to N1/N2/N3; enables scraper timer; runs first scrape | ~1–2 min |
| 6 — Dashboard | Deploys Go backend to N4; builds React frontend; pushes to Cloudflare Pages | ~1–2 min |

If any phase fails, the script exits with an error message identifying exactly which step failed. See [Troubleshooting](#troubleshooting) below. You can resume a partial run with:

```bash
./infra/up.sh --force   # skips the stale-state guard
```

When `up.sh` finishes successfully you will see:

```
━━━ CLUSTER UP ━━━

  Endpoints:
    Price app:   https://oilfield.parso.guru
    API (N1):    https://n1.oilfield.parso.guru/api/v1/health
    API (N2):    https://n2.oilfield.parso.guru/api/v1/health
    API (N3):    https://n3.oilfield.parso.guru/api/v1/health
    Ctrl (N4):   https://ctrl.oilfield.parso.guru/api/v1/health
    Dashboard:   https://oilfield-dash.pages.dev
```

### 6. Verify the cluster is healthy

Run these immediately after `up.sh` completes. See [Runtime Status](#runtime-status--cli-curl-and-web) for the full reference.

```bash
# Build the CLI tool (one-time, takes ~5 seconds)
(cd dashboard/cli && go build -o ~/bin/oilfield-dash .)
# Or without installing to ~/bin:
(cd dashboard/cli && go build -o ./oilfield-dash .)
alias oilfield-dash="$(pwd)/dashboard/cli/oilfield-dash"

# Full cluster status — nodes + prices + news
oilfield-dash status

# Node health only
oilfield-dash nodes

# Quick curl smoke test (no CLI required)
curl -s https://oilfield.parso.guru/api/v1/health | jq .
```

All three runtime nodes should show `"status": "ok"` and `"etcd_healthy": true`. The scraper will have populated etcd within ~30 seconds of `up.sh` completing.

### 7. Tear down

```bash
./infra/teardown/teardown-all.sh
```

Prompts for confirmation, then deletes all four VMs, removes `infra/state/*.ip`, clears `cluster.env`, and purges stale SSH known_hosts entries for all cluster hostnames. DNS records are left in place by default (they cost nothing and `up.sh` will overwrite them on the next cycle).

To also delete the DNS records:
```bash
REMOVE_DNS=1 ./infra/teardown/teardown-all.sh
```

To skip the confirmation prompt (CI/scripted use):
```bash
./infra/teardown/teardown-all.sh --yes
```

---

## Runtime Status — CLI, curl, and Web

### CLI tool

Build once from the repo root:

```bash
cd dashboard/cli
go build -o oilfield-dash .
cd ../..
```

The binary has no external dependencies and runs anywhere Go 1.21+ can compile. By default it talks to `https://api.oilfield.parso.guru`. Override with `--api` or the `OILFIELD_API_URL` env var.

**All subcommands:**

```
oilfield-dash status              # full cluster view: nodes + all prices + latest 5 news items
oilfield-dash nodes               # node health table only
oilfield-dash prices              # all sectors, spot prices
oilfield-dash prices crude        # crude oil only
oilfield-dash prices natgas       # natural gas only
oilfield-dash prices lng          # LNG
oilfield-dash prices lpg          # LPG
oilfield-dash prices ngls         # NGLs
oilfield-dash prices electricity  # electricity
oilfield-dash prices refined      # refined products
oilfield-dash news                # latest 20 news items
oilfield-dash news -n 5           # latest 5 news items
oilfield-dash watch               # auto-refresh every 10s (Ctrl+C to exit)
oilfield-dash watch -i 30         # auto-refresh every 30s
```

Point at a specific node instead of the round-robin API:
```bash
oilfield-dash --api https://n1.oilfield.parso.guru status
oilfield-dash --api https://n2.oilfield.parso.guru prices crude
```

### curl

All endpoints are HTTPS, no authentication required. Pipe through `jq` for readable output.

**Cluster health (via dashboard backend — aggregates all nodes):**
```bash
curl -s https://ctrl.oilfield.parso.guru/api/v1/nodes | jq .
# Returns array: [{name, status, etcd_healthy, lat, lon, city, provider}, ...]

curl -s https://ctrl.oilfield.parso.guru/api/v1/status | jq .
# Returns aggregated cluster status object
```

**Per-node health (direct, bypasses aggregator):**
```bash
curl -s https://n1.oilfield.parso.guru/api/v1/health | jq .
curl -s https://n2.oilfield.parso.guru/api/v1/health | jq .
curl -s https://n3.oilfield.parso.guru/api/v1/health | jq .
# Returns: {node, status, etcd_healthy, last_scrape, time}

curl -s https://ctrl.oilfield.parso.guru/api/v1/health | jq .
# Returns: {service, status} for the dashboard backend
```

**Round-robin API (hits N1, N2, or N3 depending on DNS):**
```bash
curl -s https://oilfield.parso.guru/api/v1/health | jq .
```

**Prices by sector:**
```bash
curl -s https://n1.oilfield.parso.guru/api/v1/prices/crude       | jq '[.[:3][] | {symbol,name,price,unit}]'
curl -s https://n2.oilfield.parso.guru/api/v1/prices/natgas      | jq '[.[:3][] | {symbol,name,price,unit}]'
curl -s https://n3.oilfield.parso.guru/api/v1/prices/electricity | jq '[.[:3][] | {symbol,name,price,unit}]'

# All sectors via the ctrl aggregator (races N1/N2/N3, returns fastest):
curl -s https://ctrl.oilfield.parso.guru/api/v1/prices/crude | jq '[.[:5][] | {symbol,price,unit,source}]'
```

Valid sector names: `crude` `natgas` `lng` `lpg` `ngls` `electricity` `refined` `coal` `carbon`

**News:**
```bash
curl -s https://n1.oilfield.parso.guru/api/v1/news | jq '[.[:5][] | {title,source,published}]'
```

**etcd cluster health (from a runtime node):**
```bash
curl -s https://n1.oilfield.parso.guru/api/v1/etcd/health | jq .
# Returns etcd member health for the three-node cluster
```

**One-liner full smoke test** — checks all four nodes and prints a pass/fail summary:
```bash
for host in n1 n2 n3 ctrl; do
  url="https://${host}.oilfield.parso.guru/api/v1/health"
  status=$(curl -sf "$url" | jq -r '.status // .service' 2>/dev/null)
  printf "%-6s %s\n" "$host" "${status:-UNREACHABLE}"
done
```

Expected output when healthy:
```
n1     ok
n2     ok
n3     ok
ctrl   ok
```

### Web dashboard

Open in any browser:

```
https://oilfield-dash.parso.guru
```

The dashboard connects to `https://ctrl.oilfield.parso.guru` and polls:
- Node health grid — every 15 seconds
- Prices (3D curve, 2D chart, table) — every 30 seconds
- News feed — every 5 minutes

If you just brought up the cluster, allow up to 60 seconds for the first scrape cycle to complete before prices appear. Node health will show immediately.

### Checking scraper logs on a node

To confirm the scraper is running and prices are being written to etcd:

```bash
# Read the IP from state after up.sh
source infra/state/cluster.env

# Check last scraper run on N1
ssh -i ~/.ssh/oilfield_ed25519 deploy@$N1_IP \
  "journalctl -u oilfield-scraper.service -n 20 --no-pager"

# Check scraper timer schedule
ssh -i ~/.ssh/oilfield_ed25519 deploy@$N1_IP \
  "systemctl list-timers oilfield-scraper.timer"

# Check oilfield-api is running
ssh -i ~/.ssh/oilfield_ed25519 deploy@$N1_IP \
  "systemctl status oilfield-api.service --no-pager"
```

Expected scraper log output:
```
[n1] scrape lock acquired — starting scrape cycle
[n1] scrape complete — 304 price points, 138 news items written to etcd
```

---

## Redeploy Without Reprovisioning

These commands update running code on existing VMs without tearing down and rebuilding:

```bash
# Redeploy Go API + scraper to all runtime nodes (N1/N2/N3)
./infra/deploy/deploy-app.sh all

# Redeploy to a single node only
./infra/deploy/deploy-app.sh n1
./infra/deploy/deploy-app.sh n2
./infra/deploy/deploy-app.sh n3

# Redeploy dashboard backend (N4) + React frontend (Cloudflare Pages)
./infra/deploy/deploy-dash.sh
```

---

## Top-Level Code Design

### Data flow

```
oilfield-scraper (oneshot, every 5 min via systemd timer)
  │
  ├── acquires distributed lease lock in etcd (/oilfield/lock/scraper)
  ├── fetches from 8 data sources concurrently
  ├── writes: /oilfield/prices/{sector}/{source}/{symbol}  → JSON price point
  │           /oilfield/news/{source}/items               → JSON array
  └── releases lock

oilfield-api (long-running, port 8080, N1/N2/N3)
  ├── GET /api/v1/health          → node name, status, last_scrape timestamp
  ├── GET /api/v1/prices/{sector} → reads etcd /oilfield/prices/{sector}/*
  ├── GET /api/v1/news            → reads /oilfield/news/*/items, deduplicates by URL
  └── GET /api/v1/etcd/health     → etcd member health report

oilfield-dash-web (long-running, port 8090, N4 only)
  ├── races GET to N1/N2/N3 in parallel; first 2xx response wins
  ├── GET /api/v1/nodes           → polls each node's /api/v1/health
  ├── GET /api/v1/prices/{sector} → proxied from fastest runtime node
  ├── GET /api/v1/news            → proxied from fastest runtime node
  ├── GET /api/v1/status          → aggregated cluster health view
  └── GET /api/v1/health          → {service, status} for the ctrl node itself

React frontend (Cloudflare Pages → ctrl.oilfield.parso.guru)
  ├── polls health every 15s
  ├── polls prices every 30s
  ├── polls news every 5min
  └── views: 3D forward curve | 2D chart | table | news | admin console
```

### etcd key schema

```
/oilfield/prices/{sector}/{source}/{symbol}   → JSON price point
/oilfield/news/{source}/items                 → JSON array of news items
/oilfield/lock/scraper                        → distributed lease lock (TTL 90s)
```

Sectors: `crude` `natgas` `lng` `lpg` `ngls` `electricity` `refined` `coal` `carbon`

### Front-end 3D chart

The 3D forward curve renders each energy sector as a flat ribbon surface. Uses a custom `BufferGeometry` quad-strip mesh (two vertices per data point at ±width/2 along the cross-axis) with `DoubleSide MeshBasicMaterial`. The canvas is mounted once; geometry is updated imperatively via buffer attribute refs on each price poll — not via React re-renders. Axis labels use `troika-three-text` (WebGL geometry — no DOM portals, no CDN font fetch at runtime).

### Distributed scrape coordination

All three runtime nodes run the scraper timer. The first node to acquire the etcd lease writes all price data; the other two exit immediately on lock failure. This avoids duplicate writes and API rate-limit exhaustion while ensuring scraping continues if any single node is down.

---

## Public Endpoints

All endpoints are unauthenticated and publicly accessible:

| Endpoint | Description |
|----------|-------------|
| `https://oilfield.parso.guru/api/v1/health` | Health check (DNS round-robin to N1/N2/N3) |
| `https://n1.oilfield.parso.guru/api/v1/health` | N1 health direct |
| `https://n2.oilfield.parso.guru/api/v1/health` | N2 health direct |
| `https://n3.oilfield.parso.guru/api/v1/health` | N3 health direct |
| `https://n1.oilfield.parso.guru/api/v1/prices/{sector}` | Prices from N1 |
| `https://oilfield.parso.guru/api/v1/news` | News (round-robin) |
| `https://n1.oilfield.parso.guru/api/v1/etcd/health` | etcd cluster health |
| `https://ctrl.oilfield.parso.guru/api/v1/health` | Dashboard backend health |
| `https://ctrl.oilfield.parso.guru/api/v1/nodes` | All node health (aggregated) |
| `https://ctrl.oilfield.parso.guru/api/v1/status` | Full cluster status |
| `https://ctrl.oilfield.parso.guru/api/v1/prices/{sector}` | Prices (races N1/N2/N3) |
| `https://oilfield-dash.parso.guru` | React dashboard |

---

## References

- **Daylight GitHub**: [https://github.com/anomalyco/daylight](https://github.com/anomalyco/daylight)
- **Parso Consulting**: [https://parso.guru](https://parso.guru)
- **Whitepaper** (this repo): [WHITEPAPER.md](WHITEPAPER.md) — *The Hidden Cost of Conventional Cloud*
- **Technical Plan** (this repo): [PLAN.md](PLAN.md)
- **EIA Open Data API**: [https://www.eia.gov/opendata/](https://www.eia.gov/opendata/) — free API key for energy price data
- **etcd**: [https://etcd.io](https://etcd.io) — distributed key-value store (v3.5.x)
- **Cloudflare Pages**: [https://pages.cloudflare.com](https://pages.cloudflare.com) — free static frontend hosting

---

## Cost Summary

| Provider | Node | Plan | $/month |
|----------|------|------|---------|
| Hetzner Cloud | N1 (runtime) | cpx11 (2vCPU/4GB) | ~$4.20 |
| Linode/Akamai | N2 (runtime) | g6-standard-1 (1vCPU/2GB) | ~$6.00 |
| Scaleway | N3 (runtime) | PLAY2-MICRO (1vCPU/2GB) | ~$4.00 |
| UpCloud | N4 (control) | DEV-1xCPU-2GB | ~$3.20 |
| Cloudflare Pages | Frontend CDN | Free tier | $0 |
| Let's Encrypt | TLS | Free | $0 |
| **Total** | | | **~$17.40/mo** |

Comparable AWS managed architecture (EKS, RDS, ElastiCache, ALB, CloudFront): **$188–260/month**.

---

## License

[LICENSE](LICENSE)
