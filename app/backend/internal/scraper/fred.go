package scraper

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FREDClient fetches commodity price series from the Federal Reserve Economic Data
// public CSV endpoint. No API key required — all series are freely downloadable.
// CSV URL: https://fred.stlouisfed.org/graph/fredgraph.csv?id=SERIES_ID
type FREDClient struct {
	http *http.Client
}

func NewFREDClient() *FREDClient {
	return &FREDClient{http: &http.Client{Timeout: 15 * time.Second}}
}

// scrapeSeries fetches a FRED CSV series and returns the most recent non-null value.
// FRED format: header row "DATE,SERIES_ID", then "YYYY-MM-DD,value" rows.
// Missing values are encoded as ".".
func (c *FREDClient) scrapeSeries(ctx context.Context, seriesID string) (float64, time.Time, error) {
	url := "https://fred.stlouisfed.org/graph/fredgraph.csv?id=" + seriesID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")
	req.Header.Set("Accept", "text/csv")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, time.Time{}, fmt.Errorf("FRED %s: HTTP %d", seriesID, resp.StatusCode)
	}

	var price float64
	var date time.Time
	header := true
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if header {
			header = false
			continue
		}
		parts := strings.SplitN(scanner.Text(), ",", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.TrimSpace(parts[1])
		if val == "." || val == "" {
			continue
		}
		p, err := strconv.ParseFloat(val, 64)
		if err != nil || p <= 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		price, date = p, d
	}
	if err := scanner.Err(); err != nil {
		return 0, time.Time{}, fmt.Errorf("FRED %s: scan error: %w", seriesID, err)
	}
	if price == 0 {
		return 0, time.Time{}, fmt.Errorf("FRED %s: no valid data rows", seriesID)
	}
	return price, date, nil
}

// ScrapeDubaiCrude returns the most recent Dubai Fateh crude spot price (USD/bbl).
// FRED series POILDUBUSDM — monthly, sourced from IMF Primary Commodity Prices.
func (c *FREDClient) ScrapeDubaiCrude(ctx context.Context) (PricePoint, error) {
	price, date, err := c.scrapeSeries(ctx, "POILDUBUSDM")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol:        "DUBAI",
		Name:          "Dubai Fateh Crude",
		Sector:        "crude",
		Exchange:      "DME",
		Geography:     "MIDDLE_EAST",
		DeliveryMonth: date.Format("2006-01-02"),
		Price:         price,
		Unit:          "USD/bbl",
		ScrapedAt:     time.Now().UTC(),
		Source:        "FRED/IMF",
	}, nil
}

// ScrapeNewcastleCoal returns the most recent Newcastle thermal coal price (USD/MT).
// FRED series PCOALAUUSDM — monthly, sourced from IMF Primary Commodity Prices.
func (c *FREDClient) ScrapeNewcastleCoal(ctx context.Context) (PricePoint, error) {
	price, date, err := c.scrapeSeries(ctx, "PCOALAUUSDM")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol:        "NEWC",
		Name:          "Newcastle Thermal Coal",
		Sector:        "coal",
		Exchange:      "ICE",
		Geography:     "ASIA_PACIFIC",
		DeliveryMonth: date.Format("2006-01-02"),
		Price:         price,
		Unit:          "USD/MT",
		ScrapedAt:     time.Now().UTC(),
		Source:        "FRED/IMF",
	}, nil
}

// ScrapeColombiaCoal returns the most recent Colombian coal export price (USD/MT).
// FRED series PCOALCOLBUSDM — monthly, IMF. Fills Latin America gap.
func (c *FREDClient) ScrapeColombiaCoal(ctx context.Context) (PricePoint, error) {
	price, date, err := c.scrapeSeries(ctx, "PCOALCOLBUSDM")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol:        "COL_COAL",
		Name:          "Colombian Coal Export",
		Sector:        "coal",
		Exchange:      "SPOT",
		Geography:     "LATAM",
		DeliveryMonth: date.Format("2006-01-02"),
		Price:         price,
		Unit:          "USD/MT",
		ScrapedAt:     time.Now().UTC(),
		Source:        "FRED/IMF",
	}, nil
}

// ScrapeNatGasEurope returns the most recent European natural gas price (USD/MMBtu).
// FRED series PNGASEUUSDM — monthly, IMF. Broader European benchmark than TTF alone.
func (c *FREDClient) ScrapeNatGasEurope(ctx context.Context) (PricePoint, error) {
	price, date, err := c.scrapeSeries(ctx, "PNGASEUUSDM")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol:        "EU_GAS",
		Name:          "Natural Gas Europe",
		Sector:        "natgas",
		Exchange:      "SPOT",
		Geography:     "EUROPE",
		DeliveryMonth: date.Format("2006-01-02"),
		Price:         price,
		Unit:          "USD/MMBtu",
		ScrapedAt:     time.Now().UTC(),
		Source:        "FRED/IMF",
	}, nil
}

// ScrapeNatGasJapan returns the most recent Japanese LNG import price (USD/MMBtu).
// FRED series PNGASJPUSDM — monthly, IMF. Key Asia LNG benchmark.
func (c *FREDClient) ScrapeNatGasJapan(ctx context.Context) (PricePoint, error) {
	price, date, err := c.scrapeSeries(ctx, "PNGASJPUSDM")
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{
		Symbol:        "JPN_LNG",
		Name:          "Japan LNG Import",
		Sector:        "lng",
		Exchange:      "SPOT",
		Geography:     "ASIA",
		DeliveryMonth: date.Format("2006-01-02"),
		Price:         price,
		Unit:          "USD/MMBtu",
		ScrapedAt:     time.Now().UTC(),
		Source:        "FRED/IMF",
	}, nil
}
