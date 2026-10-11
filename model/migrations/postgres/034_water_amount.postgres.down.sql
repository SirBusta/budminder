-- Remove the Water amount measurement unless amounts were recorded with it.
DELETE FROM activity_metric
 WHERE metric_id IN (SELECT m.id FROM metric m WHERE lower(m.name) = 'water amount'
                       AND NOT EXISTS (SELECT 1 FROM plant_measurements pm WHERE pm.metric_id = m.id));
DELETE FROM metric
 WHERE lower(name) = 'water amount'
   AND NOT EXISTS (SELECT 1 FROM plant_measurements pm WHERE pm.metric_id = metric.id);
