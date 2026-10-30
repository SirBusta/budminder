-- Sensor alerts (Settings > Alerts): any sensor at or past a chosen value.
-- The limit is in the sensor's own unit. Rows are deleted with their sensor in
-- code. Unlike alert_event, these are settings: they're in backups and the
-- Postgres move.
CREATE TABLE IF NOT EXISTS alert_rule (
    id SERIAL PRIMARY KEY,
    sensor_id INTEGER NOT NULL,
    side TEXT NOT NULL,
    limit_value DOUBLE PRECISION NOT NULL,
    hold_minutes INTEGER NOT NULL DEFAULT 10,
    enabled INTEGER NOT NULL DEFAULT 1
);
