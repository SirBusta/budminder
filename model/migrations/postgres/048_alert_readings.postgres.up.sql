-- Recent alerts show the reading when an alert started, the highest (or
-- lowest) reading while it was active, counting the 15 minutes before it
-- started, and each limit in use while it was active. value keeps being
-- overwritten (reminders, the end), so the starting reading needs its own
-- column. limits is a JSON list of {v, at, lights, stage, phase}: the reason
-- a limit changed is read from what differs between two entries. Temperatures
-- are in °C, sensor alerts in the sensor's own unit, like value.
ALTER TABLE alert_event ADD COLUMN start_value DOUBLE PRECISION;
ALTER TABLE alert_event ADD COLUMN peak_value DOUBLE PRECISION;
ALTER TABLE alert_event ADD COLUMN limits TEXT NOT NULL DEFAULT '';
