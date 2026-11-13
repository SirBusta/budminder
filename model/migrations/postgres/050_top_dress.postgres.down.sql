-- Remove the activity type again, unless it was used.
DELETE FROM activity
 WHERE name = 'Top Dress' AND icon = 'fa-trowel'
   AND NOT EXISTS (SELECT 1 FROM plant_activity pa WHERE pa.activity_id = activity.id);
