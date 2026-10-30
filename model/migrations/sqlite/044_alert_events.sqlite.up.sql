-- Environment alerts (Settings > Alerts > Recent alerts). One row per alert,
-- open until resolved_at is set. Zone and sensor names are kept so old rows
-- still read well after a rename or delete; message text is built when shown
-- or sent, in the reader's language. Temperatures are stored in °C. Left out
-- of backups and the Postgres move, like logs.
CREATE TABLE IF NOT EXISTS alert_event (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    zone_id INTEGER NOT NULL,
    zone_name TEXT NOT NULL DEFAULT '',
    sensor_id INTEGER,
    sensor_name TEXT NOT NULL DEFAULT '',
    measure TEXT NOT NULL DEFAULT '',
    side TEXT NOT NULL DEFAULT '',
    value REAL,
    limit_value REAL,
    detail TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP NOT NULL,
    notified_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    delivery TEXT NOT NULL DEFAULT '',
    delivery_error TEXT NOT NULL DEFAULT '',
    pending TEXT NOT NULL DEFAULT '',
    retry_at TIMESTAMP,
    emailed INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_event_open ON alert_event(kind, zone_id, measure, side) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_alert_event_started ON alert_event(started_at);
