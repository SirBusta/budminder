ALTER TABLE zones DROP COLUMN vpd_sensor_id;
ALTER TABLE zones DROP COLUMN vpd_source;
ALTER TABLE zones DROP COLUMN stage_plant_id;
ALTER TABLE zones DROP COLUMN light_on_above;
ALTER TABLE zones DROP COLUMN light_sensor_id;
ALTER TABLE zones DROP COLUMN light_schedule;

DROP INDEX IF EXISTS idx_climate_target_key;
DROP TABLE IF EXISTS climate_target;
