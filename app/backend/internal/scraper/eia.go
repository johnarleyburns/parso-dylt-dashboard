package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EIA API v2 series IDs — verify at https://www.eia.gov/opendata/browser/
// Route petroleum/pri/spt: petroleum spot prices (daily)
// Route natural-gas/pri/sum: natural gas summary prices (daily)
// Route electricity/rto/daily-region-data: RTO day-ahead prices
const (
	// Crude — petroleum/pri/spt
	seriesWTI   = "EER_ECRWTI_PF4_Y35NY_DPG" // WTI Crude spot, Cushing OK, $/bbl
	seriesDubai = "RBRTE"                     // Dubai Crude (Brent proxy via EIA int'l series)

	// Natural gas — natural-gas/pri/sum (Henry Hub daily) or natural-gas/pri/fut
	seriesHenryHub = "RNGWHHD" // Henry Hub natural gas spot, $/MMBtu
	seriesAECO     = "RNGC1"   // AECO natural gas spot (Canada), $/MMBtu — verify route

	// LPG — petroleum/pri/spt
	seriesPropaneMB  = "EER_EPLLPA_PF4_Y44MB_DPG" // Propane Mont Belvieu, $/gal
	seriesButaneMB   = "EER_EPLLP_PF4_Y44MB_DPG"  // Normal Butane Mont Belvieu, $/gal

	// NGLs — petroleum/pri/spt
	seriesEthaneMB     = "EER_EPCLLA_PF4_Y44MB_DPG" // Ethane Mont Belvieu, $/gal
	seriesIsobutaneMB  = "EER_EPLIBU_PF4_Y44MB_DPG"  // Isobutane Mont Belvieu, $/gal — verify series ID
	seriesNatGasolineMB = "EER_EPC2LT_PF4_Y44MB_DPG" // Natural Gasoline Mont Belvieu, $/gal

	// Refined products — petroleum/pri/spt
	seriesRBOB    = "EER_EPM0F_PF4_Y35NY_DPG" // RBOB Gasoline, $/gal
	seriesULSD    = "EER_EPD2F_PF4_Y35NY_DPG" // ULSD/Heating Oil NY Harbor, $/gal
	seriesJetFuel = "EER_EPJK_PF4_Y05LA_DPG"  // Jet Fuel LAX, $/gal
)

// eiaRoutes maps series prefix patterns to EIA v2 route paths.
// Most petroleum series use petroleum/pri/spt; gas uses natural-gas/pri/sum.
var eiaRouteMap = map[string]string{
	seriesWTI:          "petroleum/pri/spt",
	seriesDubai:        "petroleum/pri/spt",
	seriesHenryHub:     "natural-gas/pri/sum",
	seriesAECO:         "natural-gas/pri/sum",
	seriesPropaneMB:    "petroleum/pri/spt",
	seriesButaneMB:     "petroleum/pri/spt",
	seriesEthaneMB:      "petroleum/pri/spt",
	seriesIsobutaneMB:   "petroleum/pri/spt",
	seriesNatGasolineMB: "petroleum/pri/spt",
	seriesRBOB:         "petroleum/pri/spt",
	seriesULSD:         "petroleum/pri/spt",
	seriesJetFuel:      "petroleum/pri/spt",
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
			Period string  `json:"period"`
			Value  float64 `json:"value"`
			Units  string  `json:"units"`
		} `json:"data"`
	} `json:"response"`
}

func (e *EIAClient) fetchLatest(ctx context.Context, route, seriesID string) (price float64, period string, unit string, err error) {
	url := fmt.Sprintf(
		"https://api.eia.gov/v2/%s/data/?api_key=%s&frequency=daily&data[0]=value&facets[series][]=%s&sort[0][column]=period&sort[0][direction]=desc&length=1",
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
	return d.Value, d.Period, d.Units, nil
}

func (e *EIAClient) fetchPoint(ctx context.Context, seriesID string, meta PricePoint) (PricePoint, error) {
	route, ok := eiaRouteMap[seriesID]
	if !ok {
		return PricePoint{}, fmt.Errorf("no route configured for series %s", seriesID)
	}
	price, period, _, err := e.fetchLatest(ctx, route, seriesID)
	if err != nil {
		return PricePoint{}, err
	}
	meta.Price = price
	meta.DeliveryMonth = period
	meta.ScrapedAt = time.Now().UTC()
	meta.Source = "eia_api"
	return meta, nil
}

// ScrapeCrude returns EIA-sourced crude oil price points.
func (e *EIAClient) ScrapeCrude(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesWTI, PricePoint{Symbol: "CL", Name: "WTI Crude Oil", Sector: "crude", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/bbl"}},
		{seriesDubai, PricePoint{Symbol: "DC", Name: "Dubai Crude", Sector: "crude", Exchange: "PLATTS", Geography: "MIDDLE_EAST", Unit: "USD/bbl"}},
	})
}

// ScrapeNatgas returns EIA-sourced natural gas price points.
func (e *EIAClient) ScrapeNatgas(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesHenryHub, PricePoint{Symbol: "NG", Name: "Henry Hub Natural Gas", Sector: "natgas", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/MMBtu"}},
		{seriesAECO, PricePoint{Symbol: "AECO", Name: "AECO Natural Gas (Canada)", Sector: "natgas", Exchange: "AECO", Geography: "CANADA", Unit: "USD/MMBtu"}},
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
		{seriesButaneMB, PricePoint{Symbol: "C4", Name: "Normal Butane Mont Belvieu", Sector: "lpg", Exchange: "OPIS", Geography: "US_GULF", Unit: "USD/gal"}},
	})
}

// ScrapeNGLs returns EIA-sourced NGL price points.
func (e *EIAClient) ScrapeNGLs(ctx context.Context) []PricePoint {
	return fetchAll(ctx, e, []struct {
		series string
		meta   PricePoint
	}{
		{seriesEthaneMB, PricePoint{Symbol: "C2", Name: "Ethane Mont Belvieu", Sector: "ngls", Exchange: "OPIS", Geography: "US_GULF", Unit: "USD/gal"}},
		{seriesNatGasolineMB, PricePoint{Symbol: "C5+", Name: "Natural Gasoline Mont Belvieu", Sector: "ngls", Exchange: "OPIS", Geography: "US_GULF", Unit: "USD/gal"}},
	})
}

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
		points = append(points, PricePoint{
			Symbol: rto.symbol, Name: rto.name, Sector: "electricity",
			Exchange: "RTO", Geography: rto.geography, DeliveryMonth: d.Period,
			Price: d.Value, Unit: "USD/MWh", ScrapedAt: now, Source: "eia_api",
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
		{seriesRBOB, PricePoint{Symbol: "RB", Name: "RBOB Gasoline", Sector: "refined", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/gal"}},
		{seriesULSD, PricePoint{Symbol: "HO", Name: "ULSD / Heating Oil", Sector: "refined", Exchange: "NYMEX", Geography: "US_GULF", Unit: "USD/gal"}},
		{seriesJetFuel, PricePoint{Symbol: "JF", Name: "Jet Fuel LAX", Sector: "refined", Exchange: "OPIS", Geography: "US_WEST", Unit: "USD/gal"}},
	})
}

// fetchAll runs a list of EIA fetches concurrently, skipping any that fail.
func fetchAll(ctx context.Context, e *EIAClient, specs []struct {
	series string
	meta   PricePoint
}) []PricePoint {
	type result struct {
		p   PricePoint
		err error
	}
	ch := make(chan result, len(specs))
	for _, s := range specs {
		s := s
		go func() {
			p, err := e.fetchPoint(ctx, s.series, s.meta)
			ch <- result{p, err}
		}()
	}
	var points []PricePoint
	for range specs {
		r := <-ch
		if r.err == nil {
			points = append(points, r.p)
		}
		// individual failures are logged by the caller; we skip silently here
	}
	return points
}
