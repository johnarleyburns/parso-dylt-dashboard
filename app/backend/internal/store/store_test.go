package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
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

func TestHistoryRecordDedupAndGet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mk := func(price float64, min int) []HistorySample {
		return []HistorySample{{Sector: "crude", Symbol: "WTI", Name: "WTI", Unit: "USD/bbl", Price: price, ScrapedAt: base.Add(time.Duration(min) * time.Minute)}}
	}
	// first sample always inserts
	if err := st.RecordHistory(ctx, mk(80, 0), time.Hour); err != nil {
		t.Fatal(err)
	}
	// same price, 5 min later, within minInterval → skipped
	st.RecordHistory(ctx, mk(80, 5), time.Hour)
	// price changed 10 min later → inserted even within interval
	st.RecordHistory(ctx, mk(81, 10), time.Hour)
	// same price but > minInterval later → inserted
	st.RecordHistory(ctx, mk(81, 200), time.Hour)

	pts, err := st.GetHistory(ctx, "crude", "WTI", base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 3 {
		t.Fatalf("expected 3 history points, got %d", len(pts))
	}
	if pts[0].Price != 80 || pts[2].Price != 81 {
		t.Errorf("unexpected series: %+v", pts)
	}
}

func TestHistoryPrune(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	st.RecordHistory(ctx, []HistorySample{{Sector: "crude", Symbol: "X", Name: "X", Unit: "u", Price: 1, ScrapedAt: old}}, time.Nanosecond)
	st.RecordHistory(ctx, []HistorySample{{Sector: "crude", Symbol: "X", Name: "X", Unit: "u", Price: 2, ScrapedAt: recent}}, time.Nanosecond)

	if err := st.PruneHistory(ctx, time.Now().Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	pts, _ := st.GetHistory(ctx, "crude", "X", time.Now().Add(-200*24*time.Hour))
	if len(pts) != 1 || pts[0].Price != 2 {
		t.Errorf("prune failed, got %+v", pts)
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
