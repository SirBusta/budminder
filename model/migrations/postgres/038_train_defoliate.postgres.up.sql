-- Training and Defoliate become standard activity types (editable, not
-- locked), unless an install already has its own by a similar name.
INSERT INTO activity (name, lock, is_watering, is_feeding, icon, color)
SELECT 'Training', FALSE, FALSE, FALSE, 'fa-hand', '#e67e22'
 WHERE NOT EXISTS (SELECT 1 FROM activity WHERE lower(name) LIKE '%train%' OR lower(name) LIKE '%lst%');
INSERT INTO activity (name, lock, is_watering, is_feeding, icon, color)
SELECT 'Defoliate', FALSE, FALSE, FALSE, 'fa-scissors', '#9b59b6'
 WHERE NOT EXISTS (SELECT 1 FROM activity WHERE lower(name) LIKE '%defol%');
