package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"oilfield-dash/internal/client"
)

// newTestClient builds a Client pointed at a test server, using the server's
// host as both the baseURL and the domain (per-node health calls reuse baseURL).
func newTestClient(srv *httptest.Server) *client.Client {
	return client.New(srv.URL, "test.local")
}

func serveJSON(t *testing.T, v any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}

// ---- Cluster ----

func TestCluster_ParsesResponse(t *testing.T) {
	payload := client.ClusterStatus{
		Nodes: map[string]*client.NodeStatus{
			"n1": {Status: "ok", Provider: "hetzner", Heartbeat: "2026-04-24T10:00:00Z"},
		},
		Scrapelock:     "n1",
		ActiveNode:     "n1",
		ScrapeInterval: "300",
	}
	srv := httptest.NewServer(serveJSON(t, payload))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.Cluster(t.Context())
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if got.ActiveNode != "n1" {
		t.Errorf("ActiveNode: got %q, want n1", got.ActiveNode)
	}
	if got.Nodes["n1"].Provider != "hetzner" {
		t.Errorf("n1 provider: got %q, want hetzner", got.Nodes["n1"].Provider)
	}
}

func TestCluster_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).Cluster(t.Context())
	if err == nil {
		t.Error("expected error on 500, got nil")
	}
}

// ---- PricesAll ----

func TestPricesAll_ParsesAllSectors(t *testing.T) {
	payload := client.AllPrices{
		"crude": {{Symbol: "CL", Price: 82.45, Name: "WTI Crude"}},
		"natgas": {{Symbol: "NG", Price: 2.67}},
	}
	srv := httptest.NewServer(serveJSON(t, payload))
	defer srv.Close()

	got, err := newTestClient(srv).PricesAll(t.Context())
	if err != nil {
		t.Fatalf("PricesAll: %v", err)
	}
	if len(got["crude"]) != 1 || got["crude"][0].Symbol != "CL" {
		t.Errorf("crude: got %v", got["crude"])
	}
	if len(got["natgas"]) != 1 || got["natgas"][0].Price != 2.67 {
		t.Errorf("natgas: got %v", got["natgas"])
	}
}

func TestPrices_Sector(t *testing.T) {
	payload := []client.PricePoint{{Symbol: "RB", Price: 2.45, Sector: "refined"}}
	srv := httptest.NewServer(serveJSON(t, payload))
	defer srv.Close()

	got, err := newTestClient(srv).Prices(t.Context(), "refined")
	if err != nil {
		t.Fatalf("Prices: %v", err)
	}
	if len(got) != 1 || got[0].Symbol != "RB" {
		t.Errorf("got %v", got)
	}
}

// ---- News ----

func TestNews_ParsesBothSources(t *testing.T) {
	payload := client.NewsResponse{
		EIA: []client.NewsItem{{Title: "EIA item", Source: "EIA", PublishedAt: time.Now()}},
		IEA: []client.NewsItem{{Title: "IEA item", Source: "IEA", PublishedAt: time.Now()}},
	}
	srv := httptest.NewServer(serveJSON(t, payload))
	defer srv.Close()

	got, err := newTestClient(srv).News(t.Context())
	if err != nil {
		t.Fatalf("News: %v", err)
	}
	if len(got.EIA) != 1 {
		t.Errorf("EIA: got %d items, want 1", len(got.EIA))
	}
	if len(got.IEA) != 1 {
		t.Errorf("IEA: got %d items, want 1", len(got.IEA))
	}
}

// ---- Health ----

func TestHealth_OK(t *testing.T) {
	payload := client.NodeHealth{
		Node: "n1", Provider: "hetzner", Status: "ok", EtcdHealthy: true,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, payload)(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Override the per-node URL to point at the test server instead of using DNS.
	c.NodeURLFunc = func(node, domain string) string { return srv.URL + "/api/v1/health" }

	got, err := c.Health(t.Context(), "n1")
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.Status != "ok" {
		t.Errorf("status: got %q, want ok", got.Status)
	}
	if got.Node != "n1" {
		t.Errorf("node: got %q, want n1", got.Node)
	}
}

func TestHealthAll_MarksUnreachableOffline(t *testing.T) {
	// Server that returns ok for /api/v1/health
	payload := client.NodeHealth{Node: "n1", Status: "ok"}
	srv := httptest.NewServer(serveJSON(t, payload))
	defer srv.Close()

	// Point to a port that won't respond for offline nodes
	c := client.New("http://127.0.0.1:1", "127.0.0.1:1")
	healths := c.HealthAll(t.Context())

	for _, name := range []string{"n1", "n2", "n3"} {
		h := healths[name]
		if h.Status != "offline" {
			t.Errorf("node %s: expected offline for unreachable server, got %q", name, h.Status)
		}
	}
}

// ---- HTTP 404 on unknown path ----

func TestGet_404ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).Cluster(t.Context())
	if err == nil {
		t.Error("expected error for 404, got nil")
	}
}
