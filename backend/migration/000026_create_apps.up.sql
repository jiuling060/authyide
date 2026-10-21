-- Entitlements: one row per user, describing build quota, editability and the
-- optional desktop license. Must be created before apps because apps reference
-- it only through a plain column in the migration, but the order keeps the
-- FK set consistent with the user -> entitlement -> app ownership chain.
CREATE TABLE IF NOT EXISTS entitlements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    tier character varying(16) DEFAULT 'free'::character varying NOT NULL,
    free_builds_used integer DEFAULT 0 NOT NULL,
    desktop_licensed boolean DEFAULT false NOT NULL,
    pro_expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT entitlements_tier_check CHECK (((tier)::text = ANY ((ARRAY['free'::character varying, 'pro'::character varying])::text[])))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlements_user_id ON entitlements USING btree (user_id);

CREATE INDEX IF NOT EXISTS idx_entitlements_tier ON entitlements USING btree (tier);

-- Apps: a cross-platform application authored in the user center.
CREATE TABLE IF NOT EXISTS apps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    name character varying(128) NOT NULL,
    package_id character varying(255) NOT NULL,
    icon_key text,
    ad_network character varying(32) DEFAULT 'none'::character varying NOT NULL,
    ad_app_id text,
    ad_placements jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled_platforms jsonb DEFAULT '[]'::jsonb NOT NULL,
    build_count integer DEFAULT 0 NOT NULL,
    locked boolean DEFAULT false NOT NULL,
    status character varying(32) DEFAULT 'draft'::character varying NOT NULL,
    last_built_at timestamp with time zone,
    last_build_platform character varying(32),
    last_build_artifact_key text,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE INDEX IF NOT EXISTS idx_apps_user_id ON apps USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_apps_status ON apps USING btree (status);
CREATE INDEX IF NOT EXISTS idx_apps_user_build_count ON apps USING btree (user_id, build_count);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_user_package_id ON apps USING btree (user_id, package_id)
    WHERE (deleted_at IS NULL);

CREATE INDEX IF NOT EXISTS idx_apps_created_at ON apps USING btree (created_at);

-- AppBuildJob: one row per packaging request.
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
