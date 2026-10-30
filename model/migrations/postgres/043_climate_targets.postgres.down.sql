ALTER TABLE zones DROP COLUMN IF EXISTS vpd_sensor_id;
ALTER TABLE zones DROP COLUMN IF EXISTS vpd_source;
ALTER TABLE zones DROP COLUMN IF EXISTS stage_plant_id;
ALTER TABLE zones DROP COLUMN IF EXISTS light_on_above;
ALTER TABLE zones DROP COLUMN IF EXISTS light_sensor_id;
ALTER TABLE zones DROP COLUMN IF EXISTS light_schedule;

DROP INDEX IF EXISTS idx_climate_target_key;
DROP TABLE IF EXISTS climate_target;
