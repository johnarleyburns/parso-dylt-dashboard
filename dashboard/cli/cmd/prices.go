package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/render"
)

var validSectors = map[string]bool{
	"crude": true, "natgas": true, "lng": true,
	"lpg": true, "ngls": true, "electricity": true, "refined": true,
	"coal": true, "carbon": true,
}

var pricesCmd = &cobra.Command{
	Use:   "prices [sector]",
	Short: "Show spot prices — optionally filter by sector",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var sectors []string
		if len(args) == 1 {
			s := args[0]
			if !validSectors[s] {
				return fmt.Errorf("unknown sector %q — valid: crude natgas lng lpg ngls electricity refined coal carbon", s)
			}
			sectors = []string{s}
		}

		prices, err := apiClient.PricesAll(ctx)
		if err != nil {
			return fmt.Errorf("prices: %w", err)
		}
		fmt.Print(render.Prices(prices, sectors))
		return nil
	},
}
