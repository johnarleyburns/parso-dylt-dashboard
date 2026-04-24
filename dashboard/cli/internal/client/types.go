package client

import "time"

// NodeHealth is the response from GET /api/v1/health on a single node.
type NodeHealth struct {
	Node        string    `json:"node"`
	Provider    string    `json:"provider"`
	Status      string    `json:"status"`       // "ok" | "degraded"
	EtcdHealthy bool      `json:"etcd_healthy"`
	LastScrape  string    `json:"last_scrape"`  // RFC3339 or ""
	Time        time.Time `json:"time"`
}

// PricePoint matches app/backend/internal/scraper/types.go.
type PricePoint struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Sector        string    `json:"sector"`
	Exchange      string    `json:"exchange"`
	Geography     string    `json:"geography"`
	DeliveryMonth string    `json:"delivery_month"`
	Price         float64   `json:"price"`
	Unit          string    `json:"unit"`
	ScrapedAt     time.Time `json:"scraped_at"`
	Source        string    `json:"source"`
}

// NewsItem matches app/backend/internal/scraper/types.go.
type NewsItem struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
	Source      string    `json:"source"`
	Summary     string    `json:"summary"`
	Tags        []string  `json:"tags"`
}

// NodeStatus is one entry in the cluster /nodes map.
type NodeStatus struct {
	Heartbeat string `json:"heartbeat"` // RFC3339
	Status    string `json:"status"`
	IP        string `json:"ip"`
	Provider  string `json:"provider"`
}

// ClusterStatus is the response from GET /api/v1/cluster.
type ClusterStatus struct {
	Nodes          map[string]*NodeStatus `json:"nodes"`
	Scrapelock     string                 `json:"scrape_lock"`
	ActiveNode     string                 `json:"active_node"`
	ScrapeInterval string                 `json:"scrape_interval"`
}

// NewsResponse is the response from GET /api/v1/news.
type NewsResponse struct {
	EIA []NewsItem `json:"eia"`
	IEA []NewsItem `json:"iea"`
}

// AllPrices is the response from GET /api/v1/prices/all.
type AllPrices map[string][]PricePoint
