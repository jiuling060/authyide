-- Per-app desktop branding: product name and bundle identifier of the Tauri
-- shell when a user-center app is packaged for desktop-linux.
--
-- One row per app instead of one row per user, because a single user owns many
-- apps and each app ships its own bundle id. The instance-wide desktop_branding
-- table stays as the admin-console fallback for builds with no app.
--
-- updater_endpoint is not stored: it is derived from the app id so every app
-- publishes into its own updater source instead of overwriting a shared
-- manifest. The icon reuses apps.icon_key, which the mobile and desktop
-- pipelines both read.
--
-- The unique index is partial so a deleted row frees the app id for re-insert,
-- matching the soft-delete hook that turns a delete into a deleted_at update.
CREATE TABLE app_desktop_branding (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    app_id            uuid NOT NULL,
    product_name      text NOT NULL DEFAULT '',
    bundle_identifier text NOT NULL DEFAULT '',
    created_at        timestamp with time zone DEFAULT now() NOT NULL,
    updated_at        timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at        timestamp with time zone
);

CREATE UNIQUE INDEX uq_app_desktop_branding_app
    ON app_desktop_branding USING btree (app_id) WHERE (deleted_at IS NULL);
