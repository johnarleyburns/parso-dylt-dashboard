package render_test

import (
	"strings"
	"testing"
	"time"

	"oilfield-dash/internal/client"
	"oilfield-dash/internal/render"
)

// ---- Nodes ----

func makeCluster(lockHolder string, nodes map[string]*client.NodeStatus) client.ClusterStatus {
	return client.ClusterStatus{
		Nodes:          nodes,
		Scrapelock:     lockHolder,
		ActiveNode:     lockHolder,
		ScrapeInterval: "300",
	}
}

func makeHealths(statuses map[string]string) map[string]client.NodeHealth {
	out := make(map[string]client.NodeHealth)
	for name, status := range statuses {
		out[name] = client.NodeHealth{Node: name, Status: status}
	}
	return out
}

func TestNodes_ContainsNodeNames(t *testing.T) {
	cluster := makeCluster("n1", map[string]*client.NodeStatus{
		"n1": {Provider: "hetzner", Status: "ok", Heartbeat: time.Now().UTC().Format(time.RFC3339)},
		"n2": {Provider: "kamatera", Status: "ok"},
		"n3": {Provider: "scaleway", Status: "ok"},
	})
	healths := makeHealths(map[string]string{"n1": "ok", "n2": "ok", "n3": "ok"})

	out := render.Nodes(cluster, healths)
	for _, name := range []string{"n1", "n2", "n3"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected output to contain %q, got:\n%s", name, out)
		}
	}
}

func TestNodes_ShowsScrapeLockHolder(t *testing.T) {
	cluster := makeCluster("n2", map[string]*client.NodeStatus{
		"n1": {Provider: "hetzner"}, "n2": {Provider: "kamatera"}, "n3": {Provider: "scaleway"},
	})
	out := render.Nodes(cluster, makeHealths(map[string]string{"n1": "ok", "n2": "ok", "n3": "ok"}))

	// n2 row should contain HELD
	lines := strings.Split(out, "\n")
	var n2Line string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "n2") {
			n2Line = l
		}
	}
	if !strings.Contains(n2Line, "HELD") {
		t.Errorf("n2 line should contain HELD (it holds the lock), got: %q", n2Line)
	}
}

func TestNodes_OfflineNode(t *testing.T) {
	cluster := makeCluster("", map[string]*client.NodeStatus{
		"n1": {Provider: "hetzner"}, "n2": {Provider: "kamatera"}, "n3": {Provider: "scaleway"},
	})
	healths := makeHealths(map[string]string{"n1": "ok", "n2": "offline", "n3": "ok"})
	out := render.Nodes(cluster, healths)

	if !strings.Contains(out, "OFFLINE") {
		t.Errorf("expected OFFLINE in output for n2, got:\n%s", out)
	}
}

// ---- Prices ----

func TestPrices_ShowsSectorAndSymbol(t *testing.T) {
	prices := client.AllPrices{
		"crude": {
			{Symbol: "CL", Name: "WTI Crude Oil", Sector: "crude", Price: 82.45, Unit: "USD/bbl", Exchange: "NYMEX"},
		},
		"natgas": {
			{Symbol: "NG", Name: "Henry Hub", Sector: "natgas", Price: 2.67, Unit: "USD/MMBtu", Exchange: "NYMEX"},
		},
	}
	out := render.Prices(prices, nil)
	for _, want := range []string{"CL", "NG", "82.45", "2.67", "NYMEX"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPrices_FilterBySector(t *testing.T) {
	prices := client.AllPrices{
		"crude":  {{Symbol: "CL", Price: 82.45}},
		"natgas": {{Symbol: "NG", Price: 2.67}},
	}
	out := render.Prices(prices, []string{"crude"})

	if !strings.Contains(out, "CL") {
		t.Errorf("expected CL in filtered output")
	}
	if strings.Contains(out, "NG") {
		t.Errorf("NG should not appear when filtered to crude")
	}
}

func TestPrices_EmptyShowsNoDataMessage(t *testing.T) {
	out := render.Prices(client.AllPrices{}, nil)
	if !strings.Contains(out, "No price data") {
		t.Errorf("expected no-data message for empty prices, got: %q", out)
	}
}

// ---- News ----

func TestNews_ShowsTitlesAndSource(t *testing.T) {
	news := client.NewsResponse{
		EIA: []client.NewsItem{
			{Title: "EIA: LNG record", Source: "EIA", PublishedAt: time.Now(), URL: "https://eia.gov/1"},
		},
		IEA: []client.NewsItem{
			{Title: "IEA: demand outlook", Source: "IEA", PublishedAt: time.Now().Add(-time.Hour), URL: "https://iea.org/1"},
		},
	}
	out := render.News(news, 10)
	for _, want := range []string{"EIA: LNG record", "IEA: demand outlook", "[EIA]", "[IEA]"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in news output, got:\n%s", want, out)
		}
	}
}

func TestNews_SortedNewestFirst(t *testing.T) {
	old := client.NewsItem{Title: "Old news", PublishedAt: time.Now().Add(-24 * time.Hour), Source: "EIA"}
	fresh := client.NewsItem{Title: "Fresh news", PublishedAt: time.Now(), Source: "EIA"}
	news := client.NewsResponse{EIA: []client.NewsItem{old, fresh}}

	out := render.News(news, 10)
	oldIdx := strings.Index(out, "Old news")
	freshIdx := strings.Index(out, "Fresh news")
	if freshIdx > oldIdx {
		t.Errorf("expected fresh news before old news (freshIdx=%d, oldIdx=%d)", freshIdx, oldIdx)
	}
}

func TestNews_RespectsLimit(t *testing.T) {
	var items []client.NewsItem
	for i := 0; i < 10; i++ {
		items = append(items, client.NewsItem{
			Title:       "Item",
			PublishedAt: time.Now(),
			Source:      "EIA",
		})
	}
	news := client.NewsResponse{EIA: items}
	out := render.News(news, 3)

	// 3 items × "Item" title; count occurrences
	count := strings.Count(out, "Item")
	if count != 3 {
		t.Errorf("limit=3 should yield 3 items, found %d occurrences of 'Item'", count)
	}
}

func TestNews_EmptyShowsMessage(t *testing.T) {
	out := render.News(client.NewsResponse{}, 10)
	if !strings.Contains(out, "No news") {
		t.Errorf("expected no-news message for empty response, got: %q", out)
	}
}

// ---- Status ----

func TestStatus_ContainsTimestamp(t *testing.T) {
	cluster := makeCluster("", map[string]*client.NodeStatus{
		"n1": {Provider: "hetzner"}, "n2": {Provider: "kamatera"}, "n3": {Provider: "scaleway"},
	})
	healths := makeHealths(map[string]string{"n1": "ok", "n2": "ok", "n3": "ok"})
	out := render.Status(cluster, healths)

	if !strings.Contains(out, "UTC") {
		t.Errorf("expected UTC timestamp in status header, got:\n%s", out)
	}
	if !strings.Contains(out, "OILFIELD CLUSTER STATUS") {
		t.Errorf("expected title in status output, got:\n%s", out)
	}
}
