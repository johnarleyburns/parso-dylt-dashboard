package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"oilfield/internal/scraper"
)

// mockStore implements the Store interface for testing.
type mockStore struct {
	data    map[string]string
	healthy bool
}

func newMockStore(healthy bool) *mockStore {
	return &mockStore{data: make(map[string]string), healthy: healthy}
}

func (m *mockStore) setJSON(key string, v any) {
	b, _ := json.Marshal(v)
	m.data[key] = string(b)
}

func (m *mockStore) Get(_ context.Context, key string) (string, error) {
	return m.data[key], nil
}

func (m *mockStore) GetJSON(_ context.Context, key string, dest any) error {
	v, ok := m.data[key]
	if !ok || v == "" {
		return nil
	}
	return json.Unmarshal([]byte(v), dest)
}

func (m *mockStore) GetWithPrefix(_ context.Context, prefix string) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			result[k] = v
		}
	}
	return result, nil
}

func (m *mockStore) IsHealthy(_ context.Context) bool { return m.healthy }

// ---- test helpers ----

func newTestServer(store Store) *httptest.Server {
	mux := http.NewServeMux()
	srv := NewServer(store, "n1", "hetzner")
	srv.RegisterRoutes(mux)
	return httptest.NewServer(mux)
}

func getJSON(t *testing.T, url string, dest any) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("decode response from %s: %v", url, err)
	}
	return resp
}

// ---- /api/v1/health ----

func TestHealth_OK(t *testing.T) {
	store := newMockStore(true)
	store.data["/oilfield/nodes/n1/heartbeat"] = "2026-04-22T14:30:00Z"

	srv := newTestServer(store)
	defer srv.Close()

	var body map[string]any
	resp := getJSON(t, srv.URL+"/api/v1/health", &body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Errorf("status field: got %v, want ok", body["status"])
	}
	if body["etcd_healthy"] != true {
		t.Errorf("etcd_healthy: got %v, want true", body["etcd_healthy"])
	}
	if body["node"] != "n1" {
		t.Errorf("node: got %v, want n1", body["node"])
	}
}

func TestHealth_Degraded(t *testing.T) {
	store := newMockStore(false) // etcd unhealthy
	srv := newTestServer(store)
	defer srv.Close()

	var body map[string]any
	resp := getJSON(t, srv.URL+"/api/v1/health", &body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", resp.StatusCode)
	}
	if body["status"] != "degraded" {
		t.Errorf("status: got %v, want degraded", body["status"])
	}
}

// ---- /api/v1/prices/all ----

func TestPricesAll_ReturnsSectors(t *testing.T) {
	store := newMockStore(true)
	crude := []scraper.PricePoint{{Symbol: "CL", Name: "WTI", Sector: "crude", Price: 82.45, ScrapedAt: time.Now()}}
	store.setJSON("/oilfield/prices/crude/latest", crude)

	srv := newTestServer(store)
	defer srv.Close()

	var body map[string]json.RawMessage
	resp := getJSON(t, srv.URL+"/api/v1/prices/all", &body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	// All 7 sectors must be present (even empty ones)
	for _, sector := range []string{"crude", "natgas", "lng", "lpg", "ngls", "electricity", "refined"} {
		if _, ok := body[sector]; !ok {
			t.Errorf("expected sector %q in response", sector)
		}
	}
	// Crude should have one price point
	var crudePts []scraper.PricePoint
	json.Unmarshal(body["crude"], &crudePts)
	if len(crudePts) != 1 || crudePts[0].Symbol != "CL" {
		t.Errorf("crude: expected [{CL}], got %v", crudePts)
	}
}

func TestPricesAll_EmptySectorsReturnEmptyArrayNotNull(t *testing.T) {
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	var body map[string]json.RawMessage
	getJSON(t, srv.URL+"/api/v1/prices/all", &body)

	// Empty sectors must be [] not null (null would break frontend iteration)
	for sector, raw := range body {
		if string(raw) == "null" {
			t.Errorf("sector %q returned null, want []", sector)
		}
	}
}

// ---- /api/v1/prices/{sector} ----

func TestPricesSector_ValidSector(t *testing.T) {
	store := newMockStore(true)
	pts := []scraper.PricePoint{{Symbol: "NG", Price: 2.67, Sector: "natgas"}}
	store.setJSON("/oilfield/prices/natgas/latest", pts)

	srv := newTestServer(store)
	defer srv.Close()

	var result []scraper.PricePoint
	resp := getJSON(t, srv.URL+"/api/v1/prices/natgas", &result)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if len(result) != 1 || result[0].Symbol != "NG" {
		t.Errorf("expected [{NG}], got %v", result)
	}
}

func TestPricesSector_InvalidSector(t *testing.T) {
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/prices/bitcoin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown sector, got %d", resp.StatusCode)
	}
}

func TestPricesSector_EmptyReturnsArray(t *testing.T) {
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	var result []scraper.PricePoint
	getJSON(t, srv.URL+"/api/v1/prices/crude", &result)
	if result == nil {
		t.Error("expected empty array, got nil")
	}
}

// ---- /api/v1/news ----

func TestNews_ReturnsBothSources(t *testing.T) {
	store := newMockStore(true)
	store.setJSON("/oilfield/news/eia/items", []scraper.NewsItem{{Title: "EIA item", Source: "EIA"}})
	store.setJSON("/oilfield/news/iea/items", []scraper.NewsItem{{Title: "IEA item", Source: "IEA"}})

	srv := newTestServer(store)
	defer srv.Close()

	var body map[string][]scraper.NewsItem
	resp := getJSON(t, srv.URL+"/api/v1/news", &body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if len(body["eia"]) != 1 {
		t.Errorf("eia news: expected 1, got %d", len(body["eia"]))
	}
	if len(body["iea"]) != 1 {
		t.Errorf("iea news: expected 1, got %d", len(body["iea"]))
	}
}

func TestNews_EmptyReturnsArraysNotNull(t *testing.T) {
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	var body map[string]json.RawMessage
	getJSON(t, srv.URL+"/api/v1/news", &body)
	for _, src := range []string{"eia", "iea"} {
		if string(body[src]) == "null" {
			t.Errorf("news source %q returned null, want []", src)
		}
	}
}

// ---- /api/v1/cluster ----

func TestCluster_ReturnsNodeStatus(t *testing.T) {
	store := newMockStore(true)
	store.data["/oilfield/nodes/n1/heartbeat"] = "2026-04-22T14:30:00Z"
	store.data["/oilfield/nodes/n1/status"] = "ok"
	store.data["/oilfield/nodes/n1/provider"] = "hetzner"
	store.data["/oilfield/nodes/n2/status"] = "ok"
	store.data["/oilfield/nodes/n2/provider"] = "kamatera"
	store.data["/oilfield/config/active_node"] = "n2"
	store.data["/oilfield/config/scrape_interval"] = "300"
	store.data["/oilfield/locks/scrape"] = "n2"

	srv := newTestServer(store)
	defer srv.Close()

	var body map[string]any
	resp := getJSON(t, srv.URL+"/api/v1/cluster", &body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if body["active_node"] != "n2" {
		t.Errorf("active_node: got %v, want n2", body["active_node"])
	}
	if body["scrape_lock"] != "n2" {
		t.Errorf("scrape_lock: got %v, want n2", body["scrape_lock"])
	}

	nodes, ok := body["nodes"].(map[string]any)
	if !ok {
		t.Fatal("nodes field missing or wrong type")
	}
	if _, hasN1 := nodes["n1"]; !hasN1 {
		t.Error("expected n1 in nodes map")
	}
}

func TestCluster_ScrapeIntervalDefault(t *testing.T) {
	// When no scrape_interval is set, should default to "300"
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	var body map[string]any
	getJSON(t, srv.URL+"/api/v1/cluster", &body)
	if body["scrape_interval"] != "300" {
		t.Errorf("expected default scrape_interval 300, got %v", body["scrape_interval"])
	}
}

// ---- CORS ----

func TestCORSHeaderPresent(t *testing.T) {
	srv := newTestServer(newMockStore(true))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS header, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
