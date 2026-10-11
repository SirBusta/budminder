-- How much water was given: an optional "Water amount" measurement asked
-- for whenever a watering or feeding activity type is logged (Home page,
-- Plant page). Liters when Settings → Measurement units is metric, else
-- gallons; the unit can be changed in Settings → Measurements.
INSERT INTO metric (name, unit, lock)
SELECT 'Water amount',
       CASE WHEN (SELECT value FROM settings WHERE name = 'units') = 'metric' THEN 'L' ELSE 'gal' END,
       FALSE
 WHERE NOT EXISTS (SELECT 1 FROM metric WHERE lower(name) = 'water amount');

INSERT INTO activity_metric (activity_id, metric_id, required)
SELECT a.id, m.id, FALSE FROM activity a, metric m
 WHERE (a.is_watering = TRUE OR a.is_feeding = TRUE)
   AND lower(m.name) = 'water amount'
   AND NOT EXISTS (SELECT 1 FROM activity_metric am WHERE am.activity_id = a.id AND am.metric_id = m.id);
