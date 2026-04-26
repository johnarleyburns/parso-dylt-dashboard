package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"oilfield/internal/scraper"
)

// Store is the etcd access surface required by the API handlers.
// etcdstore.Client satisfies this interface; tests can use a mock.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	GetJSON(ctx context.Context, key string, dest any) error
	GetWithPrefix(ctx context.Context, prefix string) (map[string]string, error)
	IsHealthy(ctx context.Context) bool
}

type Server struct {
	store    Store
	nodeName string
	provider string
}

func NewServer(store Store, nodeName, provider string) *Server {
	return &Server{store: store, nodeName: nodeName, provider: provider}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.withCORS(s.health))
	mux.HandleFunc("GET /api/v1/prices/all", s.withCORS(s.pricesAll))
	mux.HandleFunc("GET /api/v1/prices/{sector}", s.withCORS(s.pricesSector))
	mux.HandleFunc("GET /api/v1/news", s.withCORS(s.news))
	mux.HandleFunc("GET /api/v1/cluster", s.withCORS(s.cluster))
	// Preflight handler for cross-origin requests
	mux.HandleFunc("OPTIONS /api/v1/", s.preflight)
}

func (s *Server) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		h(w, r)
	}
}

func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

// allowedOrigins reads CORS_ORIGIN from the environment on each call so that
// tests can override it with t.Setenv without a package-level cache invalidation.
func allowedOrigins() map[string]bool {
	raw := os.Getenv("CORS_ORIGIN")
	if raw == "" {
		raw = "https://oilfield-dash.parso.guru,https://oilfield-dash.pages.dev"
	}
	m := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			m[o] = true
		}
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Origin")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// setCORS sets Access-Control-Allow-Origin if the request origin is in the allowlist.
func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	ao := allowedOrigins()
	if ao["*"] || ao[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	etcdOK := s.store.IsHealthy(ctx)
	lastScrape, _ := s.store.Get(ctx, "/oilfield/nodes/"+s.nodeName+"/heartbeat")

	status := http.StatusOK
	health := "ok"
	if !etcdOK {
		status = http.StatusServiceUnavailable
		health = "degraded"
	}

	writeJSON(w, status, map[string]any{
		"node":         s.nodeName,
		"provider":     s.provider,
		"status":       health,
		"etcd_healthy": etcdOK,
		"last_scrape":  lastScrape,
		"time":         time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) pricesAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sectors := []string{"crude", "natgas", "lng", "lpg", "ngls", "electricity", "refined"}
	result := make(map[string][]scraper.PricePoint, len(sectors))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sector := range sectors {
		sector := sector
		wg.Add(1)
		go func() {
			defer wg.Done()
			var pts []scraper.PricePoint
			if err := s.store.GetJSON(ctx, "/oilfield/prices/"+sector+"/latest", &pts); err != nil || pts == nil {
				pts = []scraper.PricePoint{}
			}
			mu.Lock()
			result[sector] = pts
			mu.Unlock()
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) pricesSector(w http.ResponseWriter, r *http.Request) {
	sector := r.PathValue("sector")
	// Basic allowlist — only known sectors
	allowed := map[string]bool{
		"crude": true, "natgas": true, "lng": true,
		"lpg": true, "ngls": true, "electricity": true, "refined": true,
	}
	if !allowed[sector] {
		http.Error(w, `{"error":"unknown sector"}`, http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var pts []scraper.PricePoint
	s.store.GetJSON(ctx, "/oilfield/prices/"+sector+"/latest", &pts)
	if pts == nil {
		pts = []scraper.PricePoint{}
	}
	writeJSON(w, http.StatusOK, pts)
}

func (s *Server) news(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Scan all /oilfield/news/*/items keys so new sources need no handler changes.
	raw, _ := s.store.GetWithPrefix(ctx, "/oilfield/news/")
	var all []scraper.NewsItem
	for _, v := range raw {
		var items []scraper.NewsItem
		if err := json.Unmarshal([]byte(v), &items); err == nil {
			all = append(all, items...)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].PublishedAt.After(all[j].PublishedAt)
	})
	const maxItems = 300
	if len(all) > maxItems {
		all = all[:maxItems]
	}
	if all == nil {
		all = []scraper.NewsItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": all})
}

type nodeStatus struct {
	Heartbeat string `json:"heartbeat"`
	Status    string `json:"status"`
	IP        string `json:"ip"`
	Provider  string `json:"provider"`
}

func (s *Server) cluster(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	nodeKeys, _ := s.store.GetWithPrefix(ctx, "/oilfield/nodes/")
	nodes := make(map[string]*nodeStatus)
	for key, val := range nodeKeys {
		// key format: /oilfield/nodes/{name}/{field}
		parts := strings.Split(strings.TrimPrefix(key, "/oilfield/nodes/"), "/")
		if len(parts) != 2 {
			continue
		}
		name, field := parts[0], parts[1]
		if nodes[name] == nil {
			nodes[name] = &nodeStatus{}
		}
		switch field {
		case "heartbeat":
			nodes[name].Heartbeat = val
		case "status":
			nodes[name].Status = val
		case "ip":
			nodes[name].IP = val
		case "provider":
			nodes[name].Provider = val
		}
	}

	activeNode, _ := s.store.Get(ctx, "/oilfield/config/active_node")
	scrapeInterval, _ := s.store.Get(ctx, "/oilfield/config/scrape_interval")
	if scrapeInterval == "" {
		scrapeInterval = "300"
	}
	lockHolder, _ := s.store.Get(ctx, "/oilfield/locks/scrape")

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":            nodes,
		"scrape_lock":      lockHolder,
		"active_node":      activeNode,
		"scrape_interval":  scrapeInterval,
	})
}
