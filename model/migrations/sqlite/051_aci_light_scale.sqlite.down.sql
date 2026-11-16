-- Units stay as they are: the up migration can't know what each one was.
UPDATE sensor_data SET value = value / 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE sensor_data_hourly SET min_val = min_val / 10, max_val = max_val / 10, avg_val = avg_val / 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE rolling_averages SET avg_value = avg_value / 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE alert_rule SET limit_value = limit_value / 10
 WHERE sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
UPDATE zones SET light_on_above = light_on_above / 10
 WHERE light_on_above IS NOT NULL
   AND light_sensor_id IN (SELECT id FROM sensors WHERE source = 'acinfinity' AND type LIKE 'ACI.%.12');
