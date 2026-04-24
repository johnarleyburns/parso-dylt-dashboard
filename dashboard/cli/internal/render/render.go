package render

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"oilfield-dash/internal/client"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	headerStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))  // green
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // yellow
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray
	boldStyle   = lipgloss.NewStyle().Bold(true)
)

const divider = "═══════════════════════════════════════════════════════════"

// Status renders the full cluster status view.
func Status(cluster client.ClusterStatus, healths map[string]client.NodeHealth) string {
	var b strings.Builder
	now := time.Now().UTC()

	b.WriteString(titleStyle.Render(fmt.Sprintf("OILFIELD CLUSTER STATUS  [%s]", now.Format("2006-01-02 15:04:05 UTC"))))
	b.WriteString("\n" + divider + "\n")
	b.WriteString(Nodes(cluster, healths))
	return b.String()
}

// Nodes renders the node table.
func Nodes(cluster client.ClusterStatus, healths map[string]client.NodeHealth) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(
		fmt.Sprintf("%-6s %-12s %-12s %-14s %s", "NODE", "PROVIDER", "STATUS", "HEARTBEAT", "SCRAPE LOCK"),
	))
	b.WriteString("\n")

	for _, name := range []string{"n1", "n2", "n3"} {
		node := cluster.Nodes[name]
		health := healths[name]

		provider := "—"
		heartbeatStr := "—"
		statusStr := dimStyle.Render("● UNKNOWN")

		if node != nil {
			provider = titleCase(node.Provider)
			if node.Heartbeat != "" {
				if t, err := time.Parse(time.RFC3339, node.Heartbeat); err == nil {
					heartbeatStr = timeAgo(t)
				}
			}
		}

		switch health.Status {
		case "ok":
			statusStr = okStyle.Render("● OK")
		case "degraded":
			statusStr = warnStyle.Render("● DEGRADED")
		case "offline", "":
			statusStr = offStyle.Render("● OFFLINE")
		}

		lockStr := dimStyle.Render("—")
		if cluster.Scrapelock == name {
			lockStr = warnStyle.Render("HELD")
		}

		b.WriteString(fmt.Sprintf("%-6s %-12s %-20s %-14s %s\n",
			name, provider, statusStr, heartbeatStr, lockStr))
	}

	// Scrape summary line
	b.WriteString("\n")
	active := cluster.ActiveNode
	if active == "" {
		active = "—"
	}
	interval := cluster.ScrapeInterval
	if interval == "" {
		interval = "300"
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("Active scraper: %s   Interval: %ss", active, interval)))
	b.WriteString("\n")

	return b.String()
}

// Prices renders a price table for the given sectors. Pass nil sectors for all.
func Prices(prices client.AllPrices, sectors []string) string {
	if sectors == nil {
		sectors = []string{"crude", "natgas", "lng", "lpg", "ngls", "electricity", "refined"}
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(
		fmt.Sprintf("%-12s %-8s %-32s %-10s %-8s %s",
			"SECTOR", "SYMBOL", "NAME", "PRICE", "UNIT", "EXCH"),
	))
	b.WriteString("\n")

	found := false
	for _, sector := range sectors {
		pts, ok := prices[sector]
		if !ok || len(pts) == 0 {
			continue
		}
		found = true
		for _, p := range pts {
			priceStr := fmt.Sprintf("$%.2f", p.Price)
			b.WriteString(fmt.Sprintf("  %-10s %-8s %-32s %-10s %-8s %s\n",
				sector,
				p.Symbol,
				truncate(p.Name, 30),
				priceStr,
				p.Unit,
				p.Exchange,
			))
		}
	}
	if !found {
		b.WriteString(dimStyle.Render("  No price data available yet — scraper has not run"))
		b.WriteString("\n")
	}
	return b.String()
}

// News renders the news feed.
func News(news client.NewsResponse, limit int) string {
	var b strings.Builder

	items := append(news.EIA, news.IEA...)
	// Sort newest-first by pub date (simple bubble — small list)
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].PublishedAt.After(items[i].PublishedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	if len(items) == 0 {
		b.WriteString(dimStyle.Render("No news items available yet."))
		b.WriteString("\n")
		return b.String()
	}

	for _, item := range items {
		date := item.PublishedAt.Format("2006-01-02")
		source := fmt.Sprintf("[%s]", item.Source)
		b.WriteString(boldStyle.Render(item.Title))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %s %s  %s", date, source, item.URL)))
		b.WriteString("\n")
		if item.Summary != "" {
			b.WriteString(fmt.Sprintf("  %s\n", truncate(item.Summary, 100)))
		}
		if len(item.Tags) > 0 {
			b.WriteString(dimStyle.Render("  #" + strings.Join(item.Tags, " #")))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- helpers ----

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// truncate clips s to maxRunes runes, appending "…" if clipped.
func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-1]) + "…"
}
