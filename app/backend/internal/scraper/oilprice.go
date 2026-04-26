package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OilPriceAPIClient fetches global commodity prices from oilpriceapi.com.
// Free trial: https://oilpriceapi.com — sign up for OILPRICE_API_KEY.
// If OILPRICE_API_KEY is empty the scraper is disabled; all calls return an error.
//
// API reference: https://docs.oilpriceapi.com/
// Auth: Authorization: Token {key}
// Symbol codes (verify at https://oilpriceapi.com/commodities):
//   DUBAI_CRUDE_USD, BRENT_CRUDE_USD, WTI_USD, URALS_CRUDE_USD,
//   SINGAPORE_VLSFO_USD, JKM_LNG_USD, NEWC_COAL_USD, EUA_EUR
type OilPriceAPIClient struct {
	apiKey string
	http   *http.Client
}

func NewOilPriceAPIClient(apiKey string) *OilPriceAPIClient {
	return &OilPriceAPIClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// IsEnabled returns false when no API key is configured.
func (c *OilPriceAPIClient) IsEnabled() bool { return c.apiKey != "" }

type oilPriceResp struct {
	Status string `json:"status"`
	Data   struct {
		Price     float64 `json:"price"`
		Code      string  `json:"code"`
		CreatedAt string  `json:"created_at"`
		Type      string  `json:"type"`
		Currency  string  `json:"currency"`
	} `json:"data"`
	Error string `json:"error"`
}

func (c *OilPriceAPIClient) fetch(ctx context.Context, code string) (float64, time.Time, error) {
	if !c.IsEnabled() {
		return 0, time.Time{}, fmt.Errorf("OilPriceAPI: no API key configured (set OILPRICE_API_KEY)")
	}
	url := "https://api.oilpriceapi.com/v1/prices/latest?by_code=" + code
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	var r oilPriceResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, time.Time{}, fmt.Errorf("OilPriceAPI %s: decode: %w", code, err)
	}
	if r.Status != "success" {
		return 0, time.Time{}, fmt.Errorf("OilPriceAPI %s: %s", code, r.Error)
	}
	if r.Data.Price <= 0 {
		return 0, time.Time{}, fmt.Errorf("OilPriceAPI %s: zero price", code)
	}

	ts := time.Now().UTC()
	if r.Data.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.Data.CreatedAt); err == nil {
			ts = t
		}
	}
	return r.Data.Price, ts, nil
}

func (c *OilPriceAPIClient) ScrapeDubaiCrude(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "DUBAI_CRUDE_USD")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "DUBAI", Name: "Dubai Crude", Sector: "crude",
		Exchange: "DME", Geography: "MIDDLE_EAST",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "USD/bbl",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}

func (c *OilPriceAPIClient) ScrapeUrals(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "URALS_CRUDE_USD")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "URALS", Name: "Urals Crude", Sector: "crude",
		Exchange: "SPOT", Geography: "RUSSIA",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "USD/bbl",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}

func (c *OilPriceAPIClient) ScrapeSingaporeVLSFO(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "SINGAPORE_VLSFO_USD")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "SG_VLSFO", Name: "Singapore VLSFO Bunker", Sector: "refined",
		Exchange: "SGX", Geography: "ASIA",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "USD/MT",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}

func (c *OilPriceAPIClient) ScrapeJKM(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "JKM_LNG_USD")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "JKM", Name: "JKM LNG (Japan/Korea Marker)", Sector: "lng",
		Exchange: "PLATTS", Geography: "ASIA",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "USD/MMBtu",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}

func (c *OilPriceAPIClient) ScrapeNewcastleCoal(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "NEWC_COAL_USD")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "NEWC", Name: "Newcastle Thermal Coal", Sector: "coal",
		Exchange: "ICE", Geography: "ASIA_PACIFIC",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "USD/MT",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}

func (c *OilPriceAPIClient) ScrapeEUCarbon(ctx context.Context) (PricePoint, error) {
	price, ts, err := c.fetch(ctx, "EUA_EUR")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol: "EUA", Name: "EU Carbon Allowance (ETS)", Sector: "carbon",
		Exchange: "EEX", Geography: "EUROPE",
		DeliveryMonth: ts.Format("2006-01-02"),
		Price: price, Unit: "EUR/tCO2",
		ScrapedAt: time.Now().UTC(), Source: "OilPriceAPI",
	}, nil
}
