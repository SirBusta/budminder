-- Photo compression (Settings > Photos): which quality a plant photo was
-- checked at and what happened. NULL = never checked (an untouched upload).
-- 'compressed' photos are never compressed again; 'kept' photos are still
-- originals (compressing saved less than 10%, or BudMinder can't re-encode
-- them).
ALTER TABLE plant_images ADD COLUMN IF NOT EXISTS compress_quality INTEGER;
ALTER TABLE plant_images ADD COLUMN IF NOT EXISTS compress_result TEXT;
