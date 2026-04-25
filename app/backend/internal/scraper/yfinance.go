package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// YahooFinanceScraper fetches commodity futures prices from Yahoo Finance's
// internal chart JSON API. This API returns structured JSON (no CSS selectors)
// and is far more stable than HTML scraping of exchange settlement pages.
//
// Ticker format: CL=F (WTI), BZ=F (Brent), NG=F (Natural Gas), etc.
// Falls back from query1 to query2 host on failure.
type YahooFinanceScraper struct {
	http *http.Client
}

func NewYahooFinanceScraper() *YahooFinanceScraper {
	return &YahooFinanceScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

type yfResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// fetchTicker returns the regular market price for a Yahoo Finance ticker.
// Tries query1 then query2 as fallback.
func (s *YahooFinanceScraper) fetchTicker(ctx context.Context, ticker string) (price float64, currency string, err error) {
	hosts := []string{"query1.finance.yahoo.com", "query2.finance.yahoo.com"}
	for _, host := range hosts {
		url := fmt.Sprintf("https://%s/v8/finance/chart/%s?interval=1d&range=1d", host, ticker)
		price, currency, err = s.doFetch(ctx, url)
		if err == nil && price > 0 {
			return price, currency, nil
		}
	}
	return 0, "", fmt.Errorf("yahoo finance: no price for %s: %w", ticker, err)
}

func (s *YahooFinanceScraper) doFetch(ctx context.Context, url string) (float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://finance.yahoo.com/")

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var r yfResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, "", fmt.Errorf("json decode: %w", err)
	}
	if r.Chart.Error != nil {
		return 0, "", fmt.Errorf("yahoo api error: %s — %s", r.Chart.Error.Code, r.Chart.Error.Description)
	}
	if len(r.Chart.Result) == 0 || r.Chart.Result[0].Meta.RegularMarketPrice == 0 {
		return 0, "", fmt.Errorf("no price in response")
	}
	m := r.Chart.Result[0].Meta
	return m.RegularMarketPrice, m.Currency, nil
}

func (s *YahooFinanceScraper) scrape(ctx context.Context, ticker string, meta PricePoint) (PricePoint, error) {
	price, _, err := s.fetchTicker(ctx, ticker)
	if err != nil {
		return PricePoint{}, err
	}
	meta.Price = price
	meta.DeliveryMonth = frontMonthLabel()
	meta.ScrapedAt = time.Now().UTC()
	meta.Source = "yfinance"
	return meta, nil
}

// Futures tickers — front-month contracts.
func (s *YahooFinanceScraper) ScrapeWTI(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "CL=F", PricePoint{
		Symbol: "CL", Name: "WTI Crude Oil (front month)", Sector: "crude",
		Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/bbl",
	})
}

func (s *YahooFinanceScraper) ScrapeBrent(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "BZ=F", PricePoint{
		Symbol: "BZ", Name: "Brent Crude (front month)", Sector: "crude",
		Exchange: "ICE", Geography: "NORTH_SEA", Unit: "USD/bbl",
	})
}

func (s *YahooFinanceScraper) ScrapeNatGas(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "NG=F", PricePoint{
		Symbol: "NG", Name: "Henry Hub Natural Gas (front month)", Sector: "natgas",
		Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/MMBtu",
	})
}

func (s *YahooFinanceScraper) ScrapeHeatingOil(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "HO=F", PricePoint{
		Symbol: "HO", Name: "Heating Oil / ULSD (front month)", Sector: "refined",
		Exchange: "NYMEX", Geography: "US_NORTHEAST", Unit: "USD/gal",
	})
}

func (s *YahooFinanceScraper) ScrapeRBOB(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "RB=F", PricePoint{
		Symbol: "RB", Name: "RBOB Gasoline (front month)", Sector: "refined",
		Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/gal",
	})
}

func (s *YahooFinanceScraper) ScrapeTTF(ctx context.Context) (PricePoint, error) {
	return s.scrape(ctx, "TTF=F", PricePoint{
		Symbol: "TTF", Name: "TTF Natural Gas (Netherlands, front month)", Sector: "natgas",
		Exchange: "ICE", Geography: "EUROPE", Unit: "EUR/MWh",
	})
}
