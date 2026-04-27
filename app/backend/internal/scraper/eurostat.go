package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EurostatClient fetches EU household electricity prices from the Eurostat SDMX REST API.
// Free, no key. Dataset nrg_pc_204: Electricity prices for household consumers (bi-annual).
// Values are EUR/KWH (excluding taxes); we convert to EUR/MWh for consistency.
// API reference: https://wikis.ec.europa.eu/display/EUROSTATHELP/API+Statistics+-+data+query
type EurostatClient struct {
	http *http.Client
}

func NewEurostatClient() *EurostatClient {
	return &EurostatClient{http: &http.Client{Timeout: 20 * time.Second}}
}

// eurostatGeos are the country/aggregate codes queried in one batched request.
var eurostatGeos = []struct {
	code string
	name string
	geo  string
}{
	{"EU27_2020", "EU27 Electricity", "EUROPE"},
	{"DE", "Germany Electricity", "EUROPE"},
	{"FR", "France Electricity", "EUROPE"},
	{"ES", "Spain Electricity", "EUROPE"},
	{"IT", "Italy Electricity", "EUROPE"},
	{"PL", "Poland Electricity", "EUROPE"},
	{"NL", "Netherlands Electricity", "EUROPE"},
	{"SE", "Sweden Electricity", "EUROPE"},
}

// eurostatResponse is the minimal subset of the Eurostat JSON-SDMX 2.0 response.
type eurostatResponse struct {
	Value     map[string]float64 `json:"value"`
	Dimension struct {
		Geo struct {
			Category struct {
				Index map[string]int    `json:"index"`
				Label map[string]string `json:"label"`
			} `json:"category"`
		} `json:"geo"`
		Time struct {
			Category struct {
				Index map[string]int    `json:"index"`
				Label map[string]string `json:"label"`
			} `json:"category"`
		} `json:"time"`
	} `json:"dimension"`
	Size []int `json:"size"`
}

// ScrapeHouseholdElectricity returns the most recent bi-annual household electricity
// price (EUR/MWh, ex-taxes) for key EU member states plus the EU27 aggregate.
func (c *EurostatClient) ScrapeHouseholdElectricity(ctx context.Context) []PricePoint {
	// Build geo filter: EU27_2020+DE+FR+...
	var geoCodes []string
	for _, g := range eurostatGeos {
		geoCodes = append(geoCodes, g.code)
	}
	geoFilter := ""
	for i, code := range geoCodes {
		if i > 0 {
			geoFilter += "+"
		}
		geoFilter += code
	}

	url := "https://ec.europa.eu/eurostat/api/dissemination/sdmx/2.1/data/nrg_pc_204/" +
		"S.E7000.TOT_KWH.KWH.X_TAX.EUR." + geoFilter +
		".?format=JSON&lang=EN&lastTimePeriod=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}

	var r eurostatResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil
	}

	// Find the most recent time period label for DeliveryMonth.
	latestPeriod := "latest"
	for label := range r.Dimension.Time.Category.Label {
		latestPeriod = label
		break
	}

	// The number of time periods in this response (should be 1).
	nTime := len(r.Dimension.Time.Category.Index)
	if nTime == 0 {
		nTime = 1
	}

	now := time.Now().UTC()
	var pts []PricePoint

	for _, g := range eurostatGeos {
		pos, ok := r.Dimension.Geo.Category.Index[g.code]
		if !ok {
			continue
		}
		// Flat index: geo_pos * nTime + time_pos (time_pos=0 since lastTimePeriod=1)
		key := fmt.Sprintf("%d", pos*nTime)
		rawVal, exists := r.Value[key]
		if !exists || rawVal <= 0 {
			continue
		}
		// EUR/KWH → EUR/MWh
		priceMWh := rawVal * 1000

		pts = append(pts, PricePoint{
			Symbol:        "ESTAT_" + g.code,
			Name:          g.name + " (household, ex-tax)",
			Sector:        "electricity",
			Exchange:      "EUROSTAT",
			Geography:     g.geo,
			DeliveryMonth: latestPeriod,
			Price:         priceMWh,
			Unit:          "EUR/MWh",
			ScrapedAt:     now,
			Source:        "Eurostat/nrg_pc_204",
		})
	}
	return pts
}
