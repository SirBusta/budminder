-- Until this fix the 10-minute rollup rewrote each hour older than about a
-- day from only its last few minutes of readings. Rebuild every hour that
-- still has raw readings; hours whose readings were pruned keep the only
-- numbers there are.
INSERT OR REPLACE INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
SELECT sensor_id, strftime('%Y-%m-%d %H:00:00', create_dt), MIN(value), MAX(value), AVG(value), COUNT(*)
FROM sensor_data
GROUP BY sensor_id, strftime('%Y-%m-%d %H:00:00', create_dt);
