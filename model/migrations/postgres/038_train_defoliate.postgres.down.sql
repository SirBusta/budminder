-- Remove the two activity types again, unless they were used.
DELETE FROM activity
 WHERE name IN ('Training', 'Defoliate') AND icon IN ('fa-hand', 'fa-scissors')
   AND NOT EXISTS (SELECT 1 FROM plant_activity pa WHERE pa.activity_id = activity.id);
