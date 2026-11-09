-- Alert notifications go out through every notification method that is on.
-- When some methods fail and others work, retry_methods lists the ones that
-- failed (comma-separated), so a retry doesn't send the same notice twice to
-- the ones that worked. '' means every method that is on.
ALTER TABLE alert_event ADD COLUMN retry_methods TEXT NOT NULL DEFAULT '';
