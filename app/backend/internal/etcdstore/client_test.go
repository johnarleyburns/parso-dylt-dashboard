package etcdstore_test

import (
	"context"
	"testing"
	"time"

	"oilfield/internal/etcdstore"
)

// These tests require a live etcd instance at 127.0.0.1:2379.
// They are skipped automatically when etcd is not reachable.
// Run with: ETCD_INTEGRATION=1 go test ./internal/etcdstore/...

const testEndpoint = "http://127.0.0.1:2379"

func newTestClient(t *testing.T) *etcdstore.Client {
	t.Helper()
	c, err := etcdstore.New([]string{testEndpoint})
	if err != nil {
		t.Skipf("etcd not available at %s — skipping integration test: %v", testEndpoint, err)
	}
	if !c.IsHealthy(context.Background()) {
		c.Close()
		t.Skipf("etcd not healthy at %s — skipping integration test", testEndpoint)
	}
	return c
}

func TestGetPut_RoundTrip(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx := context.Background()
	key := "/test/oilfield/getput"
	defer c.Delete(ctx, key)

	if err := c.Put(ctx, key, "hello"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestGetJSON_RoundTrip(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx := context.Background()
	key := "/test/oilfield/getjson"
	defer c.Delete(ctx, key)

	type payload struct {
		Symbol string  `json:"symbol"`
		Price  float64 `json:"price"`
	}
	in := payload{Symbol: "CL", Price: 82.45}
	if err := c.PutJSON(ctx, key, in); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	var out payload
	if err := c.GetJSON(ctx, key, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out != in {
		t.Errorf("got %+v, want %+v", out, in)
	}
}

func TestGet_MissingKeyReturnsEmpty(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	got, err := c.Get(context.Background(), "/test/oilfield/does-not-exist")
	if err != nil {
		t.Fatalf("Get missing key: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestAcquireLock_MutualExclusion(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx := context.Background()
	lockKey := "/test/oilfield/lock"
	defer c.Delete(ctx, lockKey)

	// First acquisition should succeed
	id1, acquired, err := c.AcquireLock(ctx, lockKey, "n1", 30)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected first acquisition to succeed")
	}
	defer c.RevokeLease(ctx, id1)

	// Second acquisition on same key must fail
	_, acquired2, err := c.AcquireLock(ctx, lockKey, "n2", 30)
	if err != nil {
		t.Fatalf("second AcquireLock: %v", err)
	}
	if acquired2 {
		t.Error("expected second acquisition to fail while first lease is active")
	}
}

func TestAcquireLock_AutoReleasesOnRevoke(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx := context.Background()
	lockKey := "/test/oilfield/lock-release"
	defer c.Delete(ctx, lockKey)

	id, _, err := c.AcquireLock(ctx, lockKey, "n1", 30)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	c.RevokeLease(ctx, id)

	// After revoke the key should be gone
	time.Sleep(200 * time.Millisecond)
	val, _ := c.Get(ctx, lockKey)
	if val != "" {
		t.Errorf("expected key to be deleted after lease revoke, got %q", val)
	}
}

func TestGetWithPrefix(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx := context.Background()
	prefix := "/test/oilfield/prefix/"
	keys := []string{prefix + "a", prefix + "b", prefix + "c"}
	for _, k := range keys {
		c.Put(ctx, k, "v")
		defer c.Delete(ctx, k)
	}

	result, err := c.GetWithPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("GetWithPrefix: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(result), result)
	}
}
