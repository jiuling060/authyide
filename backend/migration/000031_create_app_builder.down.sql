DROP INDEX IF EXISTS idx_app_build_jobs_platform;
DROP INDEX IF EXISTS idx_app_build_jobs_created_at;
DROP INDEX IF EXISTS idx_app_build_jobs_user_id;
DROP INDEX IF EXISTS idx_app_build_jobs_status_enqueued_at;
DROP TABLE IF EXISTS app_build_jobs;

DROP INDEX IF EXISTS idx_app_branding_single;
DROP TABLE IF EXISTS app_branding;
