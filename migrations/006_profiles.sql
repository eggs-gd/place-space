-- Profiles let a shared install keep each person's watches apart. A watch
-- without a profile belongs to the install itself, as on the desktop.
CREATE TABLE profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX profiles_name ON profiles(name);

ALTER TABLE watches ADD COLUMN profile_id TEXT REFERENCES profiles(id);

-- Watch names follow the city, so two people may both watch Kyiv.
DROP INDEX watches_name;

CREATE UNIQUE INDEX watches_profile_name ON watches(ifnull(profile_id, ''), name);
