-- Mobile app builder. Restores app_build_jobs dropped by 000028 without the
-- apps(id) foreign key: this deployment has no user-center app concept, so a
-- job only records who asked for it.
--
-- app_branding is a single-row table (the current deployment's mobile app
-- branding). The partial unique index on singleton_key enforces that exactly
-- one live row exists while still allowing a deleted row to coexist.

CREATE TABLE IF NOT EXISTS app_branding (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    singleton_key text NOT NULL DEFAULT 'authyide',
    name text NOT NULL DEFAULT '',
    version text NOT NULL DEFAULT '',
    version_code integer NOT NULL DEFAULT 1,
    server_url text NOT NULL DEFAULT '',
    updates_server text NOT NULL DEFAULT '',
    logo_light_key text NOT NULL DEFAULT '',
    logo_dark_key text NOT NULL DEFAULT '',
    icon_key text NOT NULL DEFAULT '',
    icon_dark_key text NOT NULL DEFAULT '',
    adaptive_icon_key text NOT NULL DEFAULT '',
    splash_key text NOT NULL DEFAULT '',
    splash_dark_key text NOT NULL DEFAULT '',
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_branding_single
    ON app_branding USING btree (singleton_key)
    WHERE (deleted_at IS NULL);

-- AppBuildJob: one row per packaging request.
CREATE TABLE IF NOT EXISTS app_build_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    platform character varying(32) NOT NULL,
    target_version text NOT NULL,
    version_code integer NOT NULL,
    status character varying(32) DEFAULT 'queued'::character varying NOT NULL,
    current_step character varying(64),
    worker_id character varying(128),
    log_path text,
    log_tail text,
    error text,
    exit_code integer,
    artifact_path text,
    artifact_sha256 text,
    artifact_size bigint DEFAULT 0 NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    published_at timestamp with time zone,
    enqueued_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE INDEX IF NOT EXISTS idx_app_build_jobs_status_enqueued_at
    ON app_build_jobs USING btree (status, enqueued_at);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_user_id
    ON app_build_jobs USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_created_at
    ON app_build_jobs USING btree (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_build_jobs_platform
    ON app_build_jobs USING btree (platform);
