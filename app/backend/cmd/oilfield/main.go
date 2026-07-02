// Command oilfield is the single-server ("solo") build of the energy dashboard.
//
// One process serves the REST API, the embedded React frontend, and runs the
// scraper on an in-process ticker — all backed by one local SQLite file. It is
// the low-cost replacement for the multi-node etcd cluster + Cloudflare Pages.
package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"oilfield/internal/api"
	"oilfield/internal/scrapejob"
	"oilfield/internal/store"
	"oilfield/internal/webui"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoiOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}

func main() {
	scrapeOnce := flag.Bool("scrape-once", false, "run a single scrape cycle and exit")
	flag.Parse()

	nodeName := envOr("NODE_NAME", "solo")
	provider := envOr("NODE_PROVIDER", "ionos")
	dbPath := envOr("DB_PATH", "./.data/oilfield.db")
	addr := envOr("ADDR", ":8444")
	eiaKey := os.Getenv("EIA_API_KEY")
	oilKey := os.Getenv("OILPRICE_API_KEY")
	interval := time.Duration(atoiOr(os.Getenv("SCRAPE_INTERVAL"), 300)) * time.Second
	concurrency := atoiOr(os.Getenv("SCRAPE_CONCURRENCY"), 3)

	st, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	scrapeCfg := scrapejob.Config{
		NodeName:    nodeName,
		EIAKey:      eiaKey,
		OilPriceKey: oilKey,
		Concurrency: concurrency,
	}

	if *scrapeOnce {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		scrapejob.Run(ctx, st, scrapeCfg)
		return
	}

	if eiaKey == "" {
		log.Printf("[%s] warning: EIA_API_KEY not set — EIA sectors will be empty", nodeName)
	}

	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	st.Put(initCtx, "/oilfield/nodes/"+nodeName+"/provider", provider)
	st.Put(initCtx, "/oilfield/config/scrape_interval", strconv.Itoa(int(interval.Seconds())))
	cancel()

	// --- HTTP mux: API + embedded SPA ---
	mux := http.NewServeMux()
	srv := api.NewServer(st, nodeName, provider)
	srv.RegisterRoutes(mux)

	var static fs.FS = webui.Embedded()
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			static = os.DirFS(dir)
			log.Printf("[%s] serving frontend from disk: %s", nodeName, dir)
		}
	}
	mux.Handle("/", webui.Handler(static))

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- scraper ticker (single writer, no lock) ---
	scrapeCtx, stopScrape := context.WithCancel(context.Background())
	defer stopScrape()
	go func() {
		runOne := func() {
			ctx, c := context.WithTimeout(scrapeCtx, 8*time.Minute)
			defer c()
			scrapejob.Run(ctx, st, scrapeCfg)
		}
		if envOr("SCRAPE_ON_START", "true") == "true" {
			runOne()
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-scrapeCtx.Done():
				return
			case <-ticker.C:
				runOne()
			}
		}
	}()

	certFile, keyFile := os.Getenv("TLS_CERT"), os.Getenv("TLS_KEY")
	tls := certFile != "" && keyFile != "" && fileExists(certFile) && fileExists(keyFile)

	go func() {
		if tls {
			log.Printf("[%s] oilfield listening on %s (HTTPS)", nodeName, addr)
			if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen tls: %v", err)
			}
		} else {
			log.Printf("[%s] oilfield listening on %s (HTTP)", nodeName, addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %v", err)
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	log.Printf("[%s] shutdown signal received", nodeName)
	stopScrape()
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	httpServer.Shutdown(shutdownCtx)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
