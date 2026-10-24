-- 000031 rebuilt app_build_jobs without app_id on the premise that the
-- deployment has no user-center app concept; 000035 corrected that and let
-- self-service packaging count builds per app.
--
-- Self-service packaging still builds from the instance-wide template, so a
-- user cannot ship code they wrote in a session: the builder copies
-- /opt/authyide/src/mobile and nothing in the enqueue request carries files.
-- This migration adds the snapshot table that closes that gap, plus the two
-- nullable columns that point at it.
--
-- source_snapshot_id is nullable on both apps and app_build_jobs: null means
-- "build from the instance template", which stays the behavior for admin
-- console builds and for apps that never bound a snapshot.
--
-- Rows are never soft-deleted. app_build_jobs.source_snapshot_id keeps
-- pointing at them for provenance after an app unbinds, so history must stay
-- queryable. superseded is an audit flag on the snapshot itself.

CREATE TABLE IF NOT EXISTS app_source_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    task_id uuid NOT NULL REFERENCES tasks(id),
    object_key text NOT NULL,
    sha256 character varying(64) NOT NULL,
    size_bytes bigint NOT NULL,
    file_count integer NOT NULL,
    status character varying(32) NOT NULL DEFAULT 'uploading'::character varying,
    superseded boolean DEFAULT false NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);

-- One current snapshot per session is enforced in the application layer, not
-- here: the superseded flag is set in the same transaction as the promote.
CREATE INDEX IF NOT EXISTS idx_app_source_snapshots_user_id_created_at
    ON app_source_snapshots USING btree (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_source_snapshots_task_id
    ON app_source_snapshots USING btree (task_id);
CREATE INDEX IF NOT EXISTS idx_app_source_snapshots_status
    ON app_source_snapshots USING btree (status);

ALTER TABLE apps ADD COLUMN IF NOT EXISTS source_snapshot_id uuid;
ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS source_snapshot_id uuid;
ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS source_task_id uuid;
