CREATE TABLE IF NOT EXISTS kv (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_kv_prefix ON kv(key);

-- Time series of the front/spot price per product, appended each scrape cycle
-- (deduped: a row is written only when the price changes or enough time passes).
CREATE TABLE IF NOT EXISTS price_history (
  sector     TEXT NOT NULL,
  symbol     TEXT NOT NULL,
  name       TEXT NOT NULL,
  price      REAL NOT NULL,
  unit       TEXT NOT NULL,
  scraped_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hist ON price_history(sector, symbol, scraped_at);
