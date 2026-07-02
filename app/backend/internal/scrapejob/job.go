// Package scrapejob runs one full scrape cycle against every data source and
// writes the results into the store. It is the single-server replacement for the
// old cmd/scraper orchestration: no distributed lock (single writer), bounded
// concurrency to protect a small shared host, callable both from an in-process
// ticker and from a one-shot CLI invocation.
package scrapejob

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"oilfield/internal/scraper"
)

// Store is the write surface the scrape job needs.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string) error
	PutJSON(ctx context.Context, key string, v any) error
	GetJSON(ctx context.Context, key string, dest any) error
}

type Config struct {
	NodeName    string
	EIAKey      string
	OilPriceKey string
	Concurrency int // max concurrent source fetches (default 3)
}

type Result struct {
	PricePoints int
	NewsItems   int
}

// Run executes one full scrape cycle. It never returns an error for individual
// source failures (those are logged); it only returns after all sources settle.
func Run(ctx context.Context, st Store, cfg Config) Result {
	node := cfg.NodeName
	if node == "" {
		node = "solo"
	}
	conc := cfg.Concurrency
	if conc < 1 {
		conc = 3
	}
	sem := make(chan struct{}, conc)

	st.Put(ctx, "/oilfield/config/active_node", node)
	st.Put(ctx, "/oilfield/nodes/"+node+"/status", "ok")

	eia := scraper.NewEIAClient(cfg.EIAKey)
	yf := scraper.NewYahooFinanceScraper()
	inv := scraper.NewInvestingScraper()
	fred := scraper.NewFREDClient()
	oilprice := scraper.NewOilPriceAPIClient(cfg.OilPriceKey)
	euenergy := scraper.NewEUEnergyScraper()
	eurostat := scraper.NewEurostatClient()
	aemo := scraper.NewAEMOClient()

	type sectorResult struct {
		sector string
		points []scraper.PricePoint
	}
	results := make(chan sectorResult, 32)
	var wg sync.WaitGroup

	run := func(sector string, fn func() []scraper.PricePoint) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- sectorResult{sector, fn()}
		}()
	}
	runSingle := func(sector string, fn func() (scraper.PricePoint, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := fn()
			if err != nil {
				log.Printf("[%s] %s scrape error: %v", node, sector, err)
				results <- sectorResult{sector, nil}
				return
			}
			results <- sectorResult{sector, []scraper.PricePoint{p}}
		}()
	}

	run("crude/eia", func() []scraper.PricePoint { return eia.ScrapeCrude(ctx) })
	run("natgas/eia", func() []scraper.PricePoint { return eia.ScrapeNatgas(ctx) })
	run("lng/eia", func() []scraper.PricePoint { return eia.ScrapeLNG(ctx) })
	run("lpg/eia", func() []scraper.PricePoint { return eia.ScrapeLPG(ctx) })
	run("ngls/eia", func() []scraper.PricePoint { return eia.ScrapeNGLs(ctx) })
	run("electricity/eia", func() []scraper.PricePoint { return eia.ScrapeElectricity(ctx) })
	run("refined/eia", func() []scraper.PricePoint { return eia.ScrapeRefined(ctx) })

	runSingle("crude/wti_fut", func() (scraper.PricePoint, error) { return yf.ScrapeWTI(ctx) })
	runSingle("crude/brent_fut", func() (scraper.PricePoint, error) { return yf.ScrapeBrent(ctx) })
	runSingle("natgas/hh_fut", func() (scraper.PricePoint, error) { return yf.ScrapeNatGas(ctx) })
	runSingle("natgas/ttf_fut", func() (scraper.PricePoint, error) { return yf.ScrapeTTF(ctx) })
	runSingle("refined/ho_fut", func() (scraper.PricePoint, error) { return yf.ScrapeHeatingOil(ctx) })
	runSingle("refined/rb_fut", func() (scraper.PricePoint, error) { return yf.ScrapeRBOB(ctx) })

	runSingle("natgas/ttf", func() (scraper.PricePoint, error) { return inv.ScrapeTTF(ctx) })

	runSingle("crude/dubai_fred", func() (scraper.PricePoint, error) { return fred.ScrapeDubaiCrude(ctx) })
	runSingle("coal/newc_fred", func() (scraper.PricePoint, error) { return fred.ScrapeNewcastleCoal(ctx) })
	runSingle("coal/colombia", func() (scraper.PricePoint, error) { return fred.ScrapeColombiaCoal(ctx) })
	runSingle("natgas/eu_fred", func() (scraper.PricePoint, error) { return fred.ScrapeNatGasEurope(ctx) })
	runSingle("lng/japan_fred", func() (scraper.PricePoint, error) { return fred.ScrapeNatGasJapan(ctx) })

	if oilprice.IsEnabled() {
		runSingle("crude/dubai_op", func() (scraper.PricePoint, error) { return oilprice.ScrapeDubaiCrude(ctx) })
		runSingle("crude/urals", func() (scraper.PricePoint, error) { return oilprice.ScrapeUrals(ctx) })
		runSingle("refined/sg_vlsfo", func() (scraper.PricePoint, error) { return oilprice.ScrapeSingaporeVLSFO(ctx) })
		runSingle("lng/jkm", func() (scraper.PricePoint, error) { return oilprice.ScrapeJKM(ctx) })
		runSingle("coal/newc_op", func() (scraper.PricePoint, error) { return oilprice.ScrapeNewcastleCoal(ctx) })
		runSingle("carbon/eua", func() (scraper.PricePoint, error) { return oilprice.ScrapeEUCarbon(ctx) })
	} else {
		log.Printf("[%s] OilPriceAPI disabled — set OILPRICE_API_KEY to enable Dubai, Urals, JKM, Singapore, coal, carbon", node)
	}

	run("electricity/epex", func() []scraper.PricePoint { return euenergy.ScrapeEPEXSpot(ctx) })
	run("electricity/eurostat", func() []scraper.PricePoint { return eurostat.ScrapeHouseholdElectricity(ctx) })
	run("electricity/aemo", func() []scraper.PricePoint { return aemo.ScrapeNEM(ctx) })

	newsSources := []struct {
		slug  string
		url   string
		label string
	}{
		{"eia", "https://www.eia.gov/rss/todayinenergy.xml", "EIA"},
		{"oilprice", "https://oilprice.com/rss/main", "OilPrice"},
		{"doe", "https://www.energy.gov/rss.xml", "US DOE"},
		{"eu_energy", "https://energy.ec.europa.eu/node/2/rss_en", "EU Energy"},
		{"uk_desnz", "https://www.gov.uk/government/organisations/department-for-energy-security-and-net-zero.atom", "UK DESNZ"},
		{"canada_nrc", "https://natural-resources.canada.ca/rss.xml", "Canada NRC"},
		{"rigzone", "https://www.rigzone.com/news/rss/rigzone_latest.aspx", "Rigzone"},
		{"carbon_brief", "https://www.carbonbrief.org/feed/", "Carbon Brief"},
		{"ieefa", "https://ieefa.org/feed/", "IEEFA"},
		{"energy_monitor", "https://www.energymonitor.ai/feed/", "Energy Monitor"},
	}

	newsResults := make(map[string][]scraper.NewsItem)
	var newsMu sync.Mutex
	for _, src := range newsSources {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items, err := scraper.ScrapeNewsRSS(ctx, src.url, src.label)
			if err != nil {
				log.Printf("[%s] RSS error (%s): %v", node, src.slug, err)
				return
			}
			var existing []scraper.NewsItem
			st.GetJSON(ctx, "/oilfield/news/"+src.slug+"/items", &existing)
			merged := scraper.MergeNews(items, existing)
			newsMu.Lock()
			newsResults[src.slug] = merged
			newsMu.Unlock()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	sectorPrices := make(map[string][]scraper.PricePoint)
	for r := range results {
		sector := r.sector
		if idx := strings.Index(sector, "/"); idx >= 0 {
			sector = sector[:idx]
		}
		sectorPrices[sector] = append(sectorPrices[sector], r.points...)
	}

	total := 0
	for sector, points := range sectorPrices {
		if len(points) == 0 {
			continue
		}
		key := "/oilfield/prices/" + sector + "/latest"
		if err := st.PutJSON(ctx, key, points); err != nil {
			log.Printf("[%s] db write error for %s: %v", node, key, err)
		} else {
			total += len(points)
		}
	}

	newsMu.Lock()
	newCount := 0
	for source, items := range newsResults {
		if err := st.PutJSON(ctx, "/oilfield/news/"+source+"/items", items); err != nil {
			log.Printf("[%s] db news write error for %s: %v", node, source, err)
		}
		newCount += len(items)
	}
	newsMu.Unlock()

	st.Put(ctx, "/oilfield/nodes/"+node+"/heartbeat", time.Now().UTC().Format(time.RFC3339))
	log.Printf("[%s] scrape complete — %d price points, %d news items written", node, total, newCount)

	return Result{PricePoints: total, NewsItems: newCount}
}
