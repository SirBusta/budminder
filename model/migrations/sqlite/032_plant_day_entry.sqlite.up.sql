-- Plant notebook: one entry per plant per day. Ticked activity types stay
-- one plant_activity row each (with their own time); the day's single note
-- lives here. Rows logged before this table existed keep their own notes and
-- are shown joined until that day is edited. entry_date is YYYY-MM-DD.
CREATE TABLE plant_day_entry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plant_id INTEGER NOT NULL REFERENCES plant(id) ON DELETE CASCADE,
    entry_date TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (plant_id, entry_date)
);

-- Icon (Font Awesome class) and color per activity type, shown in the notebook.
ALTER TABLE activity ADD COLUMN icon TEXT NOT NULL DEFAULT 'fa-tag';
ALTER TABLE activity ADD COLUMN color TEXT NOT NULL DEFAULT '#64748b';

UPDATE activity SET icon = 'fa-droplet', color = '#2f80ed' WHERE name = 'Water';
UPDATE activity SET icon = 'fa-flask', color = '#27ae60' WHERE name = 'Feed';
UPDATE activity SET icon = 'fa-note-sticky', color = '#64748b' WHERE name = 'Note';
UPDATE activity SET icon = 'fa-hand', color = '#e67e22'
 WHERE icon = 'fa-tag' AND (lower(name) LIKE '%train%' OR lower(name) LIKE '%topp%' OR lower(name) LIKE '%lst%');
UPDATE activity SET icon = 'fa-scissors', color = '#9b59b6'
 WHERE icon = 'fa-tag' AND (lower(name) LIKE '%defol%' OR lower(name) LIKE '%prun%');

-- Height becomes an activity type that records the Height measurement.
INSERT INTO activity (name, lock, icon, color)
SELECT 'Height', TRUE, 'fa-ruler', '#ca8a04'
 WHERE NOT EXISTS (SELECT 1 FROM activity WHERE lower(name) = 'height');
UPDATE activity SET icon = 'fa-ruler', color = '#ca8a04' WHERE lower(name) = 'height' AND icon = 'fa-tag';
INSERT INTO activity_metric (activity_id, metric_id, required)
SELECT a.id, m.id, TRUE FROM activity a, metric m
 WHERE lower(a.name) = 'height' AND lower(m.name) = 'height'
   AND NOT EXISTS (SELECT 1 FROM activity_metric am WHERE am.activity_id = a.id AND am.metric_id = m.id);
