package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---- trimSummary ----

func TestTrimSummary_Plain(t *testing.T) {
	got := trimSummary("hello world", 300)
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestTrimSummary_StripHTML(t *testing.T) {
	got := trimSummary("<p>EIA reports <strong>record</strong> LNG exports.</p>", 300)
	want := "EIA reports record LNG exports."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTrimSummary_Truncate(t *testing.T) {
	long := "Natural gas prices surged to a 12-month high following a prolonged cold snap across the US Midwest, driving Henry Hub spot prices above $4/MMBtu for the first time since January. Analysts expect prices to remain elevated through the heating season."
	got := trimSummary(long, 50)
	if len(got) > 50 {
		t.Errorf("expected len <= 50, got %d: %q", len(got), got)
	}
}

func TestTrimSummary_Empty(t *testing.T) {
	if got := trimSummary("", 300); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---- inferTags ----

func TestInferTags_Crude(t *testing.T) {
	tags := inferTags("WTI crude oil futures rose 2% on supply concerns")
	assertContains(t, tags, "crude")
}

func TestInferTags_MultipleSectors(t *testing.T) {
	tags := inferTags("LNG exports and propane shipments reached record levels")
	assertContains(t, tags, "lng")
	assertContains(t, tags, "lpg")
	assertContains(t, tags, "exports")
}

func TestInferTags_Electricity(t *testing.T) {
	tags := inferTags("ERCOT electricity demand hits 85 GW during heat dome")
	assertContains(t, tags, "electricity")
}

func TestInferTags_NoMatch(t *testing.T) {
	tags := inferTags("the quick brown fox")
	if len(tags) != 0 {
		t.Errorf("expected no tags, got %v", tags)
	}
}

// ---- MergeNews ----

func TestMergeNews_PrependsFresh(t *testing.T) {
	existing := []NewsItem{{URL: "https://eia.gov/1", Title: "Old"}}
	fresh := []NewsItem{{URL: "https://eia.gov/2", Title: "New"}}

	merged := MergeNews(fresh, existing)
	if len(merged) != 2 {
		t.Fatalf("expected 2 items, got %d", len(merged))
	}
	if merged[0].URL != "https://eia.gov/2" {
		t.Errorf("fresh item should be first, got %q", merged[0].URL)
	}
}

func TestMergeNews_DeduplicatesByURL(t *testing.T) {
	existing := []NewsItem{{URL: "https://eia.gov/1"}, {URL: "https://eia.gov/2"}}
	fresh := []NewsItem{{URL: "https://eia.gov/1"}, {URL: "https://eia.gov/3"}}

	merged := MergeNews(fresh, existing)
	// should have 3 unique: /1 (from existing), /2 (from existing), /3 (fresh non-dup)
	if len(merged) != 3 {
		t.Errorf("expected 3 unique items, got %d", len(merged))
	}
}

func TestMergeNews_TrimsToMax(t *testing.T) {
	existing := make([]NewsItem, 148)
	for i := range existing {
		existing[i] = NewsItem{URL: "https://eia.gov/old/" + string(rune('a'+i%26))}
	}
	fresh := []NewsItem{
		{URL: "https://eia.gov/new/1"},
		{URL: "https://eia.gov/new/2"},
		{URL: "https://eia.gov/new/3"},
		{URL: "https://eia.gov/new/4"},
	}
	merged := MergeNews(fresh, existing)
	if len(merged) != maxNewsItems {
		t.Errorf("expected %d items (maxNewsItems), got %d", maxNewsItems, len(merged))
	}
}

func TestMergeNews_EmptyExisting(t *testing.T) {
	fresh := []NewsItem{{URL: "https://eia.gov/1", Title: "First"}}
	merged := MergeNews(fresh, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 item, got %d", len(merged))
	}
}

// ---- ScrapeNewsRSS with mock server ----

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>EIA Today in Energy</title>
    <link>https://www.eia.gov/todayinenergy/</link>
    <item>
      <title>U.S. LNG Exports Set Monthly Record</title>
      <link>https://www.eia.gov/todayinenergy/detail.php?id=12345</link>
      <description>U.S. LNG exports reached a record 14.7 Bcf/d in March 2026.</description>
      <pubDate>Thu, 15 Apr 2026 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>WTI Crude Prices Rise on OPEC+ Cut</title>
      <link>https://www.eia.gov/todayinenergy/detail.php?id=12346</link>
      <description>West Texas Intermediate crude oil prices rose after OPEC+ announced production cuts.</description>
      <pubDate>Fri, 16 Apr 2026 00:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestScrapeNewsRSS_ParsesFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(sampleRSS))
	}))
	defer srv.Close()

	items, err := ScrapeNewsRSS(t.Context(), srv.URL, "EIA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Source != "EIA" {
		t.Errorf("expected source EIA, got %q", items[0].Source)
	}
	if items[0].Title == "" {
		t.Error("expected non-empty title")
	}
	if items[0].PublishedAt.IsZero() {
		t.Error("expected non-zero published_at")
	}
}

func TestScrapeNewsRSS_TagInference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(sampleRSS))
	}))
	defer srv.Close()

	items, _ := ScrapeNewsRSS(t.Context(), srv.URL, "EIA")
	// First item mentions "LNG exports" — should tag "lng" and "exports"
	assertContains(t, items[0].Tags, "lng")
	assertContains(t, items[0].Tags, "exports")
}

func TestScrapeNewsRSS_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := ScrapeNewsRSS(t.Context(), srv.URL, "EIA")
	if err == nil {
		t.Error("expected error on non-RSS response, got nil")
	}
}

func TestScrapeNewsRSS_FallbackPublishedAt(t *testing.T) {
	noDateRSS := `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
	  <item><title>No Date</title><link>https://example.com/1</link></item>
	</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(noDateRSS))
	}))
	defer srv.Close()

	before := time.Now().Add(-time.Second)
	items, err := ScrapeNewsRSS(t.Context(), srv.URL, "X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one item")
	}
	if !items[0].PublishedAt.After(before) {
		t.Errorf("expected published_at to fall back to now, got %v", items[0].PublishedAt)
	}
}

// ---- helpers ----

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, v := range slice {
		if v == want {
			return
		}
	}
	t.Errorf("expected slice %v to contain %q", slice, want)
}
