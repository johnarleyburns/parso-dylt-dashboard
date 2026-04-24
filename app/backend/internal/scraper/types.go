package scraper

import "time"

// PricePoint matches the schema in docs/etcd-schema.md (human-authored, do not change).
type PricePoint struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Sector        string    `json:"sector"`
	Exchange      string    `json:"exchange"`
	Geography     string    `json:"geography"`
	DeliveryMonth string    `json:"delivery_month"` // "YYYY-MM-01" or "spot"
	Price         float64   `json:"price"`
	Unit          string    `json:"unit"`
	ScrapedAt     time.Time `json:"scraped_at"`
	Source        string    `json:"source"`
}

// NewsItem matches the schema in docs/etcd-schema.md.
type NewsItem struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
	Source      string    `json:"source"`
	Summary     string    `json:"summary"`
	Tags        []string  `json:"tags"`
}
