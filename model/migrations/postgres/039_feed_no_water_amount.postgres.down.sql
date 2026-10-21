-- Feeding asks for the water amount again (as migration 034 set it up).
INSERT INTO activity_metric (activity_id, metric_id, required)
SELECT a.id, m.id, FALSE FROM activity a, metric m
 WHERE a.is_feeding = TRUE AND a.is_watering = FALSE
   AND lower(m.name) = 'water amount'
   AND NOT EXISTS (SELECT 1 FROM activity_metric am WHERE am.activity_id = a.id AND am.metric_id = m.id);
