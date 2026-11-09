CREATE TABLE exchange_rates (
    currency TEXT PRIMARY KEY,
    uah_rate REAL NOT NULL CHECK (uah_rate > 0),
    fetched_at TEXT NOT NULL
);

CREATE INDEX listing_observations_run ON listing_observations(run_id);
