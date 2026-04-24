package scraper

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// InvestingScraper scrapes Investing.com for TTF and NBP natural gas prices.
// Investing.com renders prices via JavaScript; this scraper attempts to parse
// the initial server-rendered HTML. A headless browser (chromedp) may be needed
// if the prices are fully JS-rendered at runtime.
type InvestingScraper struct {
	http *http.Client
}

func NewInvestingScraper() *InvestingScraper {
	return &InvestingScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

// ScrapeTTF attempts to get TTF (Title Transfer Facility) Dutch gas price.
func (s *InvestingScraper) ScrapeTTF(ctx context.Context) (PricePoint, error) {
	return s.scrapeInvesting(ctx,
		"https://www.investing.com/commodities/dutch-ttf-gas-c1-futures",
		PricePoint{
			Symbol: "TTF", Name: "TTF Natural Gas (Netherlands)", Sector: "natgas",
			Exchange: "ICE", Geography: "EUROPE", Unit: "EUR/MWh",
		},
	)
}

// ScrapeNBP attempts to get NBP (National Balancing Point) UK gas price.
func (s *InvestingScraper) ScrapeNBP(ctx context.Context) (PricePoint, error) {
	return s.scrapeInvesting(ctx,
		"https://www.investing.com/commodities/natural-gas-uk",
		PricePoint{
			Symbol: "NBP", Name: "NBP Natural Gas (UK)", Sector: "natgas",
			Exchange: "ICE", Geography: "UK", Unit: "GBp/therm",
		},
	)
}

// ScrapePropaneEurope attempts to get European propane CIF ARA price.
func (s *InvestingScraper) ScrapePropaneEurope(ctx context.Context) (PricePoint, error) {
	return s.scrapeInvesting(ctx,
		"https://www.investing.com/commodities/propane",
		PricePoint{
			Symbol: "C3E", Name: "Propane Seaport Europe (CIF ARA)", Sector: "lpg",
			Exchange: "ICIS", Geography: "EUROPE", Unit: "USD/MT",
		},
	)
}

func (s *InvestingScraper) scrapeInvesting(ctx context.Context, url string, meta PricePoint) (PricePoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PricePoint{}, err
	}
	// Investing.com blocks generic bots; a plausible UA reduces (but does not eliminate) blocking.
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.http.Do(req)
	if err != nil {
		return PricePoint{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return PricePoint{}, fmt.Errorf("investing.com blocked request (status %d) — chromedp required for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return PricePoint{}, err
	}

	// Investing.com price is in a span with data-test="instrument-price-last"
	// or a div with class "text-2xl" in newer layouts.
	price, err := extractPrice(doc, []string{
		`[data-test="instrument-price-last"]`,
		`span.text-2xl`,
		`#last_last`,
		`span.arial_20`,
		`div.instrument-price_last__s6Ysv`,
	})
	if err != nil {
		return PricePoint{}, fmt.Errorf("investing.com price not found at %s: %w", url, err)
	}

	meta.Price = price
	meta.DeliveryMonth = frontMonthLabel()
	meta.ScrapedAt = time.Now().UTC()
	meta.Source = "investing_html"
	return meta, nil
}
