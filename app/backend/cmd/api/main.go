package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"oilfield/internal/api"
	"oilfield/internal/etcdstore"
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
	provider := envOr("NODE_PROVIDER", "unknown")
	endpoints := strings.Split(envOr("ETCD_ENDPOINTS", "http://127.0.0.1:2379"), ",")
	addr := envOr("API_ADDR", ":8080")

	store, err := etcdstore.New(endpoints)
	if err != nil {
		log.Fatalf("etcd connect: %v", err)
	}
	defer store.Close()

	// Write node identity into etcd on startup (best-effort; non-fatal if etcd is initialising)
	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	store.Put(initCtx, "/oilfield/nodes/"+nodeName+"/provider", provider)
	store.Put(initCtx, "/oilfield/config/scrape_interval", "300")
	cancel()

	mux := http.NewServeMux()
	srv := api.NewServer(store, nodeName, provider)
	srv.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[%s] oilfield-api listening on %s", nodeName, addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	log.Printf("[%s] shutdown signal received", nodeName)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
}
