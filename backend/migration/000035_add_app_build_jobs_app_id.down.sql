-- Restore app_build_jobs to the 000031 shape.

ALTER TABLE app_build_jobs DROP CONSTRAINT IF EXISTS app_build_jobs_app_id_fk;

DROP INDEX IF EXISTS idx_app_build_jobs_app_id_status;
DROP INDEX IF EXISTS idx_app_build_jobs_user_id_enqueued_at;

ALTER TABLE app_build_jobs DROP COLUMN IF EXISTS app_id;
