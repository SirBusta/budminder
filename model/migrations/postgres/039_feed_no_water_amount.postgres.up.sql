-- Feeding no longer asks for a water amount: the feed's details go in the
-- day's note. An activity that is both watering and feeding keeps it, and
-- amounts already recorded stay (the notebook still shows and edits them).
DELETE FROM activity_metric
 WHERE metric_id IN (SELECT id FROM metric WHERE lower(name) = 'water amount')
   AND activity_id IN (SELECT id FROM activity WHERE is_feeding = TRUE AND is_watering = FALSE);
