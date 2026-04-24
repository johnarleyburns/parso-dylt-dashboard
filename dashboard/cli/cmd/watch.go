package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/render"
)

var watchInterval int

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Auto-refresh cluster status every N seconds (default 10)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

		refresh := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			cluster, healths, prices, news, _ := fetchAll(ctx)

			clearScreen()
			fmt.Print(render.Status(cluster, healths))
			fmt.Println()
			fmt.Print(render.Prices(prices, nil))
			fmt.Println()
			fmt.Print(render.News(news, 5))
			fmt.Printf("\n%s  (refreshes every %ds — Ctrl+C to exit)\n",
				time.Now().UTC().Format("15:04:05 UTC"), watchInterval)
		}

		refresh()
		ticker := time.NewTicker(time.Duration(watchInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				refresh()
			case <-sig:
				fmt.Println("\nexiting watch")
				return nil
			}
		}
	},
}

func init() {
	watchCmd.Flags().IntVarP(&watchInterval, "interval", "i", 10, "refresh interval in seconds")
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
