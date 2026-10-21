-- Reverse of 000027. The apps table is dropped by 000026's down migration, so
-- this only needs to guard against a fresh database where 000027 was the last
-- step applied.
ALTER TABLE app_build_jobs DROP COLUMN IF EXISTS log_tail;
