package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AEMOClient fetches real-time electricity spot prices from the Australian Energy Market
// Operator (AEMO) National Electricity Market (NEM) summary API. Free, no key required.
// Prices are AUD/MWh, updated every 5 minutes. Five regions: NSW, QLD, SA, TAS, VIC.
// API: https://visualisations.aemo.com.au/aemo/apps/api/report/ELEC_NEM_SUMMARY
type AEMOClient struct {
	http *http.Client
}

func NewAEMOClient() *AEMOClient {
	return &AEMOClient{http: &http.Client{Timeout: 15 * time.Second}}
}

type aemoSummaryResponse struct {
	ElecNemSummary []struct {
		SettlementDate string  `json:"SETTLEMENTDATE"`
		RegionID       string  `json:"REGIONID"`
		Price          float64 `json:"PRICE"`
		PriceStatus    string  `json:"PRICE_STATUS"`
	} `json:"ELEC_NEM_SUMMARY"`
}

var aemoRegionName = map[string]string{
	"NSW1": "New South Wales",
	"QLD1": "Queensland",
	"SA1":  "South Australia",
	"TAS1": "Tasmania",
	"VIC1": "Victoria",
}

// ScrapeNEM returns the most recent spot price for each NEM region (AUD/MWh).
// Only FIRM prices are included; SUBJECT TO REVIEW prices are skipped.
func (c *AEMOClient) ScrapeNEM(ctx context.Context) []PricePoint {
	url := "https://visualisations.aemo.com.au/aemo/apps/api/report/ELEC_NEM_SUMMARY"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}

	var r aemoSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil
	}

	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	var pts []PricePoint

	for _, row := range r.ElecNemSummary {
		if row.Price <= 0 {
			continue
		}
		regionName, ok := aemoRegionName[row.RegionID]
		if !ok {
			continue
		}
		pts = append(pts, PricePoint{
			Symbol:        fmt.Sprintf("AEMO_%s", row.RegionID),
			Name:          regionName + " Electricity Spot",
			Sector:        "electricity",
			Exchange:      "AEMO",
			Geography:     "OCEANIA",
			DeliveryMonth: today,
			Price:         row.Price,
			Unit:          "AUD/MWh",
			ScrapedAt:     now,
			Source:        "AEMO/NEM",
		})
	}
	return pts
}
