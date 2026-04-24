package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"oilfield-dash/internal/client"
)

// TestMain silences cobra's error/usage output so test failures stay readable.
func TestMain(m *testing.M) {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	os.Exit(m.Run())
}

// captureStdout redirects os.Stdout for the duration of fn and returns everything written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// serveJSON returns a handler that writes v as JSON.
func serveJSON(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}

// ---- envOr ----

func TestEnvOr_UsesEnvVar(t *testing.T) {
	t.Setenv("OILFIELD_TEST_UNIQUE_KEY_XYZ", "fromenv")
	if got := envOr("OILFIELD_TEST_UNIQUE_KEY_XYZ", "fallback"); got != "fromenv" {
		t.Errorf("envOr: got %q, want fromenv", got)
	}
}

func TestEnvOr_UsesFallback(t *testing.T) {
	if got := envOr("OILFIELD_DEFINITELY_UNSET_9999", "fallback"); got != "fallback" {
		t.Errorf("envOr: got %q, want fallback", got)
	}
}

// ---- validSectors ----

func TestValidSectors_ContainsAllSeven(t *testing.T) {
	for _, s := range []string{"crude", "natgas", "lng", "lpg", "ngls", "electricity", "refined"} {
		if !validSectors[s] {
			t.Errorf("validSectors missing %q", s)
		}
	}
}

func TestValidSectors_RejectsUnknown(t *testing.T) {
	for _, bad := range []string{"petroleum", "gas", "", "oil", "CRUDE"} {
		if validSectors[bad] {
			t.Errorf("validSectors should not accept %q", bad)
		}
	}
}

// ---- prices command ----

func TestPricesCmd_RejectsUnknownSector(t *testing.T) {
	rootCmd.SetArgs([]string{"--api", "http://127.0.0.1:1", "--domain", "test.local", "prices", "petroleum"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown sector, got nil")
	}
	if !strings.Contains(err.Error(), "unknown sector") {
		t.Errorf("expected 'unknown sector' in error, got: %v", err)
	}
}

func TestPricesCmd_AllSectors_DisplaysData(t *testing.T) {
	payload := client.AllPrices{
		"crude": {{Symbol: "CL", Price: 82.45, Name: "WTI Crude"}},
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	rootCmd.SetArgs([]string{"--api", srv.URL, "--domain", "test.local", "prices"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("prices: %v", err)
		}
	})
	for _, want := range []string{"CL", "82.45"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in prices output, got:\n%s", want, out)
		}
	}
}

func TestPricesCmd_SingleSector_FiltersOutput(t *testing.T) {
	payload := client.AllPrices{
		"crude":  {{Symbol: "CL", Price: 82.45}},
		"natgas": {{Symbol: "NG", Price: 2.67}},
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	rootCmd.SetArgs([]string{"--api", srv.URL, "--domain", "test.local", "prices", "crude"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("prices crude: %v", err)
		}
	})
	if !strings.Contains(out, "CL") {
		t.Errorf("expected CL in crude-filtered output")
	}
	if strings.Contains(out, "NG") {
		t.Errorf("NG should not appear when filtering to crude sector, got:\n%s", out)
	}
}

// ---- news command ----

func TestNewsCmd_DefaultLimitFlag(t *testing.T) {
	fl := newsCmd.Flags().Lookup("limit")
	if fl == nil {
		t.Fatal("--limit flag not registered on news command")
	}
	if fl.DefValue != "20" {
		t.Errorf("--limit default: got %q, want 20", fl.DefValue)
	}
}

func TestNewsCmd_DisplaysItems(t *testing.T) {
	payload := client.NewsResponse{
		EIA: []client.NewsItem{
			{Title: "Crude rises on OPEC cut", Source: "EIA", PublishedAt: time.Now()},
		},
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	rootCmd.SetArgs([]string{"--api", srv.URL, "--domain", "test.local", "news", "--limit", "20"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("news: %v", err)
		}
	})
	if !strings.Contains(out, "Crude rises on OPEC cut") {
		t.Errorf("expected news title in output, got:\n%s", out)
	}
}

func TestNewsCmd_LimitFlag_ReducesOutput(t *testing.T) {
	var items []client.NewsItem
	for i := 0; i < 10; i++ {
		items = append(items, client.NewsItem{
			Title:       "Energy Item",
			Source:      "EIA",
			PublishedAt: time.Now(),
		})
	}
	srv := httptest.NewServer(serveJSON(client.NewsResponse{EIA: items}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"--api", srv.URL, "--domain", "test.local", "news", "--limit", "3"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("news --limit 3: %v", err)
		}
	})
	if count := strings.Count(out, "Energy Item"); count != 3 {
		t.Errorf("--limit 3 should yield 3 items, got %d occurrences", count)
	}
}

// ---- nodes command ----

func TestNodesCmd_ShowsNodeNames(t *testing.T) {
	payload := client.ClusterStatus{
		Nodes: map[string]*client.NodeStatus{
			"n1": {Provider: "hetzner", Status: "ok"},
			"n2": {Provider: "kamatera", Status: "ok"},
			"n3": {Provider: "scaleway", Status: "ok"},
		},
		ActiveNode:     "n1",
		Scrapelock:     "n1",
		ScrapeInterval: "300",
	}
	srv := httptest.NewServer(serveJSON(payload))
	defer srv.Close()

	// Per-node health checks use DNS (n1.test.local etc.) which won't resolve here.
	// HealthAll catches connection errors and marks nodes "offline" — command still renders.
	rootCmd.SetArgs([]string{"--api", srv.URL, "--domain", "test.local", "nodes"})
	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("nodes: %v", err)
		}
	})
	for _, name := range []string{"n1", "n2", "n3"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected node name %q in nodes output, got:\n%s", name, out)
		}
	}
}
