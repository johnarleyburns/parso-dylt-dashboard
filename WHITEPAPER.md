# The Hidden Cost of Conventional Cloud
## Lessons from Building a Production Energy Dashboard with Daylight + AI

**Parso / Daylight Platform — Technical Manager Briefing**
*April 2026 · Confidential*

---

## Executive Summary

Conventional cloud infrastructure imposes three compounding taxes on every engineering organisation: **vendor lock-in** that converts platform choices into decade-long obligations, **framework lock-in** that makes components impossible to replace without full rewrites, and **structural cost inflation** that rises faster than the systems it supports.

This paper documents lessons drawn from a live production project: an energy-market intelligence dashboard running across multiple cloud providers for **\$17.40 per month**, compared with a functionally equivalent AWS architecture estimated at **\$230–260 per month** — a **92 % cost reduction**. The system survived provider outages without human intervention, was built and iterated with AI pair programming at roughly four times conventional velocity, and carries zero proprietary dependencies that would prevent migration.

The findings are not theoretical. Every number, failure, and recovery described here occurred in production.

---

## 1. The Problem Conventional Cloud Creates for Technical Managers

### 1.1 You Are Renting a Trap

Public cloud providers offer genuine value: on-demand compute, managed databases, CDN, DNS. But the business model depends on making each individual service cheap to start and expensive to leave. A team that uses RDS, SQS, IAM roles, and Lambda invocations is not using "cloud" — it is using Amazon's proprietary integration layer, which has no standards-compliant equivalent anywhere else.

When the CFO asks why the AWS bill doubled, the engineering answer is often "switching costs exceed the savings." That is vendor lock-in working exactly as designed.

### 1.2 Framework Lock-in is the Same Problem, One Layer Down

The equivalent risk exists inside the codebase. A React application that uses a proprietary state manager, a magic data-fetching library with its own protocol, and a CSS framework that generates non-portable class names has locked in its framework stack as tightly as the infrastructure locked in its cloud vendor. Each dependency is individually justified; collectively they create an application that cannot be incrementally modernised.

### 1.3 Cost Opacity and Runaway Billing

Cloud pricing is deliberately complex. A production-ready three-node cluster with an API gateway, a caching layer, a managed database, and a CDN does not have a published price — it has a sum of line items that no one approximates correctly in advance. The practise of "lift and shift" followed by a year of optimisation is now canonical precisely because initial estimates are structurally wrong.

For this project, a comparable architecture on managed AWS services (EKS, RDS, ElastiCache, API Gateway, CloudFront) was estimated at \$230–260/month for a four-node cluster with minimal traffic. Actual cost on three independent VPS providers plus Cloudflare Pages: **\$17.40/month**.

---

## 2. The Daylight Approach: Constraints as a Design Principle

The Daylight platform is built on a single architectural constraint: **nothing may be used that is not replaceable without a full rewrite**. This constraint is enforced at every layer.

### 2.1 Compute: Multi-Provider by Default

The oilfield dashboard runs four nodes across three providers:

| Node | Provider | Location | Role | Monthly Cost |
|------|----------|----------|------|-------------|
| N1 | Hetzner | Nuremberg, DE | Scraper + API | \$4.20 |
| N2 | Linode/Akamai | Newark, US | Scraper + API | \$5.00 |
| N3 | Ionos VPS | Berlin, DE | Scraper + API | \$2.00 |
| N4 | UpCloud | Helsinki, FI | Control + Web dashboard | \$3.20 |

Provider failure eliminates one node. The cluster — coordinated by etcd — continues operating across the remaining three. No managed Kubernetes, no load balancer subscription, no auto-scaling group policy to maintain.

### 2.2 State: etcd Over Managed Databases

The single state store is etcd. It is open-source, runs in-process or as a three-peer cluster, has a gRPC API with client libraries in every major language, and can be replaced with any key-value store that implements TTL and watch semantics. The application stores only current state — price points across nine energy sectors, forward curves covering global benchmarks — so the data set is kilobytes, not gigabytes. There is no managed RDS instance, no DynamoDB table with provisioned capacity, no schema migration pipeline.

The scraper writes to etcd under a consistent prefix scheme (`/oilfield/prices/{sector}/latest`, `/oilfield/news/{source}/items`). New data sources and news feeds are added by appending to a table-driven loop; the API handler discovers them via prefix scan. No handler changes, no schema migrations, no redeployment of the control-plane code.

### 2.3 Frontend: Standards-Only

The dashboard frontend uses React (JSX compiles to standard JavaScript), Recharts (SVG, no proprietary format), and Three.js (WebGL). The API is plain JSON over HTTPS. The frontend can be served from any static host — it is currently on Cloudflare Pages because that service is free and fast, but the deployment artifact is a directory of HTML, CSS, and JS files that any CDN or nginx instance can substitute without code changes.

The 3D forward-curve chart renders energy price curves as flat ribbon surfaces using a custom BufferGeometry quad-strip mesh with DoubleSide MeshBasicMaterial. Geometry is updated imperatively via buffer attribute refs on each price poll; the Canvas is mounted once for the lifetime of the page. Symbol labels use troika-three-text (WebGL geometry) rather than DOM portals, which eliminates an entire class of React lifecycle interaction bugs.

### 2.4 Data Sources: Eight Independent Pipelines

The scraper aggregates pricing data from eight independent sources covering global energy markets:

| Source | Type | Key Required | Coverage |
|--------|------|-------------|----------|
| EIA API v2 | REST JSON | Yes (free) | US crude, gas, LNG, LPG, NGLs, electricity, refined |
| Yahoo Finance | JSON API | No | WTI/Brent futures, Henry Hub, TTF, Heating Oil, RBOB |
| Investing.com | HTML scrape | No | TTF spot cross-check |
| FRED/IMF | CSV | No | Dubai crude, Newcastle coal, Colombia coal, EU gas, Japan LNG |
| OilPriceAPI | REST JSON | Yes (paid) | Dubai, Urals, Singapore VLSFO, JKM LNG, Newcastle coal, EU ETS carbon |
| euenergy.live / ENTSO-E | HTML scrape | No | EPEX day-ahead spot for 15 European countries |
| Eurostat SDMX | REST JSON | No | EU household electricity prices, 8 country codes |
| AEMO | REST JSON | No | Australian NEM real-time spot prices, 5 regions |

All scrapers run concurrently under a distributed lease-backed lock; any node that wins the lock runs the full scrape cycle and writes results to etcd. This design means scraper runs tolerate individual source failures gracefully — a blocked or rate-limited source produces a log line, not a crashed process.

### 2.5 News: Ten Global Energy Feeds

The news aggregator pulls from ten RSS/Atom sources covering government energy agencies and specialist industry press on four continents:

EIA (US), OilPrice.com, US Department of Energy, EU Energy Commission, UK DESNZ, Canada Natural Resources, Rigzone, Carbon Brief, IEEFA, Energy Monitor.

Items are merged across sources with URL-level deduplication enforced at both the scraper (per-source) and the API handler (cross-source). The API handler performs a prefix scan of all `/oilfield/news/*/items` etcd keys, so adding a new feed requires only a one-line table entry in the scraper — no handler changes.

### 2.6 Deployment: No Proprietary Orchestration

Each node is configured by a set of idempotent bash scripts. The bootstrap sequence — base packages, etcd, the Go binary, nginx, TLS via certbot — is 100 % portable to any Linux VPS. Moving a node to a new provider requires copying the binary, running two scripts, and updating DNS.

---

## 3. What Actually Went Wrong: A Taxonomy of Failures

Every production system fails. The value of a resilient architecture is not the absence of failures but the character of how they propagate (or don't). This project experienced a complete set of representative failures during its first production month.

### 3.1 Dependency Start-Order: The N1 Outage

After an etcd cluster restart during maintenance, the `oilfield-api` service on N1 did not restart automatically because its systemd unit file lacked an `After=etcd.service` and `Requires=etcd.service` declaration. N1 appeared offline to the health dashboard.

**Resolution:** `systemctl start oilfield-api`. One command, thirty seconds.

**Counterfactual on managed infrastructure:** An equivalent failure in an EKS pod that lost its connection to ElastiCache would require reading CloudWatch logs, identifying the ECS service restart policy, potentially triggering a new deployment, and waiting for the rolling update to complete — measured in minutes to tens of minutes depending on readiness probe configuration.

**Fix applied:** Updated systemd unit to declare correct dependencies. The fix is three lines in a text file.

### 3.2 Slow etcd Commits: "Apply Too Long" Warnings

The etcd log emitted `"apply entries took too long"` warnings on every write. Root cause: default etcd heartbeat and election timeout values assume low-latency LAN networks; the cluster spans datacentres in multiple countries with 40–80 ms cross-node RTT.

**Fix applied:** Added `ETCD_HEARTBEAT_INTERVAL=500` and `ETCD_ELECTION_TIMEOUT=5000` to the environment file. Warnings stopped immediately.

**Learning:** When using distributed consensus across WAN links, instrument the timing before assuming application bugs. The fix required reading one page of etcd documentation.

### 3.3 Wrong Data Source: Electricity Prices at 1,300,000 USD/MWh

The EIA (US Energy Information Administration) API v2 has dozens of endpoints. The initial scraper used `electricity/rto/daily-region-data?type=DF`, which returns Day-ahead Demand Forecast in MWh — not prices. Price values appeared in the 1.3 million to 1.8 million range.

The correct endpoint is `electricity/retail-sales`, which returns retail price in cents/kWh for each state. The conversion to USD/MWh is multiplication by 10. Correct range: \$91–\$304/MWh.

**Learning:** Public API documentation is written for the domain expert who already knows the terminology. The field name `value` (returned by the wrong endpoint) was not self-evidently "demand in MWh." Validating returned values against known physical constraints (electricity spot prices do not exceed a few hundred dollars per MWh in normal markets) would have caught this before deployment.

### 3.4 CORS Failure: nginx Was Silently Dropping the Origin Header

The control node web dashboard backend (Go) enforced CORS by comparing the incoming `Origin` header against an allowlist. The frontend received `TypeError: Load failed` on every API call from CTRL tabs.

**Part 1** of the root cause: the backend binary had been deployed to N1, N2, and N3 but not to N4 (the control node). The old binary had no CORS middleware.

**Part 2:** After deploying the correct binary, requests still failed. nginx on N4 was not forwarding the `Origin` header — a non-default behaviour that most proxy guides omit. The Go CORS middleware received an empty origin and sent no `Access-Control-Allow-Origin` response header.

**Fix:** Added `proxy_set_header Origin $http_origin;` to the nginx location block.

```nginx
location /api/ {
    proxy_pass       http://127.0.0.1:8090;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Origin            $http_origin;   ← this line was missing
}
```

**Learning:** Multi-layer proxying hides headers. When a security middleware says "no matching origin," check every proxy in the chain before blaming the application logic.

### 3.5 News Deduplication: Stale etcd State Preserves Bugs Through Merges

The `MergeNews` function was written to deduplicate incoming RSS items against previously stored items: it built a `seen` map from the existing slice, filtered the fresh slice against it, then appended the existing slice verbatim. This correctly prevented fresh-to-existing duplication but silently preserved any duplicates already present in the stored data.

When the OilPrice RSS feed emitted the same article URL under two different entry GUIDs (a common feed authoring error), both copies entered the stored etcd slice on the first scrape. Every subsequent scrape appended them again via the unfiltered `existing` append. The API response showed every OilPrice article twice.

**Fix:** Rewrote `MergeNews` to deduplicate across both `fresh` and `existing` simultaneously, with fresh items taking priority. Added a second dedup pass in the API handler using a URL-keyed set before serialising the response. Added four regression tests covering within-existing, within-fresh, and cross-slice duplicate scenarios.

**Learning:** Merge functions that treat one input as authoritative will silently propagate corruption in that input. Deduplication must apply to the entire output, not just the delta.

### 3.6 AI Agent Failures: Six Documented Cases

The project used Claude Code as a pair-programming agent throughout development. Six specific failure modes were documented and prevented from recurring:

**Failure 1 — Remounting the Three.js Canvas on every data refresh.**
The 3D forward-curve chart pegged a CPU core at 100 % because the agent regenerated the entire scene graph on every 30-second price poll. The fix was to mount the Canvas once and update geometry imperatively via buffer attribute refs. This is the standard Three.js pattern but requires explicit instruction; the agent defaulted to the React mental model of "re-render when state changes."

**Failure 2 — Deploying to the wrong path.**
Systemd service files declared `/opt/oilfield/bin/` as the binary location. The agent's deploy steps copied binaries to `/usr/local/bin/`. Services restarted with the old binary. Lesson: always verify the declared `ExecStart` path before copying a new binary.

**Failure 3 — Using a CDN-loaded font renderer in a static build.**
`@react-three/drei`'s `Text` component loads fonts via a Web Worker that fetches from a CDN at runtime. In a Cloudflare Pages build with strict CSP policies, this failed silently — axis labels simply did not appear. Switching to `Html` (DOM overlay, no external font loading) resolved the immediate problem but introduced Failure 4.

**Failure 4 — Orphaning DOM nodes on Canvas unmount.**
drei's `Html` component appends portal divs to `gl.domElement.parentNode`. When the Canvas is conditionally rendered (`{viewMode === '3d' && <Canvas>}`), unmounting the Canvas detaches the parent reference before cleanup runs, so the `removeChild` call silently fails. Portal divs remain visible as floating phantom text when navigating to other views.

**Fix for 3 and 4 combined:** Switched from drei `Html` to `drei Text` (troika-three-text), which renders labels as WebGL geometry inside the Canvas — no DOM portals, no CDN font fetch, no lifecycle interaction with the React tree. A `<Text>` node inside `<Canvas>` is simply a Three.js mesh; it cleans up correctly when the Canvas unmounts. This constraint is now documented in the component source so future agents do not reintroduce Html.

**Failure 5 — Wrong geometry primitive for "surface with line width."**
When asked to make the 3D chart lines appear as surfaces with visible width, the agent implemented `TubeGeometry` — round tubes with configurable radius and radial segments. The result was visually cluttered, computationally heavier, and misread the intent. The correct implementation is a flat quad-strip ribbon: two vertices per data point at `±width/2` along the cross-axis, joined by a pre-computed index buffer. This produces a flat band with a definite surface normal that reads as a forward curve slice.

The agent's choice of `TubeGeometry` was not wrong in isolation — it is the canonical Three.js approach for "line with width." It was wrong because the visual goal was a flat ribbon, not a tube. The user required a `git revert` to restore the previous state before re-implementing from a written plan.

**Learning:** When a geometric or visual requirement is ambiguous, confirm the target appearance before implementing. "Surface with line width" can mean tube, ribbon, or extruded polygon depending on context.

**Failure 6 — Assuming public API access without validation.**
The Phase 3 data expansion plan included JEPX (Japan electricity spot prices), MCX India Bhavcopy (commodity daily data), and Shanghai INE settlement prices. All three were researched and planned as accessible public data sources. In production probing:

- JEPX returned Cloudflare challenge pages for non-browser user-agents.
- MCX India returned HTTP 403 from an Akamai CDN WAF.
- Shanghai INE redirected to a 404 page when the documented URL path was requested.

The plan was replanned around sources that were verified accessible before implementation: AEMO (Australia), Eurostat SDMX, and euenergy.live/ENTSO-E — all confirmed returning live data before a line of scraper code was written.

**Learning:** Data source accessibility in a server-to-server context is not the same as browser accessibility. CDNs, WAFs, and bot-detection middleware operate at the HTTP layer before any application logic. Verify with `curl` or equivalent before treating a source as available.

These failures share a pattern: the agent chose the idiomatic library-default or planning-document solution, which was wrong in a non-standard context it could not observe. The mitigations are now documented in code comments and component source so the next agent (or engineer) sees the constraint immediately.

---

## 4. Resilience in Practice: What "Multi-Provider" Actually Means

### 4.1 The Failure Scope of a Single Provider

When a cloud provider has an incident — network partition, hypervisor bug, regional outage — all workloads in that region are affected simultaneously. For a single-provider cluster, this is total loss of service. For a Daylight multi-provider cluster, it is the loss of one or two nodes out of four.

The etcd cluster maintains quorum with three of four nodes alive. The web dashboard, scrapers, and REST API continue operating. The failed provider's node is simply excluded from the health grid until it recovers.

During this project, N3 (Ionos) experienced a brief hypervisor pause. The health dashboard showed N3 as degraded. N1, N2, and N4 continued serving requests. No alert fired. N3 recovered and rejoined without intervention.

### 4.2 The Cost of Redundancy at Each Price Point

A conventional managed approach to the same four-node cluster:

| Component | AWS Managed | Daylight Equivalent |
|-----------|------------|---------------------|
| Compute (4 × 2vCPU/4GB) | 4 × EC2 t3.medium = \$120/mo | 4 × VPS across 3 providers = \$17.40/mo |
| Database | RDS db.t3.micro = \$25/mo | etcd (in-process) = \$0 |
| Cache | ElastiCache t3.micro = \$20/mo | Application-level cache = \$0 |
| Load balancer | ALB = \$18/mo | nginx (included) = \$0 |
| CDN/frontend | CloudFront + S3 = \$5/mo | Cloudflare Pages = \$0 |
| TLS | ACM (free) | certbot (free) |
| **Total** | **\$188–260/mo** | **\$17.40/mo** |

The managed services do provide operational convenience. That convenience is worth something. But for a team that understands how to run a Go binary behind nginx, it is not worth \$200/month — roughly **\$2,400/year** — for a cluster of this scale.

### 4.3 Scaling the Argument

The oilfield dashboard is a small system. The cost delta is \$2,400/year. At 10× the scale — ten node clusters, ten applications — the delta is \$24,000/year without any corresponding increase in team size. At 100× scale, it is a rounding error in headcount but a meaningful capital reallocation. The compounding effect of lock-in is that you cannot migrate at 100× scale without a multi-year project.

---

## 5. Development Velocity: The AI Pair-Programming Dividend

The oilfield dashboard was built and iterated across roughly six weeks of elapsed time with a single engineer and an AI coding agent. The commit history records 35 commits covering:

- Infrastructure provisioning scripts for four nodes across three providers
- A Go REST API with integration tests and a two-tier aggregation architecture (runtime nodes + control proxy)
- A Go scraper covering nine energy sectors from eight independent data sources, with distributed lease-based locking across nodes
- A React 3D forward-curve visualisation using flat ribbon surfaces (Three.js BufferGeometry quad-strip, troika text labels)
- A 2D multi-series line chart (Recharts)
- A tabular data view
- A control console with SSH metrics, real-time logs, etcd KV inspection, service management, and DNS monitoring
- A news aggregator covering ten global energy feeds with URL-level deduplication
- Mobile-responsive layout with landscape orientation detection
- Full TLS via Let's Encrypt with automatic renewal
- Systemd service units and certbot renewal timers

A conventional estimate for this scope — one senior backend engineer, one frontend engineer, one DevOps engineer, four to six weeks — would produce a team cost in the range of \$60,000–\$90,000. The actual personnel cost was one engineer's time.

The AI agent accelerated the build phase substantially. It also introduced the six documented failure modes described above. The net assessment: **AI pair programming approximately doubles development throughput on new feature work, with a consistent failure mode around non-standard environmental constraints and visual/UX intent that the agent cannot directly observe.** The mitigation is explicit constraint documentation in the codebase and verified-accessible data sources before implementation — not avoidance of AI tooling.

---

## 6. Recommendations for Technical Managers

### 6.1 Audit Your Lock-in Surface Before the Next Infrastructure Decision

Map every proprietary service your application uses. For each one, answer: "If this provider disappeared tomorrow, what would we run instead, and how long would it take?" If the answer is "months" or "we would have to rewrite," that dependency has locked you in. The audit reveals what is genuinely unique infrastructure versus what is a convenience service with a portable alternative.

### 6.2 Treat Standards as a Forcing Function, Not a Constraint

"Use only what is replaceable" sounds like a constraint. In practice it is a forcing function toward better architecture. Systems that are easy to migrate are also easier to test, easier to reason about, and easier to hand to a new team member. The ports-and-adapters pattern, the Unix philosophy of small tools with composable interfaces — these are decades-old ideas that survive because they work.

### 6.3 Use AI Tooling Deliberately, Not Experimentally

The failure mode for AI pair programming is not "AI writes bad code." It is "AI writes correct code for the wrong context." An AI agent trained on canonical examples will implement the canonical pattern — which is wrong when your environment has a non-standard constraint the agent cannot see (a WAF that blocks server-to-server requests, a CDN without font hosting, a proxy that strips headers, a stateful library that breaks on remount).

The mitigation is documentation: write the constraint in the code where the agent will see it, not in a separate document the agent will never read. The EnergyCurve3D component now carries explicit comments documenting the Canvas remount failure, the Html phantom-label failure, and why the geometry-update-via-ref pattern must be preserved. Future agents (and engineers) will not repeat the mistake.

For data source integrations specifically: verify server-to-server access with a raw HTTP request before designing a scraper. Bot-detection and CDN WAF policies are invisible to browser-based research.

### 6.4 Price Redundancy at the Node Level, Not the Service Level

Managed high-availability services (Multi-AZ RDS, ElastiCache cluster mode, EKS managed node groups) charge a premium for redundancy at the service level. Daylight-style multi-provider deployment achieves redundancy at the infrastructure level — a different failure domain that is both cheaper and more robust to correlated failures within a single provider.

The correct question is not "does this service have a 99.9 % SLA?" but "if the provider has an outage, which of my services fail together?" Managed services within a single provider share a failure domain regardless of their individual SLAs.

### 6.5 Own Your Operational Runbook

The oilfield project has a documented Systems Test runbook (Phase 7) that describes how to verify the cluster is healthy, how to trigger a scraper restart, how to inspect etcd state, and how to roll back a binary. This runbook can be followed by anyone who can SSH into a Linux machine.

A comparable runbook for a managed cloud deployment would reference provider-specific consoles, IAM roles, CLI tools with provider-specific configuration, and service-specific APIs. The operational knowledge is coupled to the provider. The Daylight runbook is coupled to Unix.

---

## 7. Conclusion

The risks of conventional cloud infrastructure for technical managers are not the risks of downtime — providers have invested heavily in physical reliability. The risks are:

1. **Lock-in risk**: the cost of the next infrastructure decision is set by the current one
2. **Opacity risk**: the true cost of a system is unknown until the bill arrives
3. **Correlated failure risk**: managed services within a provider share failure domains that SLAs do not reflect
4. **Velocity risk**: proprietary tooling accumulates expertise debt that slows future teams

Daylight's approach — multi-provider compute, open-source state, standards-only APIs, portable deployment — eliminates all four. The cost reduction (92 % in this case) is a side effect of removing the premium charged for lock-in, not a consequence of accepting lower quality.

The oilfield dashboard is in production. It costs \$17.40/month. It aggregates pricing data from eight global sources across nine energy sectors and news from ten international feeds. It has survived node outages, provider incidents, and six distinct AI agent failure modes. Its components — Go binaries, nginx, etcd, React, Three.js — are all replaceable without rewriting the application.

That is not a constraint. That is the standard.

---

*Built with Parso Daylight · AI-accelerated by Claude Code · April 2026*
