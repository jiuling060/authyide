-- 000026 was applied to some instances before app_build_jobs gained log_tail,
-- and golang-migrate only records versions it has already run, so those tables
-- would stay missing the column. ADD COLUMN IF NOT EXISTS is a no-op for
-- instances where 000026 already created the full table.
ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS log_tail text;
