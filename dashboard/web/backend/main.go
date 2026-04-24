package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// ---- HTTP helpers ----

func cors(w http.ResponseWriter, r *http.Request) {
	origin := envOr("DASH_ORIGIN", "https://dash.oilfield.parso.guru")
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
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

// makeHandler builds and returns the ServeMux for the dashboard backend.
func makeHandler(agg *Aggregator) http.Handler {
	mux := http.NewServeMux()

	// Dashboard backend self-health.
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "oilfield-dash-web"})
	})

	// Aggregated health from all three runtime nodes.
	mux.HandleFunc("GET /api/v1/health/all", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		writeJSON(w, http.StatusOK, agg.healthAll(ctx))
	})

	// Cluster state proxied from first responding runtime node.
	mux.HandleFunc("GET /api/v1/cluster", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/cluster")
		proxyOrError(w, body, err)
	})

	// All-sectors price data proxied from first responding runtime node.
	mux.HandleFunc("GET /api/v1/prices/all", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/prices/all")
		proxyOrError(w, body, err)
	})

	// Single-sector prices — exact "all" pattern above takes precedence.
	mux.HandleFunc("GET /api/v1/prices/{sector}", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/prices/"+r.PathValue("sector"))
		proxyOrError(w, body, err)
	})

	// News proxied from first responding runtime node.
	mux.HandleFunc("GET /api/v1/news", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		body, _, err := agg.first(ctx, "/api/v1/news")
		proxyOrError(w, body, err)
	})

	// CORS preflight for all routes.
	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func main() {
	nodes := defaultNodes()
	agg := newAggregator(nodes)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name + "=" + n.BaseURL
	}
	log.Printf("oilfield-dash-web starting on %s — nodes: %s", listenAddr, strings.Join(names, ", "))

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      makeHandler(agg),
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
