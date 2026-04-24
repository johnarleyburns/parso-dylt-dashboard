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

// ICEScraper scrapes ICE (Intercontinental Exchange) for Brent, Gasoil, and LNG Europe.
// ICE's public settlement pages render prices in HTML tables; selectors are best-effort
// and may need adjustment if ICE changes their page layout.
type ICEScraper struct {
	http *http.Client
}

func NewICEScraper() *ICEScraper {
	return &ICEScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

// ScrapeBrent attempts to scrape ICE Brent Crude front-month settlement price.
func (s *ICEScraper) ScrapeBrent(ctx context.Context) (PricePoint, error) {
	// ICE publishes daily settlement data at a public endpoint.
	// URL targets the Brent Crude futures page — verify if layout changes.
	url := "https://www.theice.com/products/219/Brent-Crude-Futures/data?marketId=5615509&span=1"
	return s.scrapeSettlement(ctx, url, PricePoint{
		Symbol: "BZ", Name: "Brent Crude", Sector: "crude",
		Exchange: "ICE", Geography: "NORTH_SEA", Unit: "USD/bbl",
	})
}

// ScrapeGasoil attempts to scrape ICE Low Sulphur Gasoil front-month settlement.
func (s *ICEScraper) ScrapeGasoil(ctx context.Context) (PricePoint, error) {
	url := "https://www.theice.com/products/34361119/Low-Sulphur-Gasoil-Futures/data?span=1"
	return s.scrapeSettlement(ctx, url, PricePoint{
		Symbol: "GAS", Name: "ICE Low Sulphur Gasoil", Sector: "refined",
		Exchange: "ICE", Geography: "EUROPE", Unit: "USD/MT",
	})
}

// ScrapeLNGEurope attempts to scrape ICE Dutch TTF as LNG Europe proxy.
// ICE DES NWE LNG settlements are less publicly accessible; TTF is used as proxy.
func (s *ICEScraper) ScrapeLNGEurope(ctx context.Context) (PricePoint, error) {
	url := "https://www.theice.com/products/27996665/Dutch-TTF-Gas-Futures/data?span=1"
	return s.scrapeSettlement(ctx, url, PricePoint{
		Symbol: "LNGE", Name: "LNG Europe (DES NWE proxy via TTF)", Sector: "lng",
		Exchange: "ICE", Geography: "EUROPE", Unit: "USD/MWh",
	})
}

func (s *ICEScraper) scrapeSettlement(ctx context.Context, url string, meta PricePoint) (PricePoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PricePoint{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := s.http.Do(req)
	if err != nil {
		return PricePoint{}, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return PricePoint{}, err
	}

	// ICE settlement tables have a td with class "price" or similar.
	// Try multiple selectors in order of specificity.
	price, err := extractPrice(doc, []string{
		"td.price", ".settlement-price", "[data-field='settlement']",
		"table.data-table tbody tr:first-child td:nth-child(5)",
		"table tbody tr:first-child td:nth-child(4)",
	})
	if err != nil {
		return PricePoint{}, fmt.Errorf("ICE price not found at %s: %w", url, err)
	}

	meta.Price = price
	meta.DeliveryMonth = frontMonthLabel()
	meta.ScrapedAt = time.Now().UTC()
	meta.Source = "ice_html"
	return meta, nil
}

// extractPrice tries a list of CSS selectors and returns the first parseable float64.
func extractPrice(doc *goquery.Document, selectors []string) (float64, error) {
	for _, sel := range selectors {
		var found float64
		var parseErr error
		doc.Find(sel).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			text = strings.ReplaceAll(text, ",", "")
			v, err := strconv.ParseFloat(text, 64)
			if err == nil && v > 0 {
				found = v
				return false
			}
			parseErr = err
			return true
		})
		if found > 0 {
			return found, nil
		}
		_ = parseErr
	}
	return 0, fmt.Errorf("no price found with selectors %v", selectors)
}

// frontMonthLabel returns a "YYYY-MM-01" string for the nearest delivery month.
// Commodity front month is usually the next calendar month from today.
func frontMonthLabel() string {
	t := time.Now().UTC().AddDate(0, 1, 0)
	return fmt.Sprintf("%d-%02d-01", t.Year(), t.Month())
}
