-- The rolling-average trigger from 002 computed a window over EVERY reading
-- the sensor ever recorded, and replaced the one rolling_averages row once
-- per old reading, on every insert. Its cost grew with history and it held
-- the write lock while it ran. Same result (average of the 16 readings
-- before the new one), but only those 16 rows are read, through the
-- sensor_id index (rowid order = insert order).
DROP TRIGGER IF EXISTS update_rolling_avg;

CREATE TRIGGER update_rolling_avg
    AFTER INSERT ON sensor_data
BEGIN
    INSERT OR REPLACE INTO rolling_averages(sensor_id, avg_value, create_dt)
    SELECT NEW.sensor_id, AVG(value), NEW.create_dt
    FROM (
        SELECT value
        FROM sensor_data
        WHERE sensor_id = NEW.sensor_id AND id < NEW.id
        ORDER BY id DESC
        LIMIT 16
    );
END;
