package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	listenAddr     = ":8090"
	requestTimeout = 10 * time.Second
	nodeTimeout    = 5 * time.Second
)

// ---- Runtime node aggregation ----

// RuntimeNode pairs a logical name ("n1") with a base URL.
type RuntimeNode struct {
	Name    string
	BaseURL string
}

func defaultNodes() []RuntimeNode {
	domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
	return []RuntimeNode{
		{Name: "n1", BaseURL: "https://n1." + domain},
		{Name: "n2", BaseURL: "https://n2." + domain},
		{Name: "n3", BaseURL: "https://n3." + domain},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Aggregator runs concurrent requests against the runtime node set.
type Aggregator struct {
	nodes []RuntimeNode
	http  *http.Client
}

func newAggregator(nodes []RuntimeNode) *Aggregator {
	return &Aggregator{
		nodes: nodes,
		http:  &http.Client{Timeout: nodeTimeout},
	}
}

// fetch performs a GET and returns the body on 2xx, otherwise an error.
func (a *Aggregator) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// first races all nodes for path; returns body from the first 2xx response.
// Cancels remaining in-flight requests once a winner responds.
func (a *Aggregator) first(ctx context.Context, path string) ([]byte, string, error) {
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		body []byte
		name string
		err  error
	}
	ch := make(chan result, len(a.nodes))

	var wg sync.WaitGroup
	for _, n := range a.nodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := a.fetch(raceCtx, n.BaseURL+path)
			ch <- result{body, n.Name, err}
		}()
	}
	go func() { wg.Wait(); close(ch) }()

	var lastErr error
	for r := range ch {
		if r.err == nil {
			cancel()
			return r.body, r.name, nil
		}
		lastErr = r.err
	}
	return nil, "", lastErr
}

// NodeHealth mirrors the runtime node's /api/v1/health response.
type NodeHealth struct {
	Node        string `json:"node"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
	EtcdHealthy bool   `json:"etcd_healthy"`
}

// healthAll fetches /api/v1/health from every node concurrently.
// Unreachable nodes are marked offline; result is keyed by node name.
func (a *Aggregator) healthAll(ctx context.Context) map[string]NodeHealth {
	type result struct {
		name string
		h    NodeHealth
	}
	ch := make(chan result, len(a.nodes))

	for _, n := range a.nodes {
		n := n
		go func() {
			body, err := a.fetch(ctx, n.BaseURL+"/api/v1/health")
			if err != nil {
				ch <- result{n.Name, NodeHealth{Node: n.Name, Status: "offline"}}
				return
			}
			var h NodeHealth
			if err := json.Unmarshal(body, &h); err != nil {
				ch <- result{n.Name, NodeHealth{Node: n.Name, Status: "offline"}}
				return
			}
			ch <- result{n.Name, h}
		}()
	}

	out := make(map[string]NodeHealth, len(a.nodes))
	for range a.nodes {
		r := <-ch
		out[r.name] = r.h
	}
	return out
}

// ---- Admin — state-changing operations ----

var validAdminNodes = map[string]bool{"n1": true, "n2": true, "n3": true}

const (
	etcdScrapeLockKey  = "/oilfield/locks/scrape"
	etcdIntervalKey    = "/oilfield/config/scrape_interval"
)

// Admin holds configuration for write-path operations.
// Nil Admin means admin is not configured; all admin routes return 503.
type Admin struct {
	token      string
	etcdURLs   []string // etcd HTTP gateway, e.g. http://n1.oilfield.parso.guru:2379
	domain     string
	sshKey     string
	http       *http.Client
	bounceFunc func(ctx context.Context, node, domain, sshKey string) error
}

func newAdmin() *Admin {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		return nil
	}
	domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")

	var etcdURLs []string
	if ep := os.Getenv("ETCD_ENDPOINTS"); ep != "" {
		for _, e := range strings.Split(ep, ",") {
			if e = strings.TrimSpace(e); e != "" {
				etcdURLs = append(etcdURLs, e)
			}
		}
	} else {
		etcdURLs = []string{
			"http://n1." + domain + ":2379",
			"http://n2." + domain + ":2379",
			"http://n3." + domain + ":2379",
		}
	}

	return &Admin{
		token:      token,
		etcdURLs:   etcdURLs,
		domain:     domain,
		sshKey:     envOr("DEPLOY_SSH_KEY", os.ExpandEnv("$HOME/.ssh/oilfield_ed25519")),
		http:       &http.Client{Timeout: 5 * time.Second},
		bounceFunc: sshBounce,
	}
}

// requireAdmin wraps a handler, requiring a valid Bearer token.
// Returns 503 if admin is not configured, 401 if token is wrong.
func requireAdmin(adm *Admin, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if adm == nil {
			writeJSON(w, http.StatusServiceUnavailable,
				map[string]string{"error": "admin not configured — set ADMIN_TOKEN env var"})
			return
		}
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if bearer != adm.token {
			w.Header().Set("WWW-Authenticate", `Bearer realm="oilfield-admin"`)
			writeJSON(w, http.StatusUnauthorized,
				map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// etcdPost sends an HTTP POST to the etcd v3 gRPC-HTTP gateway on the first
// responding etcd URL.
func (a *Admin) etcdPost(ctx context.Context, path string, body string) error {
	var lastErr error
	for _, base := range a.etcdURLs {
		req, err := http.NewRequestWithContext(ctx, "POST", base+path, strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("etcd HTTP %d from %s%s", resp.StatusCode, base, path)
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no etcd endpoint available")
}

func etcdKey(k string) string { return base64.StdEncoding.EncodeToString([]byte(k)) }
func etcdVal(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) }

func (a *Admin) etcdDelete(ctx context.Context, key string) error {
	b, _ := json.Marshal(map[string]string{"key": etcdKey(key)})
	return a.etcdPost(ctx, "/v3/kv/deleterange", string(b))
}

func (a *Admin) etcdPut(ctx context.Context, key, value string) error {
	b, _ := json.Marshal(map[string]string{"key": etcdKey(key), "value": etcdVal(value)})
	return a.etcdPost(ctx, "/v3/kv/put", string(b))
}

// sshBounce restarts oilfield services on a runtime node via SSH.
func sshBounce(ctx context.Context, node, domain, sshKey string) error {
	host := "deploy@" + node + "." + domain
	cmd := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		host,
		"sudo systemctl restart oilfield-api oilfield-scraper",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh %s: %w — %s", node, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- TTL cache (prevents hammering nodes on demo traffic) ----

type ttlCache struct {
	mu      sync.Mutex
	val     []byte
	err     error
	fetched time.Time
	ttl     time.Duration
}

func (c *ttlCache) get(fetch func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fetched.IsZero() || time.Since(c.fetched) >= c.ttl {
		c.val, c.err = fetch()
		c.fetched = time.Now()
	}
	return c.val, c.err
}

// ---- Node geo config ----

type NodeGeo struct {
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	City     string  `json:"city"`
	Country  string  `json:"country"`
	Provider string  `json:"provider"`
	Role     string  `json:"role"` // "runtime" | "ctrl"
}

var defaultNodeGeos = []NodeGeo{
	{Name: "n1", Role: "runtime", Lat: 49.45, Lon: 11.08, City: "Nuremberg", Country: "DE", Provider: "Hetzner"},
	{Name: "n2", Role: "runtime", Lat: 40.74, Lon: -74.18, City: "Newark", Country: "US", Provider: "Linode"},
	{Name: "n3", Role: "runtime", Lat: 48.86, Lon: 2.35, City: "Paris", Country: "FR", Provider: "Scaleway"},
	{Name: "n4", Role: "ctrl", Lat: 60.17, Lon: 24.94, City: "Helsinki", Country: "FI", Provider: "UpCloud"},
}

// NodeInfo merges NodeGeo with live health status for the control console.
type NodeInfo struct {
	NodeGeo
	Status      string `json:"status"`
	EtcdHealthy bool   `json:"etcd_healthy"`
}

func sshKeyPath() string {
	return envOr("DEPLOY_SSH_KEY", os.ExpandEnv("$HOME/.ssh/oilfield_ed25519"))
}

func loadNodeGeos() []NodeGeo {
	raw := os.Getenv("DAYLIGHT_NODE_GEO")
	if raw == "" {
		return defaultNodeGeos
	}
	var geos []NodeGeo
	if err := json.Unmarshal([]byte(raw), &geos); err != nil {
		log.Printf("DAYLIGHT_NODE_GEO parse error: %v — using defaults", err)
		return defaultNodeGeos
	}
	return geos
}

// ---- SSH pull helpers ----

func sshRun(ctx context.Context, host, sshKey, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		host,
		cmd,
	).Output()
	if err != nil {
		return "", fmt.Errorf("ssh %s: %w", host, err)
	}
	return strings.TrimSpace(string(out)), nil
}

type NodeMetrics struct {
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	MemTotalMB    int64   `json:"mem_total_mb"`
	MemUsedMB     int64   `json:"mem_used_mb"`
	MemFreeMB     int64   `json:"mem_free_mb"`
	MemUsedPct    float64 `json:"mem_used_pct"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

func parseMetrics(raw string) NodeMetrics {
	var m NodeMetrics
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch {
		case fields[0] == "Mem:":
			// free -m: "Mem: total used free ..."
			if len(fields) >= 4 {
				fmt.Sscanf(fields[1], "%d", &m.MemTotalMB)
				fmt.Sscanf(fields[2], "%d", &m.MemUsedMB)
				fmt.Sscanf(fields[3], "%d", &m.MemFreeMB)
				if m.MemTotalMB > 0 {
					m.MemUsedPct = float64(m.MemUsedMB) / float64(m.MemTotalMB) * 100
				}
			}
		case len(fields) == 1 && strings.Contains(fields[0], "."):
			// awk '{print $1}' /proc/uptime — single float like "86400.5"
			var sec float64
			if n, _ := fmt.Sscanf(fields[0], "%f", &sec); n == 1 && sec > 0 {
				m.UptimeSeconds = int64(sec)
			}
		case len(fields) >= 3 && fields[0] != "Swap:" && fields[0] != "total":
			// /proc/loadavg: "0.42 0.35 0.28 1/342 12345"
			// Exclude free -m header ("total used free ...") and Swap row.
			var l1, l5, l15 float64
			if n, _ := fmt.Sscanf(line, "%f %f %f", &l1, &l5, &l15); n == 3 {
				m.Load1, m.Load5, m.Load15 = l1, l5, l15
			}
		}
	}
	return m
}

// ---- per-node SSH caches ----

type nodeSSHCache struct {
	metrics  ttlCache
	logs     ttlCache
	services ttlCache
}

var (
	sshCaches   = map[string]*nodeSSHCache{}
	sshCachesMu sync.Mutex
)

func getSSHCache(name string, metricsTTL, logsTTL time.Duration) *nodeSSHCache {
	sshCachesMu.Lock()
	defer sshCachesMu.Unlock()
	if c, ok := sshCaches[name]; ok {
		return c
	}
	c := &nodeSSHCache{
		metrics:  ttlCache{ttl: metricsTTL},
		logs:     ttlCache{ttl: logsTTL},
		services: ttlCache{ttl: 30 * time.Second},
	}
	sshCaches[name] = c
	return c
}

// ---- etcd KV index (read-only, no values) ----

// EtcdKVEntry is key metadata returned by the etcd browser endpoint.
// Values are never included.
type EtcdKVEntry struct {
	Key     string `json:"key"`
	SizeB   int    `json:"size_b"`
	Version int64  `json:"version"`
}

// etcdRangePrefix lists all KV entries under prefix from the first responding
// etcd v3 HTTP gateway. Only key path and value size are returned; raw values
// are not exposed.
func etcdRangePrefix(ctx context.Context, etcdURLs []string, prefix string) ([]EtcdKVEntry, error) {
	key := base64.StdEncoding.EncodeToString([]byte(prefix))
	endBytes := []byte(prefix)
	endBytes[len(endBytes)-1]++
	rangeEnd := base64.StdEncoding.EncodeToString(endBytes)

	reqBody, _ := json.Marshal(map[string]any{
		"key":       key,
		"range_end": rangeEnd,
	})

	cl := &http.Client{Timeout: 8 * time.Second}
	var lastErr error
	for _, base := range etcdURLs {
		req, err := http.NewRequestWithContext(ctx, "POST", base+"/v3/kv/range",
			strings.NewReader(string(reqBody)))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := cl.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var raw struct {
			KVs []struct {
				Key     string `json:"key"`
				Value   string `json:"value"`
				Version string `json:"version"`
			} `json:"kvs"`
		}
		err = json.NewDecoder(resp.Body).Decode(&raw)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		entries := make([]EtcdKVEntry, 0, len(raw.KVs))
		for _, kv := range raw.KVs {
			keyBytes, _ := base64.StdEncoding.DecodeString(kv.Key)
			valBytes, _ := base64.StdEncoding.DecodeString(kv.Value)
			var ver int64
			fmt.Sscanf(kv.Version, "%d", &ver)
			entries = append(entries, EtcdKVEntry{
				Key:     string(keyBytes),
				SizeB:   len(valBytes),
				Version: ver,
			})
		}
		return entries, nil
	}
	return nil, lastErr
}

// ---- systemd service status (SSH pull) ----

// ServiceStatus describes one systemd unit.
type ServiceStatus struct {
	Unit        string `json:"unit"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
}

// parseServiceStatus parses `systemctl show` output: property=value lines
// separated by blank lines between units.
func parseServiceStatus(raw string) []ServiceStatus {
	var result []ServiceStatus
	cur := ServiceStatus{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if cur.Unit != "" {
				result = append(result, cur)
				cur = ServiceStatus{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "Id":
			cur.Unit = v
		case "LoadState":
			cur.LoadState = v
		case "ActiveState":
			cur.ActiveState = v
		case "SubState":
			cur.SubState = v
		case "Description":
			cur.Description = v
		}
	}
	if cur.Unit != "" {
		result = append(result, cur)
	}
	return result
}

// ---- DNS resolution ----

// DNSRecord is a resolved entry for one cluster hostname.
type DNSRecord struct {
	Hostname string   `json:"hostname"`
	Type     string   `json:"type"`
	Values   []string `json:"values"`
	Error    string   `json:"error,omitempty"`
}

// resolveClusterDNS concurrently resolves all cluster DNS names for domain.
func resolveClusterDNS(ctx context.Context, domain string) []DNSRecord {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "udp", "1.1.1.1:53")
		},
	}

	type query struct {
		hostname string
		rrType   string
	}
	queries := []query{
		{domain, "A"},
		{"n1." + domain, "A"},
		{"n2." + domain, "A"},
		{"n3." + domain, "A"},
		{"ctrl." + domain, "A"},
		{"etcd." + domain, "A"},
		{"api." + domain, "A"},
		{"dash." + domain, "CNAME"},
	}

	records := make([]DNSRecord, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		i, q := i, q
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := DNSRecord{Hostname: q.hostname, Type: q.rrType}
			if q.rrType == "CNAME" {
				cname, err := resolver.LookupCNAME(ctx, q.hostname)
				if err != nil {
					rec.Error = err.Error()
				} else {
					rec.Values = []string{strings.TrimSuffix(cname, ".")}
				}
			} else {
				addrs, err := resolver.LookupHost(ctx, q.hostname)
				if err != nil {
					rec.Error = err.Error()
				} else {
					rec.Values = addrs
				}
			}
			records[i] = rec
		}()
	}
	wg.Wait()
	return records
}

// ---- HTTP helpers ----

func cors(w http.ResponseWriter, r *http.Request) {
	allowed := strings.Split(envOr("DASH_ORIGIN", "https://oilfield-dash.parso.guru"), ",")
	reqOrigin := r.Header.Get("Origin")
	for _, o := range allowed {
		if strings.TrimSpace(o) == reqOrigin {
			w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
			break
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, PUT, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization")
	w.Header().Set("Vary", "Origin")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func proxyOrError(w http.ResponseWriter, body []byte, err error) {
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "no runtime node available"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// makeHandler builds the ServeMux for the dashboard backend.
// adm may be nil (admin routes return 503).
func makeHandler(agg *Aggregator, adm *Admin) http.Handler {
	mux := http.NewServeMux()

	// ---- Read-only routes ----

	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "oilfield-dash-web"})
	})

	mux.HandleFunc("GET /api/v1/health/all", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		writeJSON(w, http.StatusOK, agg.healthAll(ctx))
	})

	mux.HandleFunc("GET /api/v1/cluster", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/cluster")
		proxyOrError(w, body, err)
	})

	mux.HandleFunc("GET /api/v1/prices/all", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/prices/all")
		proxyOrError(w, body, err)
	})

	mux.HandleFunc("GET /api/v1/prices/{sector}", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/prices/"+r.PathValue("sector"))
		proxyOrError(w, body, err)
	})

	mux.HandleFunc("GET /api/v1/news", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/news")
		proxyOrError(w, body, err)
	})

	// ---- Admin routes (write-path, require Bearer token) ----

	// Force scrape: delete the distributed scrape lock so any node can claim it.
	mux.HandleFunc("DELETE /api/v1/admin/scrape-lock", requireAdmin(adm,
		func(w http.ResponseWriter, r *http.Request) {
			cors(w, r)
			ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
			defer cancel()
			if err := adm.etcdDelete(ctx, etcdScrapeLockKey); err != nil {
				writeJSON(w, http.StatusBadGateway,
					map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "lock cleared"})
		},
	))

	// Update scrape interval: body {"seconds": N}
	mux.HandleFunc("PUT /api/v1/admin/config/scrape-interval", requireAdmin(adm,
		func(w http.ResponseWriter, r *http.Request) {
			cors(w, r)
			var body struct {
				Seconds int `json:"seconds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Seconds < 60 {
				writeJSON(w, http.StatusBadRequest,
					map[string]string{"error": "body must be {\"seconds\": N} where N ≥ 60"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
			defer cancel()
			val := fmt.Sprintf("%d", body.Seconds)
			if err := adm.etcdPut(ctx, etcdIntervalKey, val); err != nil {
				writeJSON(w, http.StatusBadGateway,
					map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK,
				map[string]string{"status": "updated", "scrape_interval": val})
		},
	))

	// Bounce node: restart oilfield-api + oilfield-scraper on a runtime node via SSH.
	mux.HandleFunc("POST /api/v1/admin/nodes/{name}/bounce", requireAdmin(adm,
		func(w http.ResponseWriter, r *http.Request) {
			cors(w, r)
			name := r.PathValue("name")
			if !validAdminNodes[name] {
				writeJSON(w, http.StatusBadRequest,
					map[string]string{"error": "unknown node — valid: n1 n2 n3"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := adm.bounceFunc(ctx, name, adm.domain, adm.sshKey); err != nil {
				writeJSON(w, http.StatusBadGateway,
					map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK,
				map[string]string{"status": "restarted", "node": name})
		},
	))

	// ---- Daylight Control Console — read-only, SSH-backed, TTL-cached ----

	nodesCache := ttlCache{ttl: 15 * time.Second}

	// GET /api/v1/nodes — geo + health for every node (15 s cache)
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		data, err := nodesCache.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			geos := loadNodeGeos()
			health := agg.healthAll(ctx)
			infos := make([]NodeInfo, 0, len(geos))
			for _, g := range geos {
				ni := NodeInfo{NodeGeo: g}
				if g.Role == "ctrl" {
					ni.Status = "ctrl"
				} else {
					ni.Status = "offline"
				}
				if h, ok := health[g.Name]; ok {
					ni.Status = h.Status
					ni.EtcdHealthy = h.EtcdHealthy
				}
				infos = append(infos, ni)
			}
			return json.Marshal(infos)
		})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// GET /api/v1/nodes/{name}/metrics — SSH pull /proc/loadavg + free -m (30 s cache)
	mux.HandleFunc("GET /api/v1/nodes/{name}/metrics", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		name := r.PathValue("name")
		domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
		host := "deploy@" + name + "." + domain
		cache := getSSHCache(name, 30*time.Second, 60*time.Second)

		data, err := cache.metrics.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			raw, err := sshRun(ctx, host, sshKeyPath(),
				"cat /proc/loadavg; free -m; awk '{print $1}' /proc/uptime")
			if err != nil {
				return nil, err
			}
			return json.Marshal(parseMetrics(raw))
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// GET /api/v1/nodes/{name}/logs — SSH pull last 100 journalctl lines (60 s cache)
	mux.HandleFunc("GET /api/v1/nodes/{name}/logs", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		name := r.PathValue("name")
		domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
		host := "deploy@" + name + "." + domain
		cache := getSSHCache(name, 30*time.Second, 60*time.Second)

		data, err := cache.logs.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			raw, err := sshRun(ctx, host, sshKeyPath(),
				"sudo journalctl -u oilfield-api -u oilfield-scraper -n 100 --no-pager -o short-iso 2>&1 || true")
			if err != nil {
				return nil, err
			}
			lines := strings.Split(raw, "\n")
			for len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			return json.Marshal(map[string]any{"node": name, "lines": lines})
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// ---- Cluster-wide read-only views ----

	// GET /api/v1/cluster/etcd — etcd KV index for /oilfield/ prefix (30 s cache).
	// Returns key paths and sizes; raw values are never exposed.
	etcdKVCache := ttlCache{ttl: 30 * time.Second}
	mux.HandleFunc("GET /api/v1/cluster/etcd", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
		etcdURLs := []string{
			"http://n1." + domain + ":2379",
			"http://n2." + domain + ":2379",
			"http://n3." + domain + ":2379",
		}
		if ep := os.Getenv("ETCD_ENDPOINTS"); ep != "" {
			for _, e := range strings.Split(ep, ",") {
				if e = strings.TrimSpace(e); e != "" {
					etcdURLs = append(etcdURLs, e)
				}
			}
		}
		data, err := etcdKVCache.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entries, err := etcdRangePrefix(ctx, etcdURLs, "/oilfield/")
			if err != nil {
				return nil, err
			}
			return json.Marshal(entries)
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// GET /api/v1/nodes/{name}/services — systemd unit status via SSH (30 s cache).
	mux.HandleFunc("GET /api/v1/nodes/{name}/services", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		name := r.PathValue("name")
		domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
		host := "deploy@" + name + "." + domain
		cache := getSSHCache(name, 30*time.Second, 60*time.Second)

		data, err := cache.services.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			raw, err := sshRun(ctx, host, sshKeyPath(),
				"systemctl show oilfield-api.service oilfield-scraper.service oilfield-scraper.timer etcd.service nginx.service"+
					" --no-pager --property=Id,LoadState,ActiveState,SubState,Description 2>&1 || true")
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{
				"node":     name,
				"services": parseServiceStatus(raw),
			})
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// GET /api/v1/cluster/dns — live DNS resolution for all cluster hostnames (60 s cache).
	dnsCache := ttlCache{ttl: 60 * time.Second}
	mux.HandleFunc("GET /api/v1/cluster/dns", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		domain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
		data, err := dnsCache.get(func() ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return json.Marshal(resolveClusterDNS(ctx, domain))
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// CORS preflight.
	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func main() {
	nodes := defaultNodes()
	agg := newAggregator(nodes)
	adm := newAdmin()

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name + "=" + n.BaseURL
	}
	adminStatus := "disabled (set ADMIN_TOKEN to enable)"
	if adm != nil {
		adminStatus = "enabled"
	}
	log.Printf("oilfield-dash-web starting on %s — nodes: %s — admin: %s",
		listenAddr, strings.Join(names, ", "), adminStatus)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      makeHandler(agg, adm),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
