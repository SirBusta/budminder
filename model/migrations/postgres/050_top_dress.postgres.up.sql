-- Top Dress becomes a standard activity type (editable, not locked), unless
-- an install already has its own (any name with "top" followed by "dress").
INSERT INTO activity (name, lock, is_watering, is_feeding, icon, color)
SELECT 'Top Dress', FALSE, FALSE, FALSE, 'fa-trowel', '#ca8a04'
 WHERE NOT EXISTS (SELECT 1 FROM activity WHERE lower(name) LIKE '%top%dress%');
