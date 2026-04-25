package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockEIAResponse builds a minimal EIA API v2 JSON response with one data point.
func mockEIAResponse(period string, value float64, units string) []byte {
	resp := map[string]any{
		"response": map[string]any{
			"total": 1,
			"data": []map[string]any{
				{"period": period, "value": value, "units": units},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestFetchLatest_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockEIAResponse("2026-04-22", 82.45, "Dollars per Barrel"))
	}))
	defer srv.Close()

	c := &EIAClient{apiKey: "test", http: srv.Client()}
	// Override the base URL by pointing the fetchLatest call to the test server.
	// Since fetchLatest builds its own URL, we use a round-tripper trick instead.
	c.http = newRedirectClient(srv.URL)

	price, period, unit, err := c.fetchLatest(t.Context(), "petroleum/pri/spt", seriesWTI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 82.45 {
		t.Errorf("price: got %v, want 82.45", price)
	}
	if period != "2026-04-22" {
		t.Errorf("period: got %q, want 2026-04-22", period)
	}
	if unit != "Dollars per Barrel" {
		t.Errorf("unit: got %q, want Dollars per Barrel", unit)
	}
}

func TestFetchLatest_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":{"total":0,"data":[]}}`))
	}))
	defer srv.Close()

	c := &EIAClient{apiKey: "test", http: newRedirectClient(srv.URL)}
	_, _, _, err := c.fetchLatest(t.Context(), "petroleum/pri/spt", "UNKNOWN_SERIES")
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

func TestFetchLatest_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := &EIAClient{apiKey: "test", http: newRedirectClient(srv.URL)}
	_, _, _, err := c.fetchLatest(t.Context(), "petroleum/pri/spt", "X")
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

func TestFetchLatest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &EIAClient{apiKey: "test", http: newRedirectClient(srv.URL)}
	_, _, _, err := c.fetchLatest(t.Context(), "petroleum/pri/spt", "X")
	// 500 returns non-JSON, so we expect a JSON decode error or no-data error
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestFetchAll_PartialSuccess(t *testing.T) {
	// Route by series ID in query string so goroutine execution order doesn't matter.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" && containsSeries(r.URL.RawQuery, seriesWTI) {
			w.Write(mockEIAResponse("2026-04-22", 82.45, "Dollars per Barrel"))
		} else {
			w.Write([]byte(`{"response":{"total":0,"data":[]}}`))
		}
	}))
	defer srv.Close()

	c := &EIAClient{apiKey: "test", http: newRedirectClient(srv.URL)}
	specs := []struct {
		series string
		meta   PricePoint
	}{
		{seriesWTI, PricePoint{Symbol: "CL", Name: "WTI", Sector: "crude", Unit: "USD/bbl"}},
		{seriesBrent, PricePoint{Symbol: "BZ", Name: "Brent", Sector: "crude", Unit: "USD/bbl"}},
	}
	pts := fetchAll(t.Context(), c, specs)
	if len(pts) != 1 {
		t.Fatalf("expected 1 successful price point, got %d", len(pts))
	}
	if pts[0].Symbol != "CL" {
		t.Errorf("expected CL, got %q", pts[0].Symbol)
	}
}

func containsSeries(query, series string) bool {
	return strings.Contains(query, series)
}

// newRedirectClient returns an *http.Client whose transport rewrites all
// request URLs to the given base URL, preserving path and query.
func newRedirectClient(baseURL string) *http.Client {
	return &http.Client{
		Transport: &redirectTransport{base: baseURL},
	}
}

type redirectTransport struct {
	base string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = req.URL.Host // keep for clarity
	// Replace scheme+host with test server
	req.URL.Scheme = "http"
	baseNoScheme := t.base
	if len(baseNoScheme) > 7 && baseNoScheme[:7] == "http://" {
		baseNoScheme = baseNoScheme[7:]
	}
	req.URL.Host = baseNoScheme
	return http.DefaultTransport.RoundTrip(req)
}
