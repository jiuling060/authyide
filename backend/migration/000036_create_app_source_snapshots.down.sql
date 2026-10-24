-- Restore apps and app_build_jobs to the 000035 shape.

DROP INDEX IF EXISTS idx_app_source_snapshots_status;
DROP INDEX IF EXISTS idx_app_source_snapshots_task_id;
DROP INDEX IF EXISTS idx_app_source_snapshots_user_id_created_at;

DROP TABLE IF EXISTS app_source_snapshots;

ALTER TABLE app_build_jobs DROP COLUMN IF EXISTS source_task_id;
ALTER TABLE app_build_jobs DROP COLUMN IF EXISTS source_snapshot_id;
ALTER TABLE apps DROP COLUMN IF EXISTS source_snapshot_id;
