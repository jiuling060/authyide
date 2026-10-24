-- Desktop client branding. A second singleton table, deliberately separate
-- from app_branding: the desktop and mobile clients evolve their identity
-- (product name, bundle id, updater endpoint) independently, so sharing one
-- row would couple two unrelated release trains.
CREATE TABLE desktop_branding (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    singleton_key     text NOT NULL DEFAULT 'authyide',
    product_name      text NOT NULL DEFAULT '',
    bundle_identifier text NOT NULL DEFAULT '',
    updater_endpoint  text NOT NULL DEFAULT '',
    icon_key          text NOT NULL DEFAULT '',
    created_at        timestamp with time zone DEFAULT now() NOT NULL,
    updated_at        timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at        timestamp with time zone
);

CREATE UNIQUE INDEX idx_desktop_branding_single
    ON desktop_branding USING btree (singleton_key) WHERE (deleted_at IS NULL);
