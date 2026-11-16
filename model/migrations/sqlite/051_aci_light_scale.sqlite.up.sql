-- AC Infinity's light sensor (sensorType 12, sold with the CO2 sensor) reports
-- brightness 0-100% with sensorPrecision 2 (divide by 10), but every version
-- before 2.8 divided it by 100, so stored history is 10 times too small (76%
-- was saved as 7.6). Scale the history, hourly rollups, rolling average,
-- sensor alert limits and the light check's "on above" value. A sensor with
-- no unit, or the "on/off" an owner typed for lack of one, gets "%".
-- alert_event rows are history and keep their old numbers.
UPDATE sensor_data SET value = value * 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE sensor_data_hourly SET min_val = min_val * 10, max_val = max_val * 10, avg_val = avg_val * 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE rolling_averages SET avg_value = avg_value * 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE alert_rule SET limit_value = limit_value * 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE zones SET light_on_above = light_on_above * 10
 WHERE light_on_above IS NOT NULL
   AND light_sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE sensors SET unit = '%'
 WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12' AND (unit IS NULL OR TRIM(unit) IN ('', 'on/off'));
