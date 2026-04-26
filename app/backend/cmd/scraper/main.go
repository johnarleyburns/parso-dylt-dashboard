package main

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"oilfield/internal/etcdstore"
	"oilfield/internal/scraper"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	nodeName := mustEnv("NODE_NAME")
	eiaKey := mustEnv("EIA_API_KEY")
	endpoints := strings.Split(envOr("ETCD_ENDPOINTS", "http://127.0.0.1:2379"), ",")

	store, err := etcdstore.New(endpoints)
	if err != nil {
		log.Fatalf("etcd connect: %v", err)
	}
	defer store.Close()

	// 8-minute ceiling keeps scrape cycles from overlapping (timer fires every 5 min).
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	const lockKey = "/oilfield/locks/scrape"

	// ⚠️ LEASE-BACKED LOCK — TTL=120s ensures auto-release on crash.
	// Do NOT use a plain Put here (Agent Failure Case #1).
	leaseID, acquired, err := store.AcquireLock(ctx, lockKey, nodeName, 120)
	if err != nil {
		log.Fatalf("lock acquire error: %v", err)
	}
	if !acquired {
		holder, _ := store.Get(ctx, lockKey)
		log.Printf("scrape lock held by %q — exiting (this is normal)", holder)
		os.Exit(0)
	}
	defer store.RevokeLease(context.Background(), leaseID)

	log.Printf("[%s] scrape lock acquired — starting scrape cycle", nodeName)
	store.Put(ctx, "/oilfield/config/active_node", nodeName)
	store.Put(ctx, "/oilfield/nodes/"+nodeName+"/status", "ok")

	eia := scraper.NewEIAClient(eiaKey)
	yf  := scraper.NewYahooFinanceScraper()
	inv := scraper.NewInvestingScraper()

	type sectorResult struct {
		sector string
		points []scraper.PricePoint
	}

	results := make(chan sectorResult, 16)
	var wg sync.WaitGroup

	run := func(sector string, fn func() []scraper.PricePoint) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pts := fn()
			results <- sectorResult{sector, pts}
		}()
	}

	runSingle := func(sector string, fn func() (scraper.PricePoint, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := fn()
			if err != nil {
				log.Printf("[%s] %s scrape error: %v", nodeName, sector, err)
				results <- sectorResult{sector, nil}
				return
			}
			results <- sectorResult{sector, []scraper.PricePoint{p}}
		}()
	}

	// EIA API sectors (concurrent, each is independently reliable)
	run("crude/eia", func() []scraper.PricePoint { return eia.ScrapeCrude(ctx) })
	run("natgas/eia", func() []scraper.PricePoint { return eia.ScrapeNatgas(ctx) })
	run("lng/eia", func() []scraper.PricePoint { return eia.ScrapeLNG(ctx) })
	run("lpg/eia", func() []scraper.PricePoint { return eia.ScrapeLPG(ctx) })
	run("ngls/eia", func() []scraper.PricePoint { return eia.ScrapeNGLs(ctx) })
	run("electricity/eia", func() []scraper.PricePoint { return eia.ScrapeElectricity(ctx) })
	run("refined/eia", func() []scraper.PricePoint { return eia.ScrapeRefined(ctx) })

	// Yahoo Finance scrapers — JSON API, no CSS selectors, resilient to exchange site changes.
	// Provides front-month futures prices as a complement to EIA spot prices.
	runSingle("crude/wti_fut", func() (scraper.PricePoint, error) { return yf.ScrapeWTI(ctx) })
	runSingle("crude/brent_fut", func() (scraper.PricePoint, error) { return yf.ScrapeBrent(ctx) })
	runSingle("natgas/hh_fut", func() (scraper.PricePoint, error) { return yf.ScrapeNatGas(ctx) })
	runSingle("natgas/ttf_fut", func() (scraper.PricePoint, error) { return yf.ScrapeTTF(ctx) })
	runSingle("refined/ho_fut", func() (scraper.PricePoint, error) { return yf.ScrapeHeatingOil(ctx) })
	runSingle("refined/rb_fut", func() (scraper.PricePoint, error) { return yf.ScrapeRBOB(ctx) })

	// Investing.com HTML scraper — best-effort for TTF spot (cross-check against YF futures).
	runSingle("natgas/ttf", func() (scraper.PricePoint, error) { return inv.ScrapeTTF(ctx) })

	// News RSS — table-driven, one goroutine per source (gofeed, rate-limit friendly)
	newsSources := []struct {
		slug  string
		url   string
		label string
	}{
		{"eia",            "https://www.eia.gov/rss/todayinenergy.xml",                                                                                           "EIA"},
		{"oilprice",       "https://oilprice.com/rss/main",                                                                                                       "OilPrice"},
		{"doe",            "https://www.energy.gov/rss.xml",                                                                                                      "US DOE"},
		{"eu_energy",      "https://energy.ec.europa.eu/node/2/rss_en",                                                                                           "EU Energy"},
		{"uk_desnz",       "https://www.gov.uk/government/organisations/department-for-energy-security-and-net-zero.atom",                                        "UK DESNZ"},
		{"canada_nrc",     "https://natural-resources.canada.ca/rss.xml",                                                                                         "Canada NRC"},
		{"rigzone",        "https://www.rigzone.com/news/rss/rigzone_latest.aspx",                                                                                "Rigzone"},
		{"carbon_brief",   "https://www.carbonbrief.org/feed/",                                                                                                   "Carbon Brief"},
		{"ieefa",          "https://ieefa.org/feed/",                                                                                                             "IEEFA"},
		{"energy_monitor", "https://www.energymonitor.ai/feed/",                                                                                                  "Energy Monitor"},
	}

	newsResults := make(map[string][]scraper.NewsItem)
	var newsMu sync.Mutex
	for _, src := range newsSources {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := scraper.ScrapeNewsRSS(ctx, src.url, src.label)
			if err != nil {
				log.Printf("[%s] RSS error (%s): %v", nodeName, src.slug, err)
				return
			}
			var existing []scraper.NewsItem
			store.GetJSON(ctx, "/oilfield/news/"+src.slug+"/items", &existing)
			merged := scraper.MergeNews(items, existing)
			newsMu.Lock()
			newsResults[src.slug] = merged
			newsMu.Unlock()
		}()
	}

	// Wait for all goroutines, then close results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Accumulate price results by sector (merge EIA + HTML into same sector key).
	// Single-goroutine consumer of a closed channel — no mutex needed.
	sectorPrices := make(map[string][]scraper.PricePoint)
	for r := range results {
		// Strip the source suffix (e.g. "crude/eia" → "crude")
		sector := r.sector
		if idx := strings.Index(sector, "/"); idx >= 0 {
			sector = sector[:idx]
		}
		sectorPrices[sector] = append(sectorPrices[sector], r.points...)
	}

	// Write price data to etcd
	total := 0
	for sector, points := range sectorPrices {
		if len(points) == 0 {
			continue
		}
		key := "/oilfield/prices/" + sector + "/latest"
		if err := store.PutJSON(ctx, key, points); err != nil {
			log.Printf("[%s] etcd write error for %s: %v", nodeName, key, err)
		} else {
			total += len(points)
		}
	}

	// Write news to etcd
	newsMu.Lock()
	for source, items := range newsResults {
		if err := store.PutJSON(ctx, "/oilfield/news/"+source+"/items", items); err != nil {
			log.Printf("[%s] etcd news write error for %s: %v", nodeName, source, err)
		}
	}
	newCount := 0
	for _, items := range newsResults {
		newCount += len(items)
	}
	newsMu.Unlock()

	// Heartbeat
	store.Put(ctx, "/oilfield/nodes/"+nodeName+"/heartbeat", time.Now().UTC().Format(time.RFC3339))

	log.Printf("[%s] scrape complete — %d price points, %d news items written to etcd", nodeName, total, newCount)
}
