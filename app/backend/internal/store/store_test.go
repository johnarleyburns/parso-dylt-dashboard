package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestPutGet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if v, _ := st.Get(ctx, "missing"); v != "" {
		t.Errorf("missing key: got %q, want empty", v)
	}
	if err := st.Put(ctx, "/a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(ctx, "/a", "2"); err != nil { // upsert
		t.Fatal(err)
	}
	if v, _ := st.Get(ctx, "/a"); v != "2" {
		t.Errorf("upsert: got %q, want 2", v)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	type row struct {
		Name  string `json:"name"`
		Price int    `json:"price"`
	}
	in := []row{{"WTI", 82}, {"Brent", 85}}
	if err := st.PutJSON(ctx, "/prices", in); err != nil {
		t.Fatal(err)
	}
	var out []row
	if err := st.GetJSON(ctx, "/prices", &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Name != "WTI" || out[1].Price != 85 {
		t.Errorf("roundtrip mismatch: %+v", out)
	}
}

func TestGetWithPrefix(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	st.Put(ctx, "/oilfield/news/eia/items", "a")
	st.Put(ctx, "/oilfield/news/iea/items", "b")
	st.Put(ctx, "/oilfield/prices/crude/latest", "c")

	news, err := st.GetWithPrefix(ctx, "/oilfield/news/")
	if err != nil {
		t.Fatal(err)
	}
	if len(news) != 2 {
		t.Errorf("prefix scan: got %d keys, want 2 (%v)", len(news), news)
	}
}

func TestDeleteAndHealth(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if !st.IsHealthy(ctx) {
		t.Error("expected healthy store")
	}
	st.Put(ctx, "/k", "v")
	if err := st.Delete(ctx, "/k"); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.Get(ctx, "/k"); v != "" {
		t.Errorf("after delete: got %q, want empty", v)
	}
}
