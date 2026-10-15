UPDATE sensor_data SET value = value / 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
UPDATE sensor_data_hourly SET min_val = min_val / 100, max_val = max_val / 100, avg_val = avg_val / 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
UPDATE rolling_averages SET avg_value = avg_value / 100
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.11');
