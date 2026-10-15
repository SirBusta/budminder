-- AC Infinity's CO2 sensor (sensorType 11) reports plain ppm, but every
-- version before 1.0.1 divided it by 100 like the x100 temperature, humidity
-- and VPD values, so stored CO2 history is 100 times too small (770 ppm was
-- saved as 7.7). Scale the history, hourly rollups and rolling average back.
UPDATE sensor_data SET value = value * 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
UPDATE sensor_data_hourly SET min_val = min_val * 100, max_val = max_val * 100, avg_val = avg_val * 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
UPDATE rolling_averages SET avg_value = avg_value * 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
