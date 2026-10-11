-- The Height activity type (and its metric link) is left in place: it may
-- have been logged against, and it's an ordinary activity without this table.
DROP TABLE plant_day_entry;
ALTER TABLE activity DROP COLUMN color;
ALTER TABLE activity DROP COLUMN icon;
