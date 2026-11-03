-- The alerts rebuild for 2.0. Recent alerts gets Clear and Restore. The Mode
-- and timing settings are gone: alerts always list, and the Email switch decides
-- what's sent. Climate targets are reseeded in Go (EnsureClimateTargets) with
-- the new defaults: this table came with 2.0, so no release has filled it. The
-- lights-off and Flower phase switches now hold one choice per measure.
ALTER TABLE alert_event ADD COLUMN cleared INTEGER NOT NULL DEFAULT 0;
DELETE FROM alert_event WHERE kind IN ('safety', 'drift');
UPDATE alert_event SET delivery = 'off' WHERE delivery = 'dry_run';
DELETE FROM climate_target;
DELETE FROM settings WHERE name IN ('alert_climate_mode', 'alert_safety_readings', 'alert_window_minutes',
    'alert_drift_hours', 'alert_pause_minutes', 'alert_repeat_critical_minutes', 'alert_repeat_warning_hours',
    'climate_split_lights', 'climate_flower_phases', 'climate_temp_by_stage', 'climate_rh_by_stage',
    'climate_temp_limits', 'climate_rh_limits');
