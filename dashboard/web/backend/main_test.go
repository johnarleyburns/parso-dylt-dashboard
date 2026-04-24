package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveJSON(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}

func readBody(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// ---- envOr ----

func TestEnvOr_UsesEnv(t *testing.T) {
	t.Setenv("DASH_TEST_KEY_UNIQUE", "fromenv")
	if got := envOr("DASH_TEST_KEY_UNIQUE", "fallback"); got != "fromenv" {
		t.Errorf("got %q, want fromenv", got)
	}
}

func TestEnvOr_UsesFallback(t *testing.T) {
	if got := envOr("DASH_DEFINITELY_UNSET_XYZ_9999", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

// ---- healthAll ----

func TestHealthAll_AggregatesAllNodes(t *testing.T) {
	s1 := httptest.NewServer(serveJSON(NodeHealth{Node: "n1", Status: "ok", Provider: "hetzner", EtcdHealthy: true}))
	defer s1.Close()
	s2 := httptest.NewServer(serveJSON(NodeHealth{Node: "n2", Status: "ok", Provider: "kamatera", EtcdHealthy: true}))
	defer s2.Close()

	agg := newAggregator([]RuntimeNode{
		{Name: "n1", BaseURL: s1.URL},
		{Name: "n2", BaseURL: s2.URL},
		{Name: "n3", BaseURL: "http://127.0.0.1:1"}, // unreachable
	})

	healths := agg.healthAll(t.Context())

	if healths["n1"].Status != "ok" {
		t.Errorf("n1: got %q, want ok", healths["n1"].Status)
	}
	if healths["n1"].Provider != "hetzner" {
		t.Errorf("n1 provider: got %q, want hetzner", healths["n1"].Provider)
	}
	if healths["n3"].Status != "offline" {
		t.Errorf("n3: got %q, want offline", healths["n3"].Status)
	}
}

func TestHealthAll_AllOfflineWhenUnreachable(t *testing.T) {
	agg := newAggregator([]RuntimeNode{
		{Name: "n1", BaseURL: "http://127.0.0.1:1"},
		{Name: "n2", BaseURL: "http://127.0.0.1:2"},
	})
	healths := agg.healthAll(t.Context())
	for _, name := range []string{"n1", "n2"} {
		if healths[name].Status != "offline" {
			t.Errorf("%s: got %q, want offline", name, healths[name].Status)
		}
	}
}

// ---- first ----

func TestFirst_ReturnsFirstResponder(t *testing.T) {
	payload := `{"crude":[{"symbol":"CL","price":82.45}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, payload)
	}))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{
		{Name: "n1", BaseURL: "http://127.0.0.1:1"}, // will fail first
		{Name: "n2", BaseURL: srv.URL},
	})
	body, name, err := agg.first(t.Context(), "/api/v1/prices/all")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if name != "n2" {
		t.Errorf("responding node: got %q, want n2", name)
	}
	if !strings.Contains(string(body), "CL") {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestFirst_ErrorWhenAllUnreachable(t *testing.T) {
	agg := newAggregator([]RuntimeNode{
		{Name: "n1", BaseURL: "http://127.0.0.1:1"},
		{Name: "n2", BaseURL: "http://127.0.0.1:2"},
	})
	_, _, err := agg.first(t.Context(), "/api/v1/cluster")
	if err == nil {
		t.Error("expected error when all nodes unreachable")
	}
}

func TestFirst_CancelledContextReturnsError(t *testing.T) {
	srv := httptest.NewServer(serveJSON(map[string]string{"ok": "yes"}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: srv.URL}})
	_, _, err := agg.first(ctx, "/api/v1/cluster")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

// ---- HTTP handlers ----

func TestHandler_SelfHealth(t *testing.T) {
	h := makeHandler(newAggregator(nil))
	r := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(readBody(t, w), "oilfield-dash-web") {
		t.Errorf("body missing service name: %s", w.Body.String())
	}
}

func TestHandler_CORS_Header(t *testing.T) {
	t.Setenv("DASH_ORIGIN", "https://dash.test.local")
	h := makeHandler(newAggregator(nil))
	r := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://dash.test.local" {
		t.Errorf("CORS origin: got %q, want https://dash.test.local", got)
	}
}

func TestHandler_Options_Preflight(t *testing.T) {
	h := makeHandler(newAggregator(nil))
	r := httptest.NewRequest("OPTIONS", "/api/v1/prices/all", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: got %d, want 204", w.Code)
	}
}

func TestHandler_PricesAll_Proxied(t *testing.T) {
	payload := map[string]any{
		"crude": []map[string]any{{"symbol": "CL", "price": 82.45}},
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: srv.URL}})
	h := makeHandler(agg)
	r := httptest.NewRequest("GET", "/api/v1/prices/all", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(readBody(t, w), "CL") {
		t.Errorf("body missing CL: %s", w.Body.String())
	}
}

func TestHandler_PricesSector_Proxied(t *testing.T) {
	payload := []map[string]any{{"symbol": "NG", "price": 2.67, "sector": "natgas"}}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: srv.URL}})
	h := makeHandler(agg)
	r := httptest.NewRequest("GET", "/api/v1/prices/natgas", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(readBody(t, w), "NG") {
		t.Errorf("body missing NG: %s", w.Body.String())
	}
}

func TestHandler_News_Proxied(t *testing.T) {
	payload := map[string]any{
		"eia": []map[string]any{{"title": "EIA LNG record", "source": "EIA"}},
		"iea": []map[string]any{},
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: srv.URL}})
	h := makeHandler(agg)
	r := httptest.NewRequest("GET", "/api/v1/news", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(readBody(t, w), "EIA LNG record") {
		t.Errorf("body missing news title: %s", w.Body.String())
	}
}

func TestHandler_Cluster_Proxied(t *testing.T) {
	payload := map[string]any{
		"nodes":      map[string]any{"n1": map[string]any{"provider": "hetzner"}},
		"active_node": "n1",
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: srv.URL}})
	h := makeHandler(agg)
	r := httptest.NewRequest("GET", "/api/v1/cluster", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(readBody(t, w), "hetzner") {
		t.Errorf("body missing hetzner: %s", w.Body.String())
	}
}

func TestHandler_503_WhenNoNodesAvailable(t *testing.T) {
	agg := newAggregator([]RuntimeNode{{Name: "n1", BaseURL: "http://127.0.0.1:1"}})
	h := makeHandler(agg)

	for _, path := range []string{"/api/v1/prices/all", "/api/v1/news", "/api/v1/cluster"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: got %d, want 503", path, w.Code)
		}
	}
}

func TestHandler_HealthAll_MarksUnreachableOffline(t *testing.T) {
	srv := httptest.NewServer(serveJSON(NodeHealth{Node: "n1", Status: "ok", Provider: "hetzner"}))
	defer srv.Close()

	agg := newAggregator([]RuntimeNode{
		{Name: "n1", BaseURL: srv.URL},
		{Name: "n2", BaseURL: "http://127.0.0.1:1"},
	})
	h := makeHandler(agg)
	r := httptest.NewRequest("GET", "/api/v1/health/all", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	body := readBody(t, w)
	if !strings.Contains(body, "offline") {
		t.Errorf("expected offline in health/all response: %s", body)
	}
	if !strings.Contains(body, "hetzner") {
		t.Errorf("expected hetzner in health/all response: %s", body)
	}
}
