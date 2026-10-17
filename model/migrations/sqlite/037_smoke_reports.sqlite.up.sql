-- Smoke reports: tasting notes for flower you grew (plant_id set) or bought
-- (source 'bought'). A report keeps its own strain/breeder name, dates and
-- grow info, so it outlives a deleted plant or strain (links go NULL).
-- Dates are YYYY-MM-DD text; scores are 1-10 (NULL = not scored); the overall
-- score is worked out when shown, from the weights in settings.
CREATE TABLE smoke_report (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plant_id INTEGER REFERENCES plant(id) ON DELETE SET NULL,
    strain_id INTEGER REFERENCES strain(id) ON DELETE SET NULL,
    source TEXT NOT NULL DEFAULT 'grown',
    strain_name TEXT NOT NULL DEFAULT '',
    breeder_name TEXT NOT NULL DEFAULT '',
    smoked_on TEXT NOT NULL DEFAULT '',
    thc TEXT NOT NULL DEFAULT '',
    cbd TEXT NOT NULL DEFAULT '',
    harvested_on TEXT NOT NULL DEFAULT '',
    grow_space TEXT NOT NULL DEFAULT '',
    lights TEXT NOT NULL DEFAULT '',
    container TEXT NOT NULL DEFAULT '',
    medium TEXT NOT NULL DEFAULT '',
    nutrients TEXT NOT NULL DEFAULT '',
    days_to_harvest INTEGER,
    dry_days INTEGER,
    cure_days INTEGER,
    dispensary TEXT NOT NULL DEFAULT '',
    price REAL,
    amount_g REAL,
    batch TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    again TEXT NOT NULL DEFAULT '',
    score_bud INTEGER,
    score_taste INTEGER,
    score_smell INTEGER,
    score_effects INTEGER,
    notes TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_smoke_report_plant_id ON smoke_report(plant_id);
CREATE INDEX idx_smoke_report_strain_id ON smoke_report(strain_id);

-- Picked choices: kind is effect | side | smell | taste | bud; name is a
-- choice-list key (e.g. "relaxed"), or the user's own words when custom = 1.
CREATE TABLE smoke_report_tag (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id INTEGER NOT NULL REFERENCES smoke_report(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    custom INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_smoke_report_tag_report_id ON smoke_report_tag(report_id);

-- Photos, in order (the first is the main one). path is under uploads/smoke/.
CREATE TABLE smoke_report_photo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id INTEGER NOT NULL REFERENCES smoke_report(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_smoke_report_photo_report_id ON smoke_report_photo(report_id);
