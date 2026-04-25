package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// EIA API v2 series IDs — verified against live API 2026-04-25.
// Route petroleum/pri/spt: petroleum spot prices (daily)
// Route natural-gas/pri/fut: natural gas futures/spot prices (daily)
// Route electricity/rto/daily-region-data: RTO day-ahead prices
const (
	// Crude — petroleum/pri/spt
	seriesWTI   = "RWTC"  // WTI Crude spot, Cushing OK, $/bbl
	seriesBrent = "RBRTE" // Europe Brent Crude spot FOB, $/bbl

	// Natural gas — natural-gas/pri/fut (Henry Hub daily spot)
	seriesHenryHub = "RNGWHHD" // Henry Hub natural gas spot, $/MMBtu

	// LPG — petroleum/pri/spt
	seriesPropaneMB = "EER_EPLLPA_PF4_Y44MB_DPG" // Propane Mont Belvieu, $/gal

	// Refined products — petroleum/pri/spt
	seriesRBOB    = "EER_EPMRR_PF4_Y05LA_DPG" // LA Reformulated RBOB Gasoline, $/gal
	seriesULSD    = "EER_EPD2F_PF4_Y35NY_DPG"  // NY Harbor Heating Oil / ULSD, $/gal
	seriesJetFuel = "EER_EPJK_PF4_RGC_DPG"    // US Gulf Coast Jet Fuel, $/gal
)

// eiaRouteMap maps each series ID to its EIA v2 API route.
var eiaRouteMap = map[string]string{
	seriesWTI:       "petroleum/pri/spt",
	seriesBrent:     "petroleum/pri/spt",
	seriesHenryHub:  "natural-gas/pri/fut",
	seriesPropaneMB: "petroleum/pri/spt",
	seriesRBOB:      "petroleum/pri/spt",
	seriesULSD:      "petroleum/pri/spt",
	seriesJetFuel:   "petroleum/pri/spt",
}

type EIAClient struct {
	apiKey string
	http   *http.Client
}

func NewEIAClient(apiKey string) *EIAClient {
	return &EIAClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

type eiaResp struct {
	Response struct {
		Data []struct {
			Period string `json:"period"`
			// EIA API v2 returns value as a JSON string ("91.38"); test mocks use a number.
			// interface{} handles both.
			Value interface{} `json:"value"`
			Units string      `json:"units"`
		} `json:"data"`
	} `json:"response"`
}

// parseEIAValue converts the mixed-type value field (string or float64) to float64.
func parseEIAValue(v interface{}) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		return strconv.ParseFloat(x, 64)
	}
	return 0, fmt.Errorf("unexpected value type %T: %v", v, v)
}

// fetchHistory returns the last `count` monthly price points for a series, oldest first.
func (e *EIAClient) fetchHistory(ctx context.Context, route, seriesID string, count int) ([]PricePoint, error) {
	url := fmt.Sprintf(
		"https://api.eia.gov/v2/%s/data/?api_key=%s&frequency=monthly&data[0]=value&facets[series][]=%s&sort[0][column]=period&sort[0][direction]=desc&length=%d",
		route, e.apiKey, seriesID, count,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r eiaResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if len(r.Response.Data) == 0 {
		return nil, fmt.Errorf("no data for series %s", seriesID)
	}
	now := time.Now().UTC()
	// Reverse so points are oldest-first (natural time order for charting).
	data := r.Response.Data
	var pts []PricePoint
	for i := len(data) - 1; i >= 0; i-- {
		d := data[i]
		v, err := parseEIAValue(d.Value)
		if err != nil || v <= 0 {
			continue
		}
		pts = append(pts, PricePoint{
			Price:         v,
			DeliveryMonth: d.Period + "-01", // "YYYY-MM" → "YYYY-MM-01" for date parsing
			ScrapedAt:     now,
			Source:        "eia_api",
		})
	}
	if len(pts) == 0 {
		return nil, fmt.Errorf("no valid price data for series %s", seriesID)
	}
	return pts, nil
}

// fetchLatest returns the single most-recent data point for a series (price, period, unit).
// Used for sources that don't support history (e.g. EIA STEO for JKM LNG).
func (e *EIAClient) fetchLatest(ctx context.Context, route, seriesID string) (price float64, period, unit string, err error) {
	url := fmt.Sprintf(
		"https://api.eia.gov/v2/%s/data/?api_key=%s&frequency=monthly&data[0]=value&facets[series][]=%s&sort[0][column]=period&sort[0][direction]=desc&length=1",
		route, e.apiKey, seriesID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()

	var r eiaResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, "", "", err
	}
	if len(r.Response.Data) == 0 {
		return 0, "", "", fmt.Errorf("no data for series %s", seriesID)
	}
	d := r.Response.Data[0]
	v, err := parseEIAValue(d.Value)
	if err != nil {
		return 0, "", "", fmt.Errorf("parse value %v: %w", d.Value, err)
	}
	return v, d.Period, d.Units, nil
}

// fetchHistoryPoints fetches monthly history for a series and fills in metadata from the template.
func (e *EIAClient) fetchHistoryPoints(ctx context.Context, seriesID string, meta PricePoint, count int) ([]PricePoint, error) {
	route, ok := eiaRouteMap[seriesID]
	if !ok {
		return nil, fmt.Errorf("no route configured for series %s", seriesID)
	}
	pts, err := e.fetchHistory(ctx, route, seriesID, count)
	if err != nil {
		return nil, err
	}
	for i := range pts {
		pts[i].Symbol = meta.Symbol
		pts[i].Name = meta.Name
		pts[i].Sector = meta.Sector
		pts[i].Exchange = meta.Exchange
		pts[i].Geography = meta.Geography
		pts[i].Unit = meta.Unit
	}
	return pts, nil
}

// fetchPoint kept for single-point use (LNG STEO, electricity RTO).
func (e *EIAClient) fetchPoint(ctx context.Context, seriesID string, meta PricePoint) (PricePoint, error) {
	pts, err := e.fetchHistoryPoints(ctx, seriesID, meta, 1)
	if err != nil {
		return PricePoint{}, err
	}
	return pts[0], nil
}

// ScrapeCrude returns EIA-sourced crude oil price points.
func (e *EIAClient) ScrapeCrude(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesWTI, PricePoint{Symbol: "CL", Name: "WTI Crude Oil", Sector: "crude", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/bbl"}},
		{seriesBrent, PricePoint{Symbol: "BZ", Name: "Europe Brent Crude", Sector: "crude", Exchange: "ICE", Geography: "NORTH_SEA", Unit: "USD/bbl"}},
	})
}

// ScrapeNatgas returns EIA-sourced natural gas price points.
func (e *EIAClient) ScrapeNatgas(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesHenryHub, PricePoint{Symbol: "NG", Name: "Henry Hub Natural Gas", Sector: "natgas", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/MMBtu"}},
	})
}

// ScrapeLNG returns EIA-sourced LNG price points.
// JKM is sourced from EIA STEO (Short-Term Energy Outlook) series.
func (e *EIAClient) ScrapeLNG(ctx context.Context) []PricePoint {
	// JKM from EIA STEO — monthly frequency, use steo route
	price, period, _, err := e.fetchLatest(ctx, "steo", "LNGPK_JKMD")
	if err != nil {
		// JKM lookup failed — log and return empty for this source
		return nil
	}
	now := time.Now().UTC()
	return []PricePoint{{
		Symbol: "JKM", Name: "Japan-Korea Marker LNG", Sector: "lng",
		Exchange: "PLATTS", Geography: "ASIA_PAC", DeliveryMonth: period,
		Price: price, Unit: "USD/MMBtu", ScrapedAt: now, Source: "eia_api",
	}}
}

// ScrapeLPG returns EIA-sourced LPG price points.
func (e *EIAClient) ScrapeLPG(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesPropaneMB, PricePoint{Symbol: "C3", Name: "Propane Mont Belvieu", Sector: "lpg", Exchange: "OPIS", Geography: "US_GULF", Unit: "USD/gal"}},
	})
}

// ScrapeNGLs returns EIA-sourced NGL price points.
// EIA no longer publishes daily Mont Belvieu NGL spot prices via API v2; returns empty.
func (e *EIAClient) ScrapeNGLs(_ context.Context) []PricePoint { return nil }

// ScrapeElectricity returns EIA RTO day-ahead price points.
// Uses electricity/rto/daily-region-data — verify facet names against EIA API browser.
func (e *EIAClient) ScrapeElectricity(ctx context.Context) []PricePoint {
	type rtoSpec struct {
		respondent string
		symbol     string
		name       string
		geography  string
	}
	rtos := []rtoSpec{
		{"PJM", "PJMW", "PJM Day-Ahead (West Hub)", "US_MID_ATL"},
		{"CAISO", "CASP", "CAISO SP-15", "US_WEST"},
		{"ERCO", "ERCH", "ERCOT Houston Hub", "US_TEXAS"},
		{"MISO", "MISO", "MISO Illinois Hub", "US_MIDWEST"},
		{"NYISO", "NYZA", "NYISO Zone A", "US_NORTHEAST"},
	}

	now := time.Now().UTC()
	var points []PricePoint
	for _, rto := range rtos {
		url := fmt.Sprintf(
			"https://api.eia.gov/v2/electricity/rto/daily-region-data/data/?api_key=%s&frequency=daily&data[0]=value&facets[respondent][]=%s&facets[type][]=DF&sort[0][column]=period&sort[0][direction]=desc&length=1",
			e.apiKey, rto.respondent,
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := e.http.Do(req)
		if err != nil {
			continue
		}
		var r eiaResp
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		if len(r.Response.Data) == 0 {
			continue
		}
		d := r.Response.Data[0]
		v, err := parseEIAValue(d.Value)
		if err != nil || v <= 0 {
			continue
		}
		points = append(points, PricePoint{
			Symbol: rto.symbol, Name: rto.name, Sector: "electricity",
			Exchange: "RTO", Geography: rto.geography, DeliveryMonth: d.Period,
			Price: v, Unit: "USD/MWh", ScrapedAt: now, Source: "eia_api",
		})
	}
	return points
}

// ScrapeRefined returns EIA-sourced refined product price points.
func (e *EIAClient) ScrapeRefined(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesRBOB, PricePoint{Symbol: "RB", Name: "RBOB Gasoline (LA)", Sector: "refined", Exchange: "NYMEX", Geography: "US_WEST", Unit: "USD/gal"}},
		{seriesULSD, PricePoint{Symbol: "HO", Name: "Heating Oil / ULSD (NY Harbor)", Sector: "refined", Exchange: "NYMEX", Geography: "US_NORTHEAST", Unit: "USD/gal"}},
		{seriesJetFuel, PricePoint{Symbol: "JF", Name: "Jet Fuel (Gulf Coast)", Sector: "refined", Exchange: "OPIS", Geography: "US_GULF", Unit: "USD/gal"}},
	})
}

// fetchAll fetches 24 months of monthly history for each spec concurrently.
func fetchAll(ctx context.Context, e *EIAClient, specs []struct {
	series string
	meta   PricePoint
}) []PricePoint {
	type result struct {
		pts []PricePoint
	}
	ch := make(chan result, len(specs))
	for _, s := range specs {
		s := s
		go func() {
			pts, _ := e.fetchHistoryPoints(ctx, s.series, s.meta, 24)
			ch <- result{pts}
		}()
	}
	var points []PricePoint
	for range specs {
		r := <-ch
		points = append(points, r.pts...)
	}
	return points
}
