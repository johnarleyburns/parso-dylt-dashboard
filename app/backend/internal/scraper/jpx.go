package scraper

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// JPXScraper scrapes Japan Exchange Group for TOCOM crude oil futures.
type JPXScraper struct {
	http *http.Client
}

func NewJPXScraper() *JPXScraper {
	return &JPXScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

// ScrapeTOCOM attempts to get TOCOM Crude Oil front-month settlement (JPY/kl).
func (s *JPXScraper) ScrapeTOCOM(ctx context.Context) (PricePoint, error) {
	url := "https://www.jpx.co.jp/english/markets/derivatives/settlement/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PricePoint{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")
	req.Header.Set("Accept", "text/html")

	resp, err := s.http.Do(req)
	if err != nil {
		return PricePoint{}, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return PricePoint{}, err
	}

	// JPX settlement table: find a row with "Crude Oil" in any cell,
	// then take the first parseable numeric value beyond the label column.
	var price float64
	var found bool
	doc.Find("table tbody tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		if !containsAny(row.Text(), []string{"Crude Oil", "原油", "TOCOM Crude"}) {
			return true // continue
		}
		row.Find("td").EachWithBreak(func(i int, td *goquery.Selection) bool {
			if i < 2 { // skip label columns
				return true
			}
			if p := parseNumeric(td.Text()); p > 0 {
				price = p
				found = true
				return false
			}
			return true
		})
		return !found
	})

	if !found {
		return PricePoint{}, fmt.Errorf("TOCOM crude price not found on JPX settlement page")
	}

	return PricePoint{
		Symbol: "TO", Name: "TOCOM Crude Oil", Sector: "crude",
		Exchange: "JPX", Geography: "ASIA_PAC",
		DeliveryMonth: frontMonthLabel(),
		Price:         price,
		Unit:          "JPY/kl",
		ScrapedAt:     time.Now().UTC(),
		Source:        "jpx_html",
	}, nil
}

// parseNumeric strips commas and whitespace, then parses a float64.
func parseNumeric(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
