package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CMEScraper fetches WTI Midland settlement from CME Group.
// CME has a public JSON endpoint for daily settlements that is more reliable
// than HTML scraping their React-rendered settlement pages.
type CMEScraper struct {
	http *http.Client
}

func NewCMEScraper() *CMEScraper {
	return &CMEScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

type cmeSettlement struct {
	Settlements []struct {
		ProductID    string  `json:"productId"`
		ProductName  string  `json:"productName"`
		SettlePrice  float64 `json:"settle"`
		ContractYear int     `json:"contractYear"`
		ContractMonth int    `json:"contractMonth"`
	} `json:"settlements"`
}

// ScrapeWTIMidland fetches WTI Midland (WTM) front-month settlement from CME.
// CME WTI Midland: product code 428 (CMA — WTI Midland Crude). Verify at
// https://www.cmegroup.com/trading/energy/crude-oil/wti-midland-american-crude-oil.html
func (s *CMEScraper) ScrapeWTIMidland(ctx context.Context) (PricePoint, error) {
	// CME's settlement JSON API — product code for WTI Midland
	url := "https://www.cmegroup.com/CmeWS/mvc/Settlements/futures/settlements/428/FUT"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PricePoint{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return PricePoint{}, err
	}
	defer resp.Body.Close()

	var data cmeSettlement
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return PricePoint{}, fmt.Errorf("CME JSON decode: %w", err)
	}
	if len(data.Settlements) == 0 {
		return PricePoint{}, fmt.Errorf("CME: no settlements in response")
	}

	// First entry is front month
	front := data.Settlements[0]
	if front.SettlePrice <= 0 {
		return PricePoint{}, fmt.Errorf("CME: zero/negative settle price")
	}

	deliveryMonth := fmt.Sprintf("%d-%02d-01", front.ContractYear, front.ContractMonth)
	return PricePoint{
		Symbol: "WTM", Name: "WTI Midland Crude", Sector: "crude",
		Exchange: "NYMEX", Geography: "US_PERMIAN",
		DeliveryMonth: deliveryMonth,
		Price:         front.SettlePrice,
		Unit:          "USD/bbl",
		ScrapedAt:     time.Now().UTC(),
		Source:        "cme_api",
	}, nil
}
