package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	body := fmt.Sprintf(`{"key":%q}`, etcdKey(key))
	return a.etcdPost(ctx, "/v3/kv/deleterange", body)
}

func (a *Admin) etcdPut(ctx context.Context, key, value string) error {
	body := fmt.Sprintf(`{"key":%q,"value":%q}`, etcdKey(key), etcdVal(value))
	return a.etcdPost(ctx, "/v3/kv/put", body)
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

// ---- HTTP helpers ----

func cors(w http.ResponseWriter, r *http.Request) {
	allowed := strings.Split(envOr("DASH_ORIGIN", "https://dash.oilfield.parso.guru"), ",")
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
