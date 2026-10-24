-- Build artifacts: one row per (job, channel).
--
-- A desktop job emits three rows (deb / rpm / appimage); a mobile job emits a
-- single apk row, so download and publish resolve through this table for both
-- platforms instead of special-casing the mobile path.
--
-- The unique index is partial: the soft-delete hook turns a delete into an
-- update that sets deleted_at, so a live-only index is what lets a retried job
-- re-insert the same (job_id, channel) pair. Mirrors idx_app_branding_single.
CREATE TABLE app_build_artifacts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    job_id          uuid NOT NULL REFERENCES app_build_jobs(id),
    platform        character varying(32) NOT NULL,
    channel         character varying(32) NOT NULL,
    file_name       text NOT NULL,
    artifact_path   text NOT NULL,
    artifact_sha256 text NOT NULL,
    artifact_size   bigint NOT NULL,
    created_at      timestamp with time zone DEFAULT now() NOT NULL,
    updated_at      timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at      timestamp with time zone
);

CREATE UNIQUE INDEX uq_app_build_artifacts_job_channel
    ON app_build_artifacts USING btree (job_id, channel) WHERE (deleted_at IS NULL);

CREATE INDEX idx_app_build_artifacts_platform_channel
    ON app_build_artifacts USING btree (platform, channel);
