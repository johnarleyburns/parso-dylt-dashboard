# PLAN.md — Sovereign Energy Market Infrastructure
## Parso Consulting / Daylight Architecture Project

> **Project Codename:** `oilfield`
> **Author:** Parso Consulting (John Burns & Chris Lalos)
> **Date:** April 2026
> **Whitepaper Target:** *Sovereign Infrastructure at Scale: Agentic Coding a Multi-Cloud Energy Market Data Platform with Daylight*

---

## Table of Contents

1. [Overview & Principles](#1-overview--principles)
2. [Provider Selection — The Four Nodes](#2-provider-selection--the-four-nodes)
3. [DNS Architecture](#3-dns-architecture)
4. [Repository & Agentic Coding Strategy](#4-repository--agentic-coding-strategy)
5. [Node Roles & Topology](#5-node-roles--topology)
6. [etcd Storage Architecture](#6-etcd-storage-architecture)
7. [Phase 0 — Manual Bootstrap (Human Required)](#7-phase-0--manual-bootstrap-human-required)
8. [Phase 1 — Agentic: Provision Scripts](#8-phase-1--agentic-provision-scripts)
9. [Phase 2 — Agentic: Daylight Runtime Setup](#9-phase-2--agentic-daylight-runtime-setup)
10. [Phase 3 — Agentic: The Energy Market Web App](#10-phase-3--agentic-the-energy-market-web-app)
11. [Phase 4 — Agentic: CLI Dashboard (Text/Read-Only)](#11-phase-4--agentic-cli-dashboard-textread-only)
12. [Phase 5 — Agentic: Web Dashboard (Read-Only)](#12-phase-5--agentic-web-dashboard-read-only)
13. [Phase 6 — Agentic: Web Dashboard (State-Changing)](#13-phase-6--agentic-web-dashboard-state-changing)
14. [Directory Structure](#14-directory-structure)
15. [Whitepaper Story Arc](#15-whitepaper-story-arc)
16. [Appendix A — Manual Steps Reference](#appendix-a--manual-steps-reference)
17. [Appendix B — Cost Summary](#appendix-b--cost-summary)
18. [Appendix C — etcd Size Budget](#appendix-c--etcd-size-budget)

---

## 1. Overview & Principles

This project builds a sovereign, multi-cloud, provider-agnostic infrastructure running a real-time global energy futures market visualization application covering crude oil, natural gas, LNG, LPG, NGLs, electricity, and refined products. It is built entirely on Daylight principles:

- **No cloud lock-in.** Four different providers, one coherent system.
- **No PostgreSQL.** etcd is the sole data store. Unix processes write state; etcd holds it.
- **Bedrock technology only.** Linux, DNS, systemd, etcd, gRPC. No Kubernetes, no managed databases, no Lambda.
- **Unix process as the unit of work.** Every service is a systemd unit. Every unit is observable, restartable, logable.
- **etcd as the nervous system.** Cluster state, config, health heartbeats, price snapshots, news index — all in etcd.
- **DNS as the load balancer.** Not an ELB. Not an NGINX upstream list. The DNS A-record set is the cluster.
- **Full node independence.** Any single node can go down. Any two nodes can go down. The remaining node(s) serve a complete, fully functional application from their own local etcd data.
- **Agentic coding with senior oversight.** Claude Code drives all code generation. Human judgment governs architecture, schema, and security boundaries.

### Why No PostgreSQL?

The application's data needs are:
- **Current state:** latest futures curve per product (read by the 3D chart)
- **News index:** last 90 days of EIA/IEA headlines + URLs (read by the news panel)
- **Cluster state:** node heartbeats, scrape locks, config

All three are current-state problems, not relational query problems. etcd handles all of them with substantial headroom (see Appendix C for sizing). Removing PostgreSQL eliminates: a replication protocol, a primary/replica failure mode, a connection pool, a migration runner, and approximately 150 lines of boilerplate the agent would have gotten wrong anyway.

The four Daylight Noble Truths applied throughout:
1. Uptime matters
2. Avoid novelty
3. Build for the team you have
4. The future team is the team you have now

---

## 2. Provider Selection — The Four Nodes

Chosen for: geographic diversity (no two nodes in the same region), no shared parent company, sub-$8/month entry VPS, API-driven provisioning, clean Ubuntu 24.04 LTS support.

| # | Role | Provider | Plan | Est. Cost/mo | Region | Provisioning API |
|---|------|----------|------|-------------|--------|-----------------|
| **N1** | Runtime Node 1 | **Hetzner Cloud** | CX22 (2 vCPU, 4 GB RAM, 40 GB NVMe) | ~$4.15 | Ashburn, VA (**US East**) | Hetzner Cloud API |
| **N2** | Runtime Node 2 | **Linode (Akamai)** | Cloud Server (1 vCPU, 2 GB RAM, 20 GB SSD) | ~$6.00 | Los Angeles, CA (**US West**) | Linode REST API |
| **N3** | Runtime Node 3 | **Ionos VPS** | VPS XS (1 vCPU, 1 GB RAM, 10 GB NVMe) | ~$2.00 | Berlin, DE (**Europe**) | Manual provisioning (my.ionos.com) |
| **N4** | Control/Dashboard | **UpCloud** | Cloud Server DEV-1SA (1 vCPU, 1 GB RAM, 10 GB MaxIOPS SSD) | ~$3.25 | Chicago, IL (**US Central**) | UpCloud API v3 |
| **CF** | Dashboard Frontend | **Cloudflare Pages** | Free tier (static React build) | $0 | Global CDN | CF Pages API |

**Total estimated cost: ~$17.40/month.**

### Geographic Distribution

| Node | Region | Failure zone |
|------|--------|-------------|
| N1/Hetzner | US East (Ashburn, VA) | US East outage → N2 + N3 hold quorum |
| N2/Linode | US West (Los Angeles, CA) | US West outage → N1 + N3 hold quorum |
| N3/Ionos | Europe (Berlin, DE) | Europe outage → N1 + N2 hold quorum |
| N4/UpCloud | US Central (Chicago, IL) | Control node down → runtime unaffected |

No two nodes share a failure zone. A US East + Europe simultaneous outage leaves N2 (US West) serving in degraded read-only mode. No provider parent company is shared across any two nodes.

### Node Independence Guarantee

Without PostgreSQL, each runtime node (N1, N2, N3) is **fully self-contained**:

```
N1 + N2 down → N3 alone serves: etcd (degraded read-only), Go API from local etcd data,
                React frontend, full price + news data from its last successful scrape.

N1 down      → N2 and N3 each independently serve the full application.
               etcd re-elects a leader automatically between N2 and N3.
               Scraper on whichever node wins the lock continues writing.
```

No node has a "primary" role. No node has a "replica" role. All peers.

---

## 3. DNS Architecture

DNS is the Daylight load balancer. No cloud load balancer, no NGINX upstream pool.

### 3.1 Domain Structure

```
oilfield.parso.guru          A     → N1, N2, N3     (round-robin, 60s TTL — user-facing app)
n1.oilfield.parso.guru       A     → N1 only         (300s TTL)
n2.oilfield.parso.guru       A     → N2 only         (300s TTL)
n3.oilfield.parso.guru       A     → N3 only         (300s TTL)
ctrl.oilfield.parso.guru     A     → N4 only         (300s TTL — SSH deploy hub + dashboard Go API :8090)
etcd.oilfield.parso.guru     A     → N1, N2, N3     (60s TTL — etcd peer discovery)
api.oilfield.parso.guru      A     → N1, N2, N3     (60s TTL — Go REST API)
dash.oilfield.parso.guru     CNAME → Cloudflare Pages (managed by CF — dashboard React frontend)
```

> **Note:** `dash.oilfield.parso.guru` is a Cloudflare Pages custom domain — CF manages the CNAME automatically when you add the domain in the Pages project settings. Do not create this record manually in `dns.sh`; it will conflict.

### 3.2 DNS Provider

**Cloudflare** (free tier) as authoritative DNS. Well-documented, curl-scriptable, programmatic record management. The `dns.sh` bootstrap script registers all records automatically after provisioning.

> ⚠️ **MANUAL STEP DNS-1:** Transfer nameservers for `parso.guru` to Cloudflare if not already done. Obtain Cloudflare API token with `Zone:DNS:Edit` scope.

### 3.3 TTL Strategy

| Record | TTL | Rationale |
|--------|-----|-----------|
| Node-specific (`n1.`, `n2.`, `n3.`, `ctrl.`) | 300s | Stable; infrequent changes |
| Cluster (`oilfield.`, `api.`, `etcd.`) | 60s | Fast failover if a node drops |
| `dash.` | Managed by CF Pages | CNAME — Cloudflare controls TTL; global CDN edge |

---

## 4. Repository & Agentic Coding Strategy

### 4.1 Repository Layout

```
oilfield/
├── infra/
│   ├── provision/           # Per-provider provisioning scripts
│   │   ├── hetzner.sh
│   │   ├── ovhcloud.sh
│   │   ├── ionos.sh
│   │   └── upcloud.sh
│   ├── bootstrap/           # Post-provision node bootstrap
│   │   ├── base.sh          # OS hardening, user setup, firewall
│   │   ├── daylight.sh      # etcd + dylt CLI install + systemd units
│   │   ├── dns.sh           # Cloudflare DNS registration
│   │   └── tls.sh           # Let's Encrypt via certbot
│   ├── deploy/              # Deployment (run from N4)
│   │   ├── deploy-app.sh
│   │   └── deploy-dash.sh
│   └── teardown/teardown-all.sh
├── app/                     # Energy market web app
│   ├── backend/
│   │   ├── cmd/api/main.go
│   │   ├── cmd/scraper/main.go
│   │   ├── internal/
│   │   │   ├── etcdstore/   # etcd I/O helpers
│   │   │   ├── scraper/     # Per-source scraper modules
│   │   │   └── api/         # HTTP handlers
│   │   └── systemd/
│   │       ├── oilfield-api.service
│   │       ├── oilfield-scraper.service
│   │       └── oilfield-scraper.timer
│   └── frontend/
│       ├── src/
│       │   ├── components/EnergyCurve3D.tsx
│       │   ├── components/NewsPanel.tsx
│       │   └── App.tsx
│       └── vite.config.ts
├── dashboard/
│   ├── cli/main.go          # Phase 4
│   └── web/                 # Phase 5+
├── dylt/                    # dylt submodule
├── docs/
│   ├── etcd-schema.md       # Human-authored — THE source of truth, never agent-generated
│   ├── agent-failures.md    # Running log for whitepaper
│   └── whitepaper/
└── PLAN.md
```

### 4.2 Agentic Coding Workflow

1. **Architect first, code second.** Before each Claude Code session, write a one-paragraph `CONTEXT.md` brief: what the module does, what interfaces it must respect, what it must NOT do.
2. **One module per session.** Each systemd unit gets its own session.
3. **etcd key schema is human-authored.** `docs/etcd-schema.md` is written and reviewed before any agent session that touches etcd.
4. **Agent output is git-committed immediately.** Every session ends with a commit tagged `[agent]`. The whitepaper audits exactly what was produced vs. hand-corrected.
5. **Failures are documented, not hidden.** Into `docs/agent-failures.md` for the whitepaper.

---

## 5. Node Roles & Topology

```
                        DNS: oilfield.parso.guru
                        A → N1, N2, N3 (round-robin, 60s TTL)
                               │
           ┌───────────────────┼───────────────────┐
           ▼                   ▼                   ▼
    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
    │  N1/Hetzner │    │ N2/Linode │    │  N3/Ionos  │
    │  US East    │    │  US West    │    │  Paris, EU  │
    │─────────────│    │─────────────│    │─────────────│
    │ etcd peer   │◄──►│ etcd peer   │◄──►│ etcd peer   │
    │ Go API :8080│    │ Go API :8080│    │ Go API :8080│
    │ Scraper     │    │ Scraper     │    │ Scraper     │
    │ nginx+React │    │ nginx+React │    │ nginx+React │
    │ (fully      │    │ (fully      │    │ (fully      │
    │  independent│    │  independent│    │  independent│
    │  no primary)│    │  no primary)│    │  no primary)│
    └─────────────┘    └─────────────┘    └─────────────┘
                               │ SSH deploy + dylt CLI
                               ▼
                    ┌─────────────────────┐
                    │  N4/UpCloud Chicago │
                    │   ctrl.oilfield.    │
                    │─────────────────────│
                    │ dylt CLI (control)  │
                    │ CLI dashboard (P4)  │
                    │ Dash backend (P5+)  │
                    │ etcd observer only  │
                    └─────────────────────┘
```

### Service Map (per runtime node N1/N2/N3)

| systemd Unit | Port | Role |
|---|---|---|
| `etcd.service` | 2379 (client), 2380 (peer) | Distributed K/V — sole data store |
| `oilfield-api.service` | 8080 | REST API — reads local etcd, serves JSON |
| `oilfield-scraper.service` + `.timer` | — | Scrapes all energy sectors + news into etcd |
| `nginx.service` | 80/443 | TLS termination + React static serving + API proxy |

---

## 6. etcd Storage Architecture

### 6.1 Design Principles

- **Each runtime node writes only to its own local etcd cluster member.** The etcd Raft protocol replicates all writes to the other members automatically.
- **One scraper runs at a time** — enforced by a lease-backed distributed lock. Any node's scraper can win the lock on any given cycle.
- **Writes are snapshot-replace, not append.** Each scrape cycle overwrites price keys with fresh data. No accumulation. etcd is a current-state store; we use it as one.
- **News uses a rolling window.** Each scrape cycle: fetch new items, prepend, trim to max 150, write back.

### 6.2 etcd Key Schema

> ⚠️ This schema is **human-authored and reviewed before any agent session**. Agents are given this document; they do not invent key names.

#### Cluster State

```
/oilfield/nodes/{name}/heartbeat        RFC3339 — written every 30s by scraper
/oilfield/nodes/{name}/status           "ok" | "degraded" | "offline"
/oilfield/nodes/{name}/ip               current public IP
/oilfield/nodes/{name}/provider         "hetzner" | "linode" | "ionos" | "upcloud"
/oilfield/config/scrape_interval        seconds (default "300")
/oilfield/config/active_node            node currently holding scrape lock
```

#### Scrape Lock (lease-backed — CRITICAL)

```
/oilfield/locks/scrape                  node name — MUST USE ETCD LEASE WITH TTL=120s
```

> ⚠️ **Agent Failure Prevention:** The scrape lock MUST be created as an etcd lease with TTL=120s, then a key PUT with `--lease=<lease_id>`. When the scraper process dies, the lease expires and the lock auto-releases within 120s. A plain PUT with no lease creates a permanent lock requiring manual etcd intervention. This is Agent Failure Case #1.

#### Price Keys (one per sector)

```
/oilfield/prices/crude/latest           JSON array of price points
/oilfield/prices/natgas/latest          JSON array
/oilfield/prices/lng/latest             JSON array
/oilfield/prices/lpg/latest             JSON array
/oilfield/prices/ngls/latest            JSON array
/oilfield/prices/electricity/latest     JSON array
/oilfield/prices/refined/latest         JSON array
```

#### Price Point JSON Schema

```json
{
  "symbol":         "CL",
  "name":           "WTI Crude Oil",
  "sector":         "crude",
  "exchange":       "NYMEX",
  "geography":      "US_GULF",
  "delivery_month": "2026-06-01",
  "price":          82.45,
  "unit":           "USD/bbl",
  "scraped_at":     "2026-04-22T14:30:00Z",
  "source":         "eia_api"
}
```

#### News Index

```
/oilfield/news/eia/items                JSON array — last 150 EIA Today in Energy items
/oilfield/news/iea/items                JSON array — last 150 IEA news items
```

#### News Item JSON Schema

```json
{
  "title":        "U.S. LNG Exports Set Monthly Record in March 2026",
  "url":          "https://www.eia.gov/todayinenergy/detail.php?id=XXXXX",
  "published_at": "2026-04-15T00:00:00Z",
  "source":       "EIA",
  "summary":      "Two-sentence plain text summary. Max 300 chars. Never full article text.",
  "tags":         ["lng", "exports", "natgas"]
}
```

### 6.3 Single-Node Degraded Mode

When 2 of 3 nodes are down, the surviving node's etcd enters a read-only state (cannot write — no quorum). The Go API continues serving its local data normally. The scraper cannot acquire the write lock but keeps retrying silently. When a second node returns, quorum restores and writes resume automatically. No manual intervention required for this failure mode.

Emergency standalone mode (manual, for extended outage):
```bash
# Surviving node only — temporary until others rejoin
etcdctl endpoint defrag
systemctl stop etcd
etcd --force-new-cluster   # restores write capability on single node
```

---

## 7. Phase 0 — Manual Bootstrap (Human Required)

These are the only steps a human must perform. Everything after Phase 0 is agentic.

### MANUAL-1: Provider Account Creation & Billing

| Provider | Actions Required |
|----------|-----------------|
| **Hetzner** | cloud.hetzner.com → Add payment → API Tokens → Create Read+Write → save as `HETZNER_API_TOKEN` |
| **Linode (Akamai)** | cloud.linode.com → Sign up → API Tokens → Create Token (Linodes: Read/Write) → save as `LINODE_TOKEN` |
| **Ionos** | my.ionos.com → Create VPS → note root password and IP → set `IONOS_VPS_IP` and `IONOS_VPS_PASSWORD` in `.env` |
| **UpCloud** | upcloud.com → Add payment → API Tokens → Create token → name it `oilfield-deploy` → save token value as `UPCLOUD_API_TOKEN` |
| **EIA** | eia.gov/opendata → Register (free, email required) → save as `EIA_API_KEY` |
| **Cloudflare Pages** | Already in Cloudflare — Pages → Create project → name it `oilfield-dash` → add custom domain `dash.oilfield.parso.guru` (CF manages CNAME automatically) |

### MANUAL-2: SSH Key Generation

```bash
ssh-keygen -t ed25519 -C "oilfield-deploy" -f ~/.ssh/oilfield_ed25519
```

### MANUAL-3: Cloudflare DNS Setup

1. Login → Add domain → Update registrar nameservers → wait for propagation
2. My Profile → API Tokens → Create Token → "Edit zone DNS" → Scope to `parso.guru`
3. Save as `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ZONE_ID`

### MANUAL-4: Create `infra/.env`

```bash
# infra/.env — NEVER commit — add to .gitignore immediately
HETZNER_API_TOKEN=
LINODE_TOKEN=

IONOS_VPS_IP=
IONOS_VPS_PASSWORD=
UPCLOUD_API_TOKEN=
UPCLOUD_ZONE=us-chi1
EIA_API_KEY=
CLOUDFLARE_API_TOKEN=
CLOUDFLARE_ZONE_ID=
CF_PAGES_PROJECT=oilfield-dash
SSH_PUBLIC_KEY_PATH=~/.ssh/oilfield_ed25519.pub
SSH_PRIVATE_KEY_PATH=~/.ssh/oilfield_ed25519
DOMAIN=oilfield.parso.guru
ETCD_CLUSTER_TOKEN=oilfield-etcd-$(openssl rand -hex 8)
```

---

## 8. Phase 1 — Agentic: Provision Scripts

Claude Code writes all four provisioning scripts given the `.env` contract above.

### 8.1 What Each Script Does

Each `infra/provision/<provider>.sh`:
1. Sources `infra/.env`
2. Uploads SSH public key to provider via API (curl only — no SDKs, no Python)
3. Creates VPS: Ubuntu 24.04 LTS, chosen region, chosen plan
4. Polls provider API until VM is active (5-second intervals)
5. Writes `NODE_IP` to `infra/state/<provider>.ip` (gitignored)
6. Calls `infra/bootstrap/base.sh` over SSH

### 8.2 Agent Prompt Template

```
You are writing infra/provision/hetzner.sh for the oilfield Daylight project.
This bash script:
- Sources infra/.env for all secrets
- Uses curl to call the Hetzner Cloud API v1 only — no SDKs, no Python, no Node
- Creates a CX22 server in Ashburn with Ubuntu 24.04
- Polls GET /v1/servers/{id} every 5 seconds until status == "running"
- Writes the server's public IP to infra/state/hetzner.ip
- Exits non-zero on any API error with a descriptive message to stderr
Constraints: bash 5+, curl, jq. Daylight principle: simplest possible solution.
```

> **N2 uses Linode (Akamai Cloud):** Clean REST API — POST /linode/instances with region, type, image, authorized_keys. Region: us-lax (Los Angeles). Type: g6-standard-1 (1 vCPU, 2GB, 50GB).

> **Note on Ionos N3 provisioning:** Ionos VPS (ionos.com/servers/vps) has no programmatic provisioning API. N3 must be created manually at my.ionos.com. Set `IONOS_VPS_IP` and `IONOS_VPS_PASSWORD` in `.env`, then `up.sh` will install the SSH key and run all bootstrap scripts automatically.

> **Note on UpCloud provisioning:** UpCloud API v3 uses HTTP Basic Authentication (`UPCLOUD_USERNAME:UPCLOUD_PASSWORD`). Clean REST endpoints at `api.upcloud.com/1.3`. Comparable in complexity to `hetzner.sh`. SSH key upload and server creation are single POST requests.

---

## 9. Phase 2 — Agentic: Daylight Runtime Setup

### 9.1 `infra/bootstrap/base.sh` (all 4 nodes)

1. `apt-get update && apt-get upgrade -y`
2. Create `deploy` user with sudo, install SSH public key
3. `PermitRootLogin no` in sshd_config
4. UFW: allow 22, 80, 443, 2379, 2380, 8080 (N1–3), 8090 (N4 only)
5. Install: `git`, `curl`, `jq`, `nginx`, `fail2ban`
6. Set hostname to node FQDN
7. Write `/etc/daylight/node.conf` with NODE_ROLE, NODE_NAME, CLUSTER_DOMAIN

### 9.2 `infra/bootstrap/daylight.sh` (N1, N2, N3 only)

1. Install **etcd v3.5.x** — pinned binary from GitHub releases (not apt)
2. Install **dylt CLI** — pinned release binary
3. Write `/etc/etcd/etcd.conf.yml` with cluster config, peer IPs from `infra/state/*.ip`
4. Write and enable `etcd.service` systemd unit
5. Write and enable `oilfield-api.service`, `oilfield-scraper.service`, `oilfield-scraper.timer` units (binaries deployed in Phase 3)

### 9.3 `infra/bootstrap/dns.sh` (runs from local machine)

After all VMs are up, call Cloudflare API to create/upsert all A records per Section 3.1. Verify with `dig`. Output confirmation table.

### 9.4 `infra/bootstrap/tls.sh` (per node)

Install certbot. Obtain Let's Encrypt cert for node FQDN. Configure nginx HTTPS. Auto-renewal via **systemd timer** (not cron — Daylight principle).

---

## 10. Phase 3 — Agentic: The Energy Market Web App

### 10.1 Data Sources — Full Energy Sector Coverage

**Strategy:** EIA API v2 first (structured JSON, free key, no scraping fragility). HTML scraping only for non-EIA international sources.

> ⚠️ **MANUAL STEP EIA-1:** Register for a free EIA API key at `eia.gov/opendata`. ~2 minutes.

#### Sector: Crude Oil

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| WTI Crude | CL | US_GULF | EIA API v2 `/petroleum/pri/fut/` | API |
| Brent Crude | BZ | NORTH_SEA | ICE `theice.com/products/219` | HTML |
| TOCOM Crude | TO | ASIA_PAC | JPX English derivatives page | HTML |
| Dubai Crude | DC | MIDDLE_EAST | EIA API v2 international series | API |
| WTI Midland | WTM | US_PERMIAN | CME Group settlement page | HTML |

#### Sector: Natural Gas

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| Henry Hub | NG | US_GULF | EIA API v2 `/natural-gas/pri/fut/` | API |
| TTF (Netherlands) | TTF | EUROPE | Investing.com TTF futures | HTML |
| NBP (UK) | NBP | UK | Investing.com NBP futures | HTML |
| AECO (Canada) | AECO | CANADA | EIA API v2 natural gas series | API |

#### Sector: LNG

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| JKM (Japan-Korea Marker) | JKM | ASIA_PAC | EIA API v2 STEO series | API |
| DES NWE | LNGE | EUROPE | ICE LNG Europe settlement | HTML |
| HH LNG Netback | HHN | US_EXPORT | EIA API v2 `/natural-gas/move/expc/` | API |
| TTF-HH Spread | THHSP | GLOBAL | Computed: TTF − HH | Derived |

#### Sector: LPG (Liquefied Petroleum Gas)

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| Propane Mont Belvieu | C3 | US_GULF | EIA API v2 `/petroleum/pri/spt/` series `EER_EPLLPA_PF4_Y44MB_DPG` | API |
| Butane Mont Belvieu | C4 | US_GULF | EIA API v2 petroleum spot series | API |
| Propane Seaport (Europe) | C3E | EUROPE | ICIS/Argus public summary page | HTML |
| Naphtha CIF NWE | NAP | EUROPE | Platts public index page | HTML |

#### Sector: NGLs (Natural Gas Liquids)

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| Ethane Mont Belvieu | C2 | US_GULF | EIA API v2 NGL series | API |
| Iso-Butane Mont Belvieu | IC4 | US_GULF | EIA API v2 NGL series | API |
| Natural Gasoline Mont Belvieu | C5+ | US_GULF | EIA API v2 `/petroleum/pri/spt/` | API |
| NGL Composite | NGLC | US | EIA API v2 NGL composite series | API |

#### Sector: Electricity

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| PJM Day-Ahead (West Hub) | PJMW | US_MID_ATL | EIA API v2 `/electricity/rto/` | API |
| CAISO (SP15) | CASP | US_WEST | EIA API v2 `/electricity/rto/` | API |
| ERCOT (Houston Hub) | ERCH | US_TEXAS | EIA API v2 `/electricity/rto/` | API |
| MISO (Illinois Hub) | MISO | US_MIDWEST | EIA API v2 `/electricity/rto/` | API |
| NYISO (Zone A) | NYZA | US_NORTHEAST | EIA API v2 `/electricity/rto/` | API |

> **Note on electricity:** EIA `/electricity/rto/` provides hourly day-ahead prices, not multi-month futures. For the 3D chart, electricity shows a **24-hour day-ahead price profile** (X = hour-of-day, Y = $/MWh, Z = region). This is more accurate than fabricated futures and more useful for the consulting demo.

#### Sector: Refined Products

| Product | Symbol | Geography | Source | Method |
|---------|--------|-----------|--------|--------|
| RBOB Gasoline | RB | US_GULF | EIA API v2 petroleum futures | API |
| ULSD / Heating Oil | HO | US_GULF | EIA API v2 petroleum futures | API |
| Jet Fuel (LAX) | JF | US_WEST | EIA API v2 `/petroleum/pri/spt/` | API |
| Gasoil (ICE) | GAS | EUROPE | ICE Gasoil settlement page | HTML |

#### News Sources

| Source | Method | Endpoint | Frequency |
|--------|--------|----------|-----------|
| EIA Today in Energy | **RSS** | `eia.gov/rss/todayinenergy.xml` | Daily |
| IEA News | **RSS** | `iea.org/news/rss` | 3–5× per week |
| EIA Weekly Petroleum Report | HTML | `eia.gov/petroleum/weekly/` | Weekly (Wed) |
| EIA Natural Gas Weekly | HTML | `eia.gov/naturalgas/weekly/` | Weekly (Thu) |

> **RSS is the correct tool for news.** Both EIA and IEA publish RSS. Parsing RSS in Go (`github.com/mmcdole/gofeed`) is two function calls. No HTML fragility. No rate limit risk. This is the Daylight answer — use the boring, stable protocol designed for the purpose.

### 10.2 Go Backend Modules (Agentic Sessions)

```
Session B1: internal/etcdstore/client.go
  - etcd v3 Go client initialization
  - Helper functions: Get, Put, PutWithLease, Delete, WatchKey
  - Given key schema from docs/etcd-schema.md
  - No business logic — only etcd I/O primitives

Session B2: cmd/scraper/main.go — orchestration only
  - Acquires scrape lock via PutWithLease (TTL=120s)
  - Calls each sector scraper in sequence
  - Writes results to etcd sector keys
  - Writes heartbeat every 30s to /oilfield/nodes/{name}/heartbeat
  - Releases lock on completion or SIGTERM
  - Contains NO scraping logic itself

Session B3: internal/scraper/eia.go
  - EIA API v2 client
  - GetCrudeOilFutures(), GetNatGasFutures(), GetNGLPrices(),
    GetRefinedPrices(), GetElectricityRTO()
  - Uses EIA_API_KEY from environment
  - Returns []PricePoint structs per call

Session B4: internal/scraper/ice.go, cme.go, investing.go, jpx.go
  - HTML scrapers using colly v2
  - One file per source domain
  - Rate limit: 2s between requests per domain (explicit time.Sleep)
  - Returns []PricePoint structs

Session B5: internal/scraper/news.go
  - RSS parser using gofeed
  - Sources: EIA Today in Energy, IEA
  - Returns []NewsItem structs
  - Rolling window: prepend new, trim to 150

Session B6: cmd/api/main.go
  - HTTP server on :8080
  - GET /api/v1/prices/{sector}    reads /oilfield/prices/{sector}/latest
  - GET /api/v1/prices/all         reads all 7 sector keys, returns merged JSON
  - GET /api/v1/news               reads both news keys, merged + sorted
  - GET /api/v1/health             node status, last heartbeat, scrape lock status
  - CORS headers
  - Graceful SIGTERM shutdown

Session B7: systemd unit files
  - oilfield-api.service   (Type=simple, Restart=always)
  - oilfield-scraper.service  (Type=oneshot)
  - oilfield-scraper.timer    (OnCalendar=*:0/5 — every 5 minutes)
```

> ⚠️ **Agent guidance — scraper pattern:** The scraper is a `oneshot` systemd service triggered by a `.timer` unit, NOT a long-running daemon with `time.Sleep` inside. Agents always propose the sleep-loop daemon. The oneshot+timer pattern is correct: the OS manages scheduling, the process starts clean each run, `systemctl status oilfield-scraper` shows last run result cleanly, and log lines have clear boundaries. This is Agent Failure Case #2.

### 10.3 React Frontend — Multi-Sector 3D Energy Chart

**3D Axes:**
- **X:** Delivery month (0–23 months forward) or hour-of-day for electricity
- **Y:** Price (USD/bbl, USD/MMBtu, USD/MWh — labeled per sector)
- **Z:** Product / geography group (one curve per product)

**Sector color coding:**

| Sector | Color |
|--------|-------|
| Crude Oil | Deep blue |
| Natural Gas | Orange |
| LNG | Amber |
| LPG / NGLs | Green |
| Electricity | Electric purple |
| Refined Products | Red |

**Technology:** React 18 + TypeScript, `@react-three/fiber` + `@react-three/drei` (Three.js), `OrbitControls`, `lucide-react` icons, Tailwind utilities only.

**Agentic sessions:**

```
Session F1: EnergyCurve3D.tsx
  - @react-three/fiber 3D scene
  - Accepts: Record<sector, PricePoint[]>
  - Renders one Line3D curve per product
  - Updates via useRef + geometry.setPositions() — does NOT remount scene on data update
  (Agent Failure Case #3: agent will remount entire scene on every 60s price poll,
   spiking CPU to 100%. Must specify the ref-update pattern explicitly.)

Session F2: NewsPanel.tsx
  - Scrollable side panel of NewsItem[]
  - Title + source badge + relative time + external link
  - Max height with overflow-y scroll

Session F3: App.tsx + layout
  - Polls /api/v1/prices/all every 60s
  - Polls /api/v1/news every 300s
  - Sector toggle checkboxes
  - Last-updated timestamp + node health indicator
```

---

## 11. Phase 4 — Agentic: CLI Dashboard (Text/Read-Only)

Runs on N4. Reads etcd and polls each node's `/api/v1/health`. No writes.

### 11.1 Sample Output

```
OILFIELD CLUSTER STATUS  [2026-04-22 14:32:01 UTC]
═══════════════════════════════════════════════════
NODE   PROVIDER    STATUS  HEARTBEAT    SCRAPE LOCK
n1     Hetzner     ● OK    12s ago      —
n2     Linode    ● OK    18s ago      HELD (23s)   ← currently scraping
n3     Ionos       ● OK     9s ago      —

ETCD     Leader: n2 (Linode)    Members: 3/3 healthy

LAST SCRAPE  n2  2026-04-22 14:30:03 UTC  (118s ago)  35 products  4 news items

SPOT PRICES
  crude      WTI (CL)    $82.45/bbl    NYMEX  US_GULF
  crude      Brent (BZ)  $85.12/bbl    ICE    NORTH_SEA
  natgas     HH (NG)     $2.67/MMBtu   NYMEX  US_GULF
  natgas     TTF         $42.10/MMBtu  ICE    EUROPE
  lng        JKM         $12.40/MMBtu  Platts ASIA_PAC
  lpg        Propane C3  $0.72/gal     OPIS   US_GULF
  ngls       Ethane C2   $0.28/gal     OPIS   US_GULF
  elec       ERCOT-H     $38.20/MWh    ERCOT  US_TEXAS
  refined    RBOB (RB)   $2.45/gal     NYMEX  US_GULF
```

### 11.2 Commands

```
oilfield-dash status          # Full status (above)
oilfield-dash nodes           # Node list only
oilfield-dash prices [sector] # Price table, optional sector filter
oilfield-dash news            # Latest news items
oilfield-dash watch           # Auto-refresh every 10s
```

**Implementation:** Go + `charmbracelet/lipgloss` for terminal table styling.

---

## 12. Phase 5 — Agentic: Web Dashboard (Read-Only)

Hybrid architecture: Cloudflare Pages serves the static frontend globally; a Go backend on N4 aggregates cluster data.

### Architecture

```
dash.oilfield.parso.guru   →  Cloudflare Pages (global CDN, free)
                                    │ fetch/XHR API calls
                                    ▼
ctrl.oilfield.parso.guru:443 → nginx → Go backend :8090 (N4/UpCloud)
                                    │ reads
                                    ▼
                              N1/N2/N3 /api/v1/health + /api/v1/prices/all
```

**Backend** (`dashboard/web/backend/main.go`): Go HTTP on `:8090`. Aggregates from all three runtime node `/api/v1/health` endpoints; proxies prices and news from whichever runtime node responds first. CORS configured to allow `https://dash.oilfield.parso.guru`. nginx on N4 terminates TLS for `ctrl.oilfield.parso.guru` and proxies to `:8090`.

**Frontend** (`dashboard/web/frontend/`): React 18. Deployed to Cloudflare Pages project `oilfield-dash`. Custom domain `dash.oilfield.parso.guru` added in CF Pages settings (CF manages the CNAME automatically). Node health grid (green/yellow/red). Embedded `EnergyCurve3D` component. News panel. Auto-refresh every 30s. All API calls go to `https://ctrl.oilfield.parso.guru`.

**Deployment:** `wrangler pages deploy dashboard/web/frontend/dist --project-name oilfield-dash`. Uses `CLOUDFLARE_API_TOKEN` already in `infra/.env`. N4 runs no static file serving — nginx on N4 only proxies the Go backend.

---

## 13. Phase 6 — Agentic: Web Dashboard (State-Changing)

N4 is the only node with write access to the provider APIs and etcd admin operations.

| Operation | Mechanism |
|-----------|-----------|
| Force scrape | Delete `/oilfield/locks/scrape` from etcd |
| Update scrape interval | PUT `/oilfield/config/scrape_interval` |
| Bounce node services | SSH → `systemctl restart oilfield-api oilfield-scraper` |
| Remove node | Cloudflare API delete from A-record sets → `etcdctl member remove` |
| Add node | Run provision + bootstrap scripts → add to etcd cluster → add to DNS |

> ⚠️ **MANUAL STEP SECURITY-1:** Before Phase 6, configure etcd RBAC. Runtime nodes N1/N2/N3 write only their own heartbeat key and the scrape lock. N4 gets a separate privileged service account for state-changing operations. Must be reviewed by a human architect.

---

## 14. Directory Structure

```
oilfield/
├── .gitignore               # infra/.env, infra/state/, node_modules
├── PLAN.md
├── docs/
│   ├── etcd-schema.md       # Human-authored — never agent-generated
│   ├── agent-failures.md    # Failure log for whitepaper
│   └── whitepaper/
├── infra/
│   ├── .env                 # gitignored
│   ├── state/               # gitignored: *.ip files
│   ├── provision/           # hetzner.sh, linode.sh, scaleway.sh, upcloud.sh
│   ├── bootstrap/           # base.sh, daylight.sh, dns.sh, tls.sh
│   ├── deploy/              # deploy-app.sh, deploy-dash.sh
│   └── teardown/teardown-all.sh
├── app/
│   ├── backend/
│   │   ├── cmd/api/main.go
│   │   ├── cmd/scraper/main.go
│   │   ├── internal/etcdstore/client.go
│   │   ├── internal/scraper/eia.go
│   │   ├── internal/scraper/ice.go
│   │   ├── internal/scraper/cme.go
│   │   ├── internal/scraper/investing.go
│   │   ├── internal/scraper/jpx.go
│   │   ├── internal/scraper/news.go
│   │   ├── systemd/oilfield-api.service
│   │   ├── systemd/oilfield-scraper.service
│   │   ├── systemd/oilfield-scraper.timer
│   │   └── go.mod
│   └── frontend/
│       ├── src/components/EnergyCurve3D.tsx
│       ├── src/components/NewsPanel.tsx
│       ├── src/App.tsx
│       ├── package.json
│       └── vite.config.ts
└── dashboard/
    ├── cli/main.go
    └── web/backend/main.go + frontend/
```

---

## 15. Whitepaper Story Arc

*"Sovereign Infrastructure at Scale: Agentic Coding a Multi-Cloud Energy Market Data Platform with Daylight"*

**Section 1: The Problem** — The vibe-coded version: one AWS EC2, RDS PostgreSQL with a read replica, ElastiCache, ALB, CloudWatch. ~$240/month. One region failure = total outage. Primary/replica PostgreSQL that breaks the app when the primary goes down.

**Section 2: The Architecture Decision** — Why PostgreSQL was eliminated. Why etcd is the correct store for a current-state problem. The sizing math: 160 KB of data against an 8 GB quota. Three peers with no primary, true independence.

**Section 3: The Agentic Process** — What Claude Code produced, session by session.

*Documented agent failures:*

- **Failure #1: The immortal scrape lock.** Agent wrote `etcdctl put /oilfield/locks/scrape n2` — a plain PUT with no TTL. When the scraper crashes, the lock never releases. Fixed by: human review of `etcd-schema.md`, which pre-specified lease-backed TTL. The schema document saved the architecture.
- **Failure #2: The sleep-loop scraper.** Agent wrote a long-running Go process with `time.Sleep(5 * time.Minute)`. Replaced with a oneshot systemd service + timer. The OS is a better scheduler than a goroutine.
- **Failure #3: The thrashing 3D chart.** Agent unmounted and remounted the entire Three.js scene on every 60-second price poll. CPU hit 100%. Fixed by: explicit instruction to use `useRef` + `geometry.setPositions()` rather than triggering a React re-render of the scene graph.
- **Failure #4: The rate-limit blizzard.** Agent's CME scraper had no inter-request delay. All three nodes would simultaneously hammer CME's settlement page every 5 minutes. Fixed by: the etcd-backed single scraper lock (already in the design) plus explicit `time.Sleep(2 * time.Second)` added to the agent prompt.

**Section 4: What the Agent Got Right** — All four provision scripts (nearly verbatim), all systemd unit files, nginx config, EIA API v2 client, RSS news parser, etcd client helpers (once given the correct lease pattern), CLI dashboard table layout.

**Section 5: The Numbers** — $15.40/month vs. $240/month. 4-node, 4-provider infrastructure across 4 geographic regions (Hetzner/US-East, Linode/US-West, Ionos/Europe, UpCloud/US-Central) plus Cloudflare Pages CDN at $0. No two nodes share a failure zone. Full energy market coverage. Any two runtime nodes can fail. Dashboard frontend served globally from Cloudflare edge — zero origin load for static assets. Zero vendor lock-in. The Daylight thesis proven numerically.

---

## Appendix A — Manual Steps Reference

| ID | Phase | Step | Time |
|----|-------|------|------|
| MANUAL-1 | Phase 0 | Create accounts + billing (Hetzner, Linode, Ionos, UpCloud + EIA) | 45–90 min |
| MANUAL-2 | Phase 0 | Generate SSH keypair | 2 min |
| MANUAL-3 | Phase 0 | Cloudflare API token + Zone ID (nameservers already transferred) | 5 min |
| MANUAL-3b | Phase 0 | Create Cloudflare Pages project `oilfield-dash`, add custom domain `dash.oilfield.parso.guru` | 5 min |
| MANUAL-4 | Phase 0 | Create `infra/.env` | 10 min |
| DNS-1 | Phase 0 | Nameserver propagation — **already done** (`parso.guru` is in Cloudflare) | 0 min |
| EIA-1 | Phase 0 | Register for free EIA API key | 2 min |
| SCHEMA-1 | Pre-Phase 3 | Review + approve `docs/etcd-schema.md` | 20 min |
| SECURITY-1 | Pre-Phase 6 | Configure etcd RBAC roles | 45 min |

**Total active human time: ~2.5 hours (nameserver propagation wait eliminated)**
**Everything else: agentic**

---

## Appendix B — Cost Summary

| Node | Provider | Plan | Cost/mo |
|------|----------|------|---------|
| N1 — Runtime | Hetzner CX22 | 2 vCPU / 4 GB / 40 GB NVMe | ~$4.15 |
| N2 — Runtime | Linode g6-standard-1 | 1 vCPU / 2 GB / 50 GB SSD | ~$12.00 |
| N3 — Runtime | Ionos VPS XS | 1 vCPU / 1 GB / 10 GB NVMe | ~$2.00 |
| N4 — Control | UpCloud DEV-1SA | 1 vCPU / 1 GB / 10 GB MaxIOPS SSD | ~$3.25 |
| Dashboard Frontend | Cloudflare Pages | Global CDN, static React build | $0 |
| DNS | Cloudflare | Free tier | $0 |
| TLS | Let's Encrypt | Free | $0 |
| EIA API | EIA OpenData | Free | $0 |
| **TOTAL** | | | **~$17.40/mo** |

**AWS equivalent** (EC2 t3.small ×3 + RDS Multi-AZ + ALB + ElastiCache + CloudWatch + CloudFront): ~$230–260/month.
**Savings: ~93%. Vendor lock-in: zero.**

---

## Appendix C — etcd Size Budget

| Key | Products/Items | Est. Size | Notes |
|-----|---------------|-----------|-------|
| `/prices/crude/latest` | 5 products × 24 months | ~18 KB | |
| `/prices/natgas/latest` | 4 products × 12 months | ~7 KB | |
| `/prices/lng/latest` | 3 products × 12 months | ~5 KB | |
| `/prices/lpg/latest` | 4 products × 6 months | ~4 KB | |
| `/prices/ngls/latest` | 4 products × 6 months | ~4 KB | |
| `/prices/electricity/latest` | 5 regions × 24 hours | ~18 KB | Day-ahead hourly |
| `/prices/refined/latest` | 4 products × 12 months | ~7 KB | |
| `/news/eia/items` | 150 items × 300 bytes | ~45 KB | Rolling window |
| `/news/iea/items` | 150 items × 300 bytes | ~45 KB | Rolling window |
| Cluster state (all nodes) | — | < 5 KB | Metadata strings |
| **Grand total** | | **~158 KB** | |

### Headroom

| Limit | Value | Usage | Headroom |
|-------|-------|-------|----------|
| Per-key value size | 1.5 MB | ~45 KB (largest: news) | **33×** |
| Total DB quota | 8 GB | ~158 KB | **50,000×** |
| Request size | 1.5 MB | < 45 KB | **33×** |

Doubling all products, adding 5 more news sources, extending forward curves to 36 months: estimated total ~700 KB — still well within a single per-key value limit. etcd is not the constraint for this application at any foreseeable scale of energy market data.

---

---

## 16. Phase 7 — Systems Test (Human-Run Runbook)

End-to-end validation from bare metal to live dashboard. All steps use scripts built in Phases 1–6.

### 16.0 Meta Scripts (Recommended)

Two top-level scripts orchestrate everything:

| Script | Purpose |
|--------|---------|
| `./infra/up.sh` | Full bring-up: provision → bootstrap → DNS → TLS → deploy (all parallel where safe) |
| `./infra/teardown/teardown-all.sh` | Delete all four VMs; pass `--yes` to skip confirmation |

**Bring up the entire cluster:**
```bash
export ADMIN_EMAIL=john@parso.guru   # required by tls.sh (Let's Encrypt registration)
./infra/up.sh
```

`up.sh` runs the four provision scripts in parallel, then daylight.sh on N1/N2/N3 in parallel, then dns.sh, then tls.sh on all four nodes in parallel, then deploy-app.sh and deploy-dash.sh. Total wall-clock time: ~25–35 min.

**Tear down the entire cluster:**
```bash
./infra/teardown/teardown-all.sh           # prompts "Type 'yes' to confirm"
./infra/teardown/teardown-all.sh --yes     # skip prompt (CI use)
REMOVE_DNS=1 ./infra/teardown/teardown-all.sh  # also delete Cloudflare A records
```

---

### 16.1 Provision (individual scripts, or use up.sh above)

```bash
./infra/provision/hetzner.sh   # → infra/state/hetzner.ip  (N1, US-East)
./infra/provision/linode.sh  # → infra/state/linode.ip (N2, US-West (Linode))
./infra/provision/scaleway.sh  # → infra/state/scaleway.ip (N3, Europe)
./infra/provision/upcloud.sh   # → infra/state/upcloud.ip  (N4, US-Central)
```

### 16.2 Bootstrap (individual scripts, or use up.sh above)

base.sh is called automatically by each provision script. After all four IPs exist:

```bash
# daylight.sh requires all 3 runtime IPs — run after all provision scripts complete
N1_IP=$(cat infra/state/hetzner.ip)
N2_IP=$(cat infra/state/linode.ip)
N3_IP=$(cat infra/state/ionos.ip)

for IP in $N1_IP $N2_IP $N3_IP; do
  ssh -i ~/.ssh/oilfield_ed25519 deploy@$IP \
    "N1_IP=$N1_IP N2_IP=$N2_IP N3_IP=$N3_IP bash -s" < infra/bootstrap/daylight.sh
done

# DNS — run once from local machine after all IPs are available
./infra/bootstrap/dns.sh

# TLS — run on each node (Let's Encrypt requires domain to resolve first)
ADMIN_EMAIL=john@parso.guru
for NODE in n1 n2 n3 ctrl; do
  ssh deploy@${NODE}.oilfield.parso.guru 'ADMIN_EMAIL=john@parso.guru bash -s' < infra/bootstrap/tls.sh
done
```

### 16.3 Deploy (individual scripts, or use up.sh above)

```bash
./infra/deploy/deploy-app.sh all   # cross-compile api+scraper → scp → restart on N1/N2/N3
./infra/deploy/deploy-dash.sh      # Go backend → N4; React frontend → Cloudflare Pages
```

### 16.4 Verify — CLI Dashboard

```bash
export OILFIELD_DOMAIN=oilfield.parso.guru
go run ./dashboard/cli/... status    # Expect: n1/n2/n3 ● OK, prices populated
go run ./dashboard/cli/... nodes     # Expect: heartbeats <60s ago
go run ./dashboard/cli/... prices crude  # Expect: WTI, Brent, TOCOM prices
go run ./dashboard/cli/... news      # Expect: EIA + IEA headlines
go run ./dashboard/cli/... watch     # Expect: auto-refresh every 10s
```

### 16.5 Verify — Web Dashboard

```
open https://dash.oilfield.parso.guru
```

Expected: NodeHealthGrid shows n1/n2/n3 green, 3D energy chart renders with curves per product, NewsPanel shows news, sector toggles work, admin panel (gear icon) accessible with ADMIN_TOKEN.

### 16.6 Verify — Price App

```
open https://oilfield.parso.guru
```

Expected: React app served by nginx from N1/N2/N3 round-robin, 3D chart interactive, prices updating.

### 16.7 Verify — Admin Panel (Phase 6)

In `dash.oilfield.parso.guru`:
1. Click gear icon → enter `$ADMIN_TOKEN`
2. Click "Force Scrape Now" → lock clears within 2s → next scraper run starts
3. Set interval to 600 → Save → confirm `oilfield-dash nodes` shows new interval
4. Click N1 Restart → confirm N1 services restart, rejoin within 30s

### 16.8 Pass/Fail Criteria

| Check | Pass |
|-------|------|
| `oilfield-dash status` | n1 ● OK, n2 ● OK, n3 ● OK |
| `oilfield-dash prices crude` | ≥ 2 price points with real USD values |
| `dash.oilfield.parso.guru` loads | 3D chart renders, no console errors |
| Force Scrape via admin | Lock clears within 2s |
| etcd quorum survives N1 restart | `oilfield-dash status` recovers within 30s |
| Total monthly cost | ≤ $17.40/mo (see Appendix B) |

---

*End of PLAN.md — Parso Consulting / oilfield project*
*Revision 4: added meta scripts up.sh + teardown-all.sh, Phase 7 runbook updated*
*Last updated: April 2026*

---

## Appendix C — Daylight Control Console (Phase 8)

> **Codename:** `daylight-ctrl`  
> **Design brief:** Demo-focused, cyberpunk terminal aesthetic, world map of nodes, fully mobile-usable. Unauthenticated read-only; admin actions gated by existing ADMIN_TOKEN.

### C.1 Decisions (from design session, 2026-04-25)

| Question | Answer |
|---|---|
| Node metrics | Pull-based: SSH into node, read `/proc/loadavg` + `free -b`. No new binary. |
| Log access | Last N lines on demand (`journalctl -n 100 --no-pager`). No streaming. |
| Auth | Map/status view is public (unauthenticated). Admin actions (bounce, force-scrape) keep existing Bearer token gate. |
| Placement | Same CF Pages project (`oilfield-dash`). Accessed via ADMIN button in existing header → switches view mode. |
| Mobile | First-class: same responsive strategy as oilfield dashboard. |
| Aesthetic | Cyberpunk/terminal: dark background, neon green/cyan, scan-line overlay, glowing borders, monospace font. |
| Map | Leaflet.js + CartoDB Dark Matter tiles (free, no API key). SVG polylines animate between nodes. |

### C.2 New API Endpoints (dashboard/web/backend)

| Method | Path | Cache TTL | Description |
|---|---|---|---|
| GET | `/api/v1/nodes` | 15 s | All nodes: geo, status, etcd role, heartbeat |
| GET | `/api/v1/nodes/{name}/metrics` | 30 s | SSH pull: load avg, RAM, uptime |
| GET | `/api/v1/nodes/{name}/logs` | 60 s | SSH pull: last 100 journalctl lines |

All three are read-only. Cache prevents DDOS-by-refresh from demo audience. Node geo stored in etcd at `/daylight/nodes/{name}/geo` (JSON); falls back to `DAYLIGHT_NODE_GEO` env-var JSON array.

### C.3 Frontend Components

```
AdminConsole.tsx          ← full-screen view, cyberpunk chrome
  WorldMap.tsx            ← Leaflet map, dark tiles, node markers + SVG connection lines
  NodeGrid.tsx            ← status cards below/beside map, one per node
  NodeDetailDrawer.tsx    ← slide-in panel: metrics gauges + log terminal
```

### C.4 Layout (responsive)

```
Desktop (≥768px)
┌─────────────────────────┬────────────────┐
│  WORLD MAP (Leaflet)    │  NODE CARDS    │
│  with SVG lines         │  (scrollable)  │
│                         │                │
│                         │  [selected]    │
│                         │  metrics+logs  │
└─────────────────────────┴────────────────┘

Mobile (<768px)
┌──────────────────────┐
│  WORLD MAP           │  (60vh)
│  tap node → details  │
├──────────────────────┤
│  NODE CARDS (H-scroll)│ (fixed height)
├──────────────────────┤
│  DETAIL / LOGS        │ (flex remaining)
└──────────────────────┘
```

### C.5 Cyberpunk Aesthetic Spec

- **Background:** `#000d1a` (deep navy black)
- **Primary neon:** `#00ff9f` (matrix green)
- **Secondary neon:** `#00d4ff` (cyan)
- **Alert neon:** `#ff0080` (hot pink)
- **Borders:** `1px solid #00ff9f33` with `box-shadow: 0 0 8px #00ff9f22`
- **Font:** `'Courier New', Courier, monospace` everywhere
- **Scan lines:** CSS `repeating-linear-gradient` overlay, 2px period, 5% opacity
- **Map markers:** pulsing SVG circles, colour by health (green/amber/red neon)
- **Connection lines:** animated SVG dashes cycling along the great-circle path
- **Node cards:** dark glass `background: #00ff9f08`, neon border glow on hover

### C.6 Implementation Phases

1. Backend: cache primitives + `/nodes` + `/metrics` + `/logs` endpoints
2. Frontend: install react-leaflet; WorldMap with markers and lines
3. Frontend: NodeGrid cards + NodeDetailDrawer
4. Frontend: AdminConsole layout wired into App.tsx ADMIN button
5. Deploy + test on mobile

*End Appendix C*
