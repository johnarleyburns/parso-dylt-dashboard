package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/render"
)

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Show node health table only",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cluster, err := apiClient.Cluster(ctx)
		if err != nil {
			return fmt.Errorf("cluster: %w", err)
		}
		healths := apiClient.HealthAll(ctx)
		fmt.Print(render.Nodes(cluster, healths))
		return nil
	},
}
