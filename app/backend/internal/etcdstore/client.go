package etcdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Client struct {
	etcd *clientv3.Client
}

func New(endpoints []string) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd connect: %w", err)
	}
	return &Client{etcd: cli}, nil
}

func (c *Client) Close() { c.etcd.Close() }

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	resp, err := c.etcd.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if len(resp.Kvs) == 0 {
		return "", nil
	}
	return string(resp.Kvs[0].Value), nil
}

func (c *Client) Put(ctx context.Context, key, value string) error {
	_, err := c.etcd.Put(ctx, key, value)
	return err
}

func (c *Client) PutJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.etcd.Put(ctx, key, string(b))
	return err
}

func (c *Client) GetJSON(ctx context.Context, key string, dest any) error {
	raw, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.etcd.Delete(ctx, key)
	return err
}

// AcquireLock atomically acquires a lock using a lease-backed CAS transaction.
// Returns (leaseID, true, nil) if acquired; (0, false, nil) if held by another node.
//
// ⚠️ LEASE REQUIRED: the lock MUST use a lease so it auto-releases if the process
// crashes. A plain Put with no lease creates a permanent lock (Agent Failure Case #1).
func (c *Client) AcquireLock(ctx context.Context, key, holder string, ttlSeconds int64) (clientv3.LeaseID, bool, error) {
	lease, err := c.etcd.Grant(ctx, ttlSeconds)
	if err != nil {
		return 0, false, fmt.Errorf("grant lease (ttl=%ds): %w", ttlSeconds, err)
	}

	resp, err := c.etcd.Txn(ctx).
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, holder, clientv3.WithLease(lease.ID))).
		Commit()
	if err != nil {
		c.etcd.Revoke(ctx, lease.ID)
		return 0, false, fmt.Errorf("lock txn: %w", err)
	}
	if !resp.Succeeded {
		c.etcd.Revoke(ctx, lease.ID)
		return 0, false, nil
	}
	return lease.ID, true, nil
}

func (c *Client) RevokeLease(ctx context.Context, id clientv3.LeaseID) {
	c.etcd.Revoke(ctx, id)
}

func (c *Client) IsHealthy(ctx context.Context) bool {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := c.etcd.Get(tctx, "/oilfield/config/scrape_interval")
	return err == nil
}

// GetWithPrefix returns all keys under a prefix as a map[key]value.
func (c *Client) GetWithPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	resp, err := c.etcd.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		m[string(kv.Key)] = string(kv.Value)
	}
	return m, nil
}
