package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oilfield-dash/internal/client"
)

// statusFixtureServer is a minimal multi-route test server for status tests.
type statusFixtureServer struct {
	srv *httptest.Server
}

func (s *statusFixtureServer) url() string { return s.srv.URL }
func (s *statusFixtureServer) close()      { s.srv.Close() }

// makeStatusFixture builds a test server with the three status endpoints pre-populated.
func makeStatusFixture(
	cluster client.ClusterStatus,
	prices client.AllPrices,
	news client.NewsResponse,
) *statusFixtureServer {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/cluster", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cluster)
	})
	mux.HandleFunc("/api/v1/prices/all", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, prices)
	})
	mux.HandleFunc("/api/v1/news", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, news)
	})
	// Per-node health: return ok for any path not matched above.
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, client.NodeHealth{Status: "ok"})
	})
	return &statusFixtureServer{srv: httptest.NewServer(mux)}
}

// ---- status command tests ----

func TestStatusCmd_ShowsNodeNames(t *testing.T) {
	cluster := client.ClusterStatus{
		Nodes: map[string]*client.NodeStatus{
			"n1": {Provider: "hetzner", Status: "ok", Heartbeat: time.Now().UTC().Format(time.RFC3339)},
			"n2": {Provider: "linode", Status: "ok"},
			"n3": {Provider: "ionos", Status: "ok"},
		},
		ActiveNode:     "n1",
		Scrapelock:     "n1",
		ScrapeInterval: "300",
	}
	fix := makeStatusFixture(cluster, client.AllPrices{}, client.NewsResponse{})
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	for _, name := range []string{"n1", "n2", "n3"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected node name %q in status output, got:\n%s", name, out)
		}
	}
}

func TestStatusCmd_ShowsTimestamp(t *testing.T) {
	fix := makeStatusFixture(
		client.ClusterStatus{Nodes: map[string]*client.NodeStatus{"n1": {}, "n2": {}, "n3": {}}},
		client.AllPrices{},
		client.NewsResponse{},
	)
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	if !strings.Contains(out, "UTC") {
		t.Errorf("expected UTC timestamp in status output, got:\n%s", out)
	}
}

func TestStatusCmd_ShowsPricesWhenAvailable(t *testing.T) {
	cluster := client.ClusterStatus{
		Nodes: map[string]*client.NodeStatus{
			"n1": {Provider: "hetzner", Status: "ok"},
			"n2": {Provider: "linode", Status: "ok"},
			"n3": {Provider: "ionos", Status: "ok"},
		},
	}
	prices := client.AllPrices{
		"crude": {
			{Symbol: "CL", Name: "WTI Crude Oil", Price: 82.45, Unit: "USD/bbl", Exchange: "NYMEX", Sector: "crude"},
		},
		"coal": {
			{Symbol: "MTF", Name: "Newcastle Coal", Price: 135.50, Unit: "USD/t", Exchange: "ICE", Sector: "coal"},
		},
		"carbon": {
			{Symbol: "CFI2Z4", Name: "EUA Carbon", Price: 64.20, Unit: "EUR/t", Exchange: "ICE", Sector: "carbon"},
		},
	}
	fix := makeStatusFixture(cluster, prices, client.NewsResponse{})
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	for _, want := range []string{"CL", "82.45", "MTF", "135.50", "CFI2Z4", "64.20"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in status prices output, got:\n%s", want, out)
		}
	}
}

func TestStatusCmd_ShowsNoDataMessage_WhenPricesEmpty(t *testing.T) {
	fix := makeStatusFixture(
		client.ClusterStatus{Nodes: map[string]*client.NodeStatus{"n1": {}, "n2": {}, "n3": {}}},
		client.AllPrices{},
		client.NewsResponse{},
	)
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	if !strings.Contains(out, "No price data") {
		t.Errorf("expected no-data message when prices empty, got:\n%s", out)
	}
}

func TestStatusCmd_ShowsNewsItems(t *testing.T) {
	news := client.NewsResponse{
		EIA: []client.NewsItem{
			{Title: "Crude rises on OPEC cut", Source: "EIA", PublishedAt: time.Now()},
		},
	}
	fix := makeStatusFixture(
		client.ClusterStatus{Nodes: map[string]*client.NodeStatus{"n1": {}, "n2": {}, "n3": {}}},
		client.AllPrices{},
		news,
	)
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	if !strings.Contains(out, "Crude rises on OPEC cut") {
		t.Errorf("expected news title in status output, got:\n%s", out)
	}
}

func TestStatusCmd_HandlesAPIError_Gracefully(t *testing.T) {
	// Point at a port that won't respond — fetchAll prints warnings but status
	// command itself must not return an error (it degrades gracefully).
	rootCmd.SetArgs([]string{"--api", "http://127.0.0.1:1", "--domain", "test.local", "status"})
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("status should not return error on unreachable API (prints warning): %v", err)
	}
}

func TestStatusCmd_CoalAndCarbonSectorsAppear(t *testing.T) {
	prices := client.AllPrices{
		"coal":   {{Symbol: "COAL", Name: "Newcastle Coal Futures", Price: 140.0, Unit: "USD/t", Exchange: "ICE", Sector: "coal"}},
		"carbon": {{Symbol: "EUA", Name: "EU Allowance", Price: 65.0, Unit: "EUR/t", Exchange: "ICE", Sector: "carbon"}},
	}
	fix := makeStatusFixture(
		client.ClusterStatus{Nodes: map[string]*client.NodeStatus{"n1": {}, "n2": {}, "n3": {}}},
		prices,
		client.NewsResponse{},
	)
	defer fix.close()

	rootCmd.SetArgs([]string{"--api", fix.url(), "--domain", "test.local", "status"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("status: %v", err)
		}
	})
	for _, want := range []string{"coal", "carbon", "COAL", "EUA"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in status output, got:\n%s", want, out)
		}
	}
}
