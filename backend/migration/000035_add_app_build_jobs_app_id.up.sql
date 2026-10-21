-- 000028 dropped app_build_jobs with the app_id column because the user-center
-- app concept was not delivered in that release. 000031 rebuilt the table for
-- the admin-only app builder and, on the premise that this deployment has no
-- user-center app concept, recreated it without app_id.
--
-- That premise is wrong: 000026 created apps and entitlements and both are live.
-- This migration reattaches app_id so self-service packaging can count builds
-- per app and lock the app after the free-tier quota is used.
--
-- app_id is nullable: admin-console builds are instance-wide and reference no
-- app. ADD COLUMN IF NOT EXISTS keeps 000035 a no-op for instances where the
-- column was created out of band.

ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS app_id uuid;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'app_build_jobs_app_id_fk'
    ) THEN
        ALTER TABLE app_build_jobs
            ADD CONSTRAINT app_build_jobs_app_id_fk
            FOREIGN KEY (app_id) REFERENCES apps(id);
    END IF;
END $$;

-- One job per app, so the frontend can show the app's own build history.
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_app_id_status
    ON app_build_jobs USING btree (app_id, status);

-- Self-service listing is always "my latest jobs first".
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_user_id_enqueued_at
    ON app_build_jobs USING btree (user_id, enqueued_at DESC);
