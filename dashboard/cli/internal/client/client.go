package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client wraps the oilfield HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
	domain  string
	// NodeURLFunc builds the health check URL for a given node name and domain.
	// Defaults to https://{node}.{domain}/api/v1/health.
	// Override in tests to avoid external DNS lookups.
	NodeURLFunc func(node, domain string) string
}

// New returns a Client targeting the given base URL (e.g. "https://api.oilfield.parso.guru").
// domain is used to derive per-node health URLs (e.g. "oilfield.parso.guru").
func New(baseURL, domain string) *Client {
	return &Client{
		baseURL: baseURL,
		domain:  domain,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) nodeHealthURL(node string) string {
	if c.NodeURLFunc != nil {
		return c.NodeURLFunc(node, c.domain)
	}
	return fmt.Sprintf("https://%s.%s/api/v1/health", node, c.domain)
}

func (c *Client) get(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// Cluster fetches cluster state (nodes, scrape lock, active_node) from the API.
func (c *Client) Cluster(ctx context.Context) (ClusterStatus, error) {
	var s ClusterStatus
	err := c.get(ctx, c.baseURL+"/api/v1/cluster", &s)
	return s, err
}

// Health fetches the health of a specific node by name.
func (c *Client) Health(ctx context.Context, node string) (NodeHealth, error) {
	var h NodeHealth
	err := c.get(ctx, c.nodeHealthURL(node), &h)
	return h, err
}

// HealthAll fetches health from all three runtime nodes concurrently.
// Returns results keyed by node name; unreachable nodes are marked degraded.
func (c *Client) HealthAll(ctx context.Context) map[string]NodeHealth {
	nodes := []string{"n1", "n2", "n3"}
	type result struct {
		name string
		h    NodeHealth
		err  error
	}
	ch := make(chan result, len(nodes))
	for _, n := range nodes {
		n := n
		go func() {
			h, err := c.Health(ctx, n)
			ch <- result{n, h, err}
		}()
	}
	out := make(map[string]NodeHealth, len(nodes))
	for range nodes {
		r := <-ch
		if r.err != nil {
			out[r.name] = NodeHealth{Node: r.name, Status: "offline"}
		} else {
			out[r.name] = r.h
		}
	}
	return out
}

// PricesAll fetches all sector price data.
func (c *Client) PricesAll(ctx context.Context) (AllPrices, error) {
	var p AllPrices
	err := c.get(ctx, c.baseURL+"/api/v1/prices/all", &p)
	return p, err
}

// Prices fetches price data for one sector.
func (c *Client) Prices(ctx context.Context, sector string) ([]PricePoint, error) {
	var pts []PricePoint
	err := c.get(ctx, c.baseURL+"/api/v1/prices/"+sector, &pts)
	return pts, err
}

// News fetches news items from all sources.
func (c *Client) News(ctx context.Context) (NewsResponse, error) {
	var n NewsResponse
	err := c.get(ctx, c.baseURL+"/api/v1/news", &n)
	return n, err
}
