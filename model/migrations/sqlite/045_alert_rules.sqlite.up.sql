-- Sensor alerts (Settings > Alerts): any sensor at or past a chosen value.
-- The limit is in the sensor's own unit. Rows are deleted with their sensor in
-- code (no cascades: production SQLite doesn't enforce them). Unlike
-- alert_event, these are settings: they're in backups and the Postgres move.
CREATE TABLE IF NOT EXISTS alert_rule (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sensor_id INTEGER NOT NULL,
    side TEXT NOT NULL,
    limit_value REAL NOT NULL,
    hold_minutes INTEGER NOT NULL DEFAULT 10,
    enabled INTEGER NOT NULL DEFAULT 1
);
