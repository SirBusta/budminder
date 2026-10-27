-- Until this fix the 10-minute rollup rewrote each hour older than about a
-- day from only its last few minutes of readings. Rebuild every hour that
-- still has raw readings; hours whose readings were pruned keep the only
-- numbers there are.
INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
SELECT sensor_id, date_trunc('hour', create_dt), MIN(value), MAX(value), AVG(value), COUNT(*)
FROM sensor_data
GROUP BY sensor_id, date_trunc('hour', create_dt)
ON CONFLICT (sensor_id, bucket) DO UPDATE SET
    min_val      = EXCLUDED.min_val,
    max_val      = EXCLUDED.max_val,
    avg_val      = EXCLUDED.avg_val,
    sample_count = EXCLUDED.sample_count;
