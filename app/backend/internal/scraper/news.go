package scraper

import (
	"context"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

const (
	eiaRSSURL = "https://www.eia.gov/rss/todayinenergy.xml"
	oilpriceRSSURL = "https://oilprice.com/rss/main"

	maxNewsItems = 150
)

// ScrapeNewsRSS fetches RSS from the given URL and returns NewsItems.
// source is the label written to NewsItem.Source ("EIA" or "IEA").
func ScrapeNewsRSS(ctx context.Context, feedURL, source string) ([]NewsItem, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	items := make([]NewsItem, 0, len(feed.Items))
	for _, item := range feed.Items {
		pub := now
		if item.PublishedParsed != nil {
			pub = item.PublishedParsed.UTC()
		}

		summary := item.Description
		if summary == "" {
			summary = item.Content
		}
		summary = trimSummary(summary, 300)

		items = append(items, NewsItem{
			Title:       item.Title,
			URL:         item.Link,
			PublishedAt: pub,
			Source:      source,
			Summary:     summary,
			Tags:        inferTags(item.Title + " " + summary),
		})
	}
	return items, nil
}

// MergeNews prepends fresh items onto existing, deduplicates by URL, trims to max.
func MergeNews(fresh, existing []NewsItem) []NewsItem {
	seen := make(map[string]bool, len(existing))
	for _, item := range existing {
		seen[item.URL] = true
	}
	var merged []NewsItem
	for _, item := range fresh {
		if !seen[item.URL] {
			merged = append(merged, item)
			seen[item.URL] = true
		}
	}
	merged = append(merged, existing...)
	if len(merged) > maxNewsItems {
		merged = merged[:maxNewsItems]
	}
	return merged
}

// trimSummary strips HTML tags and truncates to maxLen characters.
func trimSummary(s string, maxLen int) string {
	// strip simple HTML tags
	inTag := false
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(b.String())
	if len(clean) > maxLen {
		clean = clean[:maxLen]
	}
	return clean
}

// inferTags returns energy-sector tags based on keywords in the text.
func inferTags(text string) []string {
	text = strings.ToLower(text)
	tagMap := []struct {
		tag      string
		keywords []string
	}{
		{"crude", []string{"crude", "wti", "brent", "oil price"}},
		{"natgas", []string{"natural gas", "henry hub", "ttf", "nbp", "lng", "natgas"}},
		{"lng", []string{"lng", "liquefied natural gas", "jkm", "cargoes"}},
		{"lpg", []string{"lpg", "propane", "butane"}},
		{"ngls", []string{"ngl", "ethane", "natural gas liquid"}},
		{"electricity", []string{"electricity", "power", "mwh", "grid", "renewable"}},
		{"refined", []string{"gasoline", "diesel", "jet fuel", "heating oil", "rbob", "ulsd"}},
		{"exports", []string{"export", "shipment", "terminal"}},
		{"production", []string{"production", "output", "rig count", "drilling"}},
		{"storage", []string{"inventory", "storage", "stockpile"}},
	}
	var tags []string
	for _, t := range tagMap {
		for _, kw := range t.keywords {
			if strings.Contains(text, kw) {
				tags = append(tags, t.tag)
				break
			}
		}
	}
	return tags
}
