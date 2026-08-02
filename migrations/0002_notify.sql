ALTER TABLE job_events ADD COLUMN notified_at INTEGER;
UPDATE job_events SET notified_at = at;
