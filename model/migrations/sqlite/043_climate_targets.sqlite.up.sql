-- Climate targets (Settings > Grow setup): Low / Target / High per stage for
-- VPD (kPa), temperature (always stored in °C) and humidity (%), with separate
-- lights-on and lights-off values. phase '' is the whole stage; Flower also
-- keeps 'stretch', 'bloom' and 'ripen' rows so switching phases on and off
-- never loses values. Rows are seeded in Go (EnsureClimateTargets) from the
-- old plant_status.vpd_low/vpd_high, which stay only for that.
CREATE TABLE climate_target (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status_id INTEGER NOT NULL REFERENCES plant_status(id) ON DELETE CASCADE,
    phase TEXT NOT NULL DEFAULT '',
    measure TEXT NOT NULL,
    lights TEXT NOT NULL,
    low REAL,
    target REAL,
    high REAL
);
CREATE UNIQUE INDEX idx_climate_target_key ON climate_target(status_id, phase, measure, lights);

-- Edit zone: light schedule per stage (JSON {"seedling":["06:00","00:00"],...};
-- NULL = no schedule, lights-on targets all the time), the optional light check
-- sensor, the plant whose stage the zone follows when stages differ, and where
-- the zone's VPD comes from ('sensor', 'calc' from temp + humidity, NULL = none).
ALTER TABLE zones ADD COLUMN light_schedule TEXT;
ALTER TABLE zones ADD COLUMN light_sensor_id INTEGER REFERENCES sensors(id) ON DELETE SET NULL;
ALTER TABLE zones ADD COLUMN light_on_above REAL;
ALTER TABLE zones ADD COLUMN stage_plant_id INTEGER REFERENCES plant(id) ON DELETE SET NULL;
ALTER TABLE zones ADD COLUMN vpd_source TEXT;
ALTER TABLE zones ADD COLUMN vpd_sensor_id INTEGER REFERENCES sensors(id) ON DELETE SET NULL;

-- A leaf offset used to switch on the calculated VPD.
UPDATE zones SET vpd_source = 'calc' WHERE leaf_temp_offset IS NOT NULL;
