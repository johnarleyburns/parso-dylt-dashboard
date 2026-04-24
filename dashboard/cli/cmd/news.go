package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/render"
)

var newsLimit int

var newsCmd = &cobra.Command{
	Use:   "news",
	Short: "Show latest energy market news (EIA + IEA RSS)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		news, err := apiClient.News(ctx)
		if err != nil {
			return fmt.Errorf("news: %w", err)
		}
		fmt.Print(render.News(news, newsLimit))
		return nil
	},
}

func init() {
	newsCmd.Flags().IntVarP(&newsLimit, "limit", "n", 20, "max number of news items to show")
}
