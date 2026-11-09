CREATE TABLE areas (
    source_type TEXT NOT NULL,
    city_key TEXT NOT NULL,
    area_key TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    PRIMARY KEY (source_type, city_key, area_key)
);

CREATE TABLE area_scans (
    source_type TEXT NOT NULL,
    city_key TEXT NOT NULL,
    scanned_at TEXT NOT NULL,
    PRIMARY KEY (source_type, city_key)
);
