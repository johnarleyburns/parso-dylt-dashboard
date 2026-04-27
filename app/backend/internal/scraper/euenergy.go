package scraper

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EUEnergyScraper fetches EPEX day-ahead electricity spot prices from euenergy.live.
// Source data is ultimately from ENTSO-E Transparency Platform (free, no key).
// Prices are EUR/MWh, updated daily at ~13:00 CET for the next day.
type EUEnergyScraper struct {
	http *http.Client
}

func NewEUEnergyScraper() *EUEnergyScraper {
	return &EUEnergyScraper{http: &http.Client{Timeout: 15 * time.Second}}
}

// countryLink matches: country_link">COUNTRY</a>...<td class="price">PRICE</td>
var epexRowRe = regexp.MustCompile(`country_link">([^<]+)</a>.*?class="price">([\d.]+)`)

// epexGeoMap maps the display name from euenergy.live to ISO-2 + geography.
var epexGeoMap = map[string][2]string{
	"Germany":     {"DE", "EUROPE"},
	"France":      {"FR", "EUROPE"},
	"Spain":       {"ES", "EUROPE"},
	"Italy":       {"IT", "EUROPE"},
	"Netherlands": {"NL", "EUROPE"},
	"Belgium":     {"BE", "EUROPE"},
	"Poland":      {"PL", "EUROPE"},
	"Austria":     {"AT", "EUROPE"},
	"Czechia":     {"CZ", "EUROPE"},
	"Sweden":      {"SE", "EUROPE"},
	"Norway":      {"NO", "EUROPE"},
	"Denmark":     {"DK", "EUROPE"},
	"Portugal":    {"PT", "EUROPE"},
	"Greece":      {"GR", "EUROPE"},
	"Romania":     {"RO", "EUROPE"},
}

// ScrapeEPEXSpot returns day-ahead EPEX electricity prices for key European markets.
// Multi-zone countries (NO1/NO2, SE1-SE4, DK1/DK2) keep only the first zone encountered.
func (c *EUEnergyScraper) ScrapeEPEXSpot(ctx context.Context) []PricePoint {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://euenergy.live/", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; oilfield-scraper/1.0)")
	req.Header.Set("Accept", "text/html")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}

	seen := map[string]bool{}
	var pts []PricePoint
	today := time.Now().UTC().Format("2006-01-02")

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		m := epexRowRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		country := strings.TrimSpace(m[1])
		geo, ok := epexGeoMap[country]
		if !ok || seen[geo[0]] {
			continue
		}
		price, err := strconv.ParseFloat(m[2], 64)
		if err != nil || price <= 0 {
			continue
		}
		seen[geo[0]] = true
		pts = append(pts, PricePoint{
			Symbol:        fmt.Sprintf("EPEX_%s", geo[0]),
			Name:          country + " Day-Ahead Electricity",
			Sector:        "electricity",
			Exchange:      "EPEX",
			Geography:     geo[1],
			DeliveryMonth: today,
			Price:         price,
			Unit:          "EUR/MWh",
			ScrapedAt:     time.Now().UTC(),
			Source:        "euenergy.live/ENTSO-E",
		})
	}
	return pts
}
