// Package store is the single-server data layer for oilfield-solo.
//
// It replaces the multi-node etcd cluster with a local SQLite file, exposing the
// same method set the API handlers and scrape job previously used against etcd.
// The store is a generic key/value table so the JSON payloads and handler code
// carry over unchanged from the etcd design.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	db *sql.DB
}

// New opens (creating if needed) the SQLite database at path and applies the schema.
// WAL mode + a busy timeout keep it safe under the in-process scraper + API concurrency.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // single writer; avoids SQLITE_BUSY under WAL for this workload
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM kv WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (s *Store) Put(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kv (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) PutJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Put(ctx, key, string(b))
}

func (s *Store) GetJSON(ctx context.Context, key string, dest any) error {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM kv WHERE key = ?", key)
	return err
}

// GetWithPrefix returns all key/value pairs whose key starts with prefix.
func (s *Store) GetWithPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM kv WHERE key LIKE ? ESCAPE '\\'", escapeLike(prefix)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// HistorySample is one observation of a product's front/spot price.
type HistorySample struct {
	Sector    string
	Symbol    string
	Name      string
	Unit      string
	Price     float64
	ScrapedAt time.Time
}

// HistoryPoint is a single time-series datum returned to the API.
type HistoryPoint struct {
	Price     float64   `json:"price"`
	ScrapedAt time.Time `json:"scraped_at"`
}

// RecordHistory appends samples to price_history, skipping a sample when the last
// stored value for that (sector,symbol) is identical AND newer than minInterval —
// this keeps flat series compact while still capturing every price change.
func (s *Store) RecordHistory(ctx context.Context, samples []HistorySample, minInterval time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, smp := range samples {
		if smp.Symbol == "" || smp.Price <= 0 {
			continue
		}
		var lastPrice float64
		var lastAt string
		row := tx.QueryRowContext(ctx,
			"SELECT price, scraped_at FROM price_history WHERE sector=? AND symbol=? ORDER BY scraped_at DESC LIMIT 1",
			smp.Sector, smp.Symbol)
		switch err := row.Scan(&lastPrice, &lastAt); err {
		case nil:
			unchanged := lastPrice == smp.Price
			if unchanged {
				if t, perr := time.Parse(time.RFC3339, lastAt); perr == nil && smp.ScrapedAt.Sub(t) < minInterval {
					continue // unchanged and too soon — skip
				}
			}
		case sql.ErrNoRows:
			// first observation — always insert
		default:
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO price_history (sector, symbol, name, price, unit, scraped_at) VALUES (?, ?, ?, ?, ?, ?)",
			smp.Sector, smp.Symbol, smp.Name, smp.Price, smp.Unit, smp.ScrapedAt.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetHistory returns the price series for one product since the given time, ascending.
func (s *Store) GetHistory(ctx context.Context, sector, symbol string, since time.Time) ([]HistoryPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT price, scraped_at FROM price_history WHERE sector=? AND symbol=? AND scraped_at >= ? ORDER BY scraped_at ASC",
		sector, symbol, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoryPoint
	for rows.Next() {
		var p float64
		var at string
		if err := rows.Scan(&p, &at); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, at)
		out = append(out, HistoryPoint{Price: p, ScrapedAt: t})
	}
	return out, rows.Err()
}

// PruneHistory deletes samples older than the cutoff (retention window).
func (s *Store) PruneHistory(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM price_history WHERE scraped_at < ?", before.UTC().Format(time.RFC3339))
	return err
}

// IsHealthy reports whether the database is reachable.
func (s *Store) IsHealthy(ctx context.Context) bool {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var one int
	return s.db.QueryRowContext(tctx, "SELECT 1").Scan(&one) == nil
}

// escapeLike escapes LIKE wildcards so a literal prefix isn't treated as a pattern.
func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_':
			out = append(out, '\\', s[i])
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
