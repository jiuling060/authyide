-- Recreate the placement column exactly as 000026 created it.
ALTER TABLE apps ADD COLUMN IF NOT EXISTS ad_placements jsonb DEFAULT '{}'::jsonb NOT NULL;

-- Recreate app_build_jobs as 000026 defined it plus the log_tail column that
-- 000027 added, so an old worker deployment can still insert full rows.
CREATE TABLE IF NOT EXISTS app_build_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    app_id uuid NOT NULL REFERENCES apps(id),
    user_id uuid NOT NULL REFERENCES users(id),
    platform character varying(32) NOT NULL,
    status character varying(32) DEFAULT 'queued'::character varying NOT NULL,
    artifact_key text,
    artifact_size bigint DEFAULT 0 NOT NULL,
    worker_id character varying(128),
    commit_ref character varying(255),
    log_tail text,
    error text,
    enqueued_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_app_build_jobs_app_id_created_at ON app_build_jobs USING btree (app_id, created_at);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_user_id ON app_build_jobs USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_status_enqueued_at ON app_build_jobs USING btree (status, enqueued_at);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_platform ON app_build_jobs USING btree (platform);
