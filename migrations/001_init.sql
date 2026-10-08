CREATE TABLE sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    configuration TEXT NOT NULL,
    status TEXT NOT NULL,
    last_success_at TEXT,
    last_error_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE watches (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    query_json TEXT NOT NULL,
    filters_json TEXT NOT NULL,
    poll_interval_seconds INTEGER NOT NULL CHECK (poll_interval_seconds > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX watches_name ON watches(name);

CREATE TABLE watch_sources (
    watch_id TEXT NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id),
    PRIMARY KEY (watch_id, source_id)
);

CREATE TABLE listings (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    price_amount INTEGER,
    currency TEXT NOT NULL,
    location_name TEXT NOT NULL,
    address TEXT NOT NULL,
    latitude REAL,
    longitude REAL,
    rooms INTEGER,
    area REAL,
    floor INTEGER,
    total_floors INTEGER,
    images_json TEXT NOT NULL,
    published_at TEXT,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    raw_data TEXT NOT NULL,
    UNIQUE (source, external_id)
);

CREATE INDEX listings_source_url ON listings(source, url);

CREATE TABLE poll_runs (
    id TEXT PRIMARY KEY,
    watch_id TEXT NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    received INTEGER NOT NULL,
    new_count INTEGER NOT NULL,
    changed INTEGER NOT NULL,
    matched INTEGER NOT NULL,
    rejected INTEGER NOT NULL,
    duplicates INTEGER NOT NULL,
    duplicate_links INTEGER NOT NULL,
    failed INTEGER NOT NULL,
    errors_json TEXT NOT NULL
);

CREATE INDEX poll_runs_watch_started ON poll_runs(watch_id, started_at);
CREATE INDEX poll_runs_source_started ON poll_runs(source_id, started_at);

CREATE TABLE listing_observations (
    id TEXT PRIMARY KEY,
    listing_id TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES poll_runs(id) ON DELETE CASCADE,
    seen_at TEXT NOT NULL,
    price_amount INTEGER,
    currency TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    images_json TEXT NOT NULL,
    present INTEGER NOT NULL CHECK (present IN (0, 1)),
    content_hash TEXT NOT NULL
);

CREATE INDEX listing_observations_listing_seen ON listing_observations(listing_id, seen_at);

CREATE TABLE matches (
    watch_id TEXT NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
    listing_id TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    reasons_json TEXT NOT NULL,
    first_matched_at TEXT,
    last_matched_at TEXT,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (watch_id, listing_id)
);

-- Exact duplicates stay as separate listings. The link can be dropped later
-- without destroying either side.
CREATE TABLE duplicate_links (
    listing_a TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    listing_b TEXT NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (listing_a, listing_b),
    CHECK (listing_a < listing_b)
);
