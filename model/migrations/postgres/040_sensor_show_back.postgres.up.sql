-- Migration 013 dropped sensors.show on PostgreSQL only (SQLite can't drop
-- columns, so it kept it). Manage sensors reads and writes it again as the
-- "Environment page" tick, so put it back. visibility still decides hiding.
ALTER TABLE sensors ADD COLUMN IF NOT EXISTS show BOOLEAN NOT NULL DEFAULT TRUE;
