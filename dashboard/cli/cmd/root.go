package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"oilfield-dash/internal/client"
)

var (
	apiURL string
	domain string
	apiClient *client.Client
)

var rootCmd = &cobra.Command{
	Use:   "oilfield-dash",
	Short: "oilfield cluster dashboard — energy market data",
	Long: `oilfield-dash reads the oilfield cluster API and displays
energy market prices, node health, and news in the terminal.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		apiClient = client.New(apiURL, domain)
	},
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	defaultDomain := envOr("OILFIELD_DOMAIN", "oilfield.parso.guru")
	defaultAPI := envOr("OILFIELD_API_URL", "https://api."+defaultDomain)

	rootCmd.PersistentFlags().StringVar(&apiURL, "api", defaultAPI,
		"oilfield API base URL ($OILFIELD_API_URL)")
	rootCmd.PersistentFlags().StringVar(&domain, "domain", defaultDomain,
		"cluster domain for per-node health checks ($OILFIELD_DOMAIN)")

	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(nodesCmd)
	rootCmd.AddCommand(pricesCmd)
	rootCmd.AddCommand(newsCmd)
	rootCmd.AddCommand(watchCmd)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
