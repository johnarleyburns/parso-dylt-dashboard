package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/client"
	"oilfield-dash/internal/render"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show full cluster status — nodes, prices, recent news",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cluster, healths, prices, news, err := fetchAll(ctx)
	if err != nil {
		return err
	}

	fmt.Print(render.Status(cluster, healths))
	fmt.Println()
	fmt.Println(render.Prices(prices, nil))
	fmt.Println(render.News(news, 5))
	return nil
}

// fetchAll retrieves cluster, health, prices and news concurrently.
func fetchAll(ctx context.Context) (
	cluster client.ClusterStatus,
	healths map[string]client.NodeHealth,
	prices client.AllPrices,
	news client.NewsResponse,
	err error,
) {
	var wg sync.WaitGroup
	var clusterErr, pricesErr, newsErr error

	wg.Add(3)
	go func() {
		defer wg.Done()
		cluster, clusterErr = apiClient.Cluster(ctx)
	}()
	go func() {
		defer wg.Done()
		prices, pricesErr = apiClient.PricesAll(ctx)
	}()
	go func() {
		defer wg.Done()
		news, newsErr = apiClient.News(ctx)
	}()

	// Node health runs separately (direct per-node calls)
	healths = apiClient.HealthAll(ctx)
	wg.Wait()

	for _, e := range []error{clusterErr, pricesErr, newsErr} {
		if e != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
	}
	return cluster, healths, prices, news, nil
}
