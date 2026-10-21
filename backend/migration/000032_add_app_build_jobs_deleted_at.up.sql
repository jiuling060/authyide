-- 000031 shipped app_build_jobs without the soft-delete column that
-- entx.SoftDeleteMixin2 contributes to the AppBuildJob schema. Every ent query
-- on this table appends "AND deleted_at IS NULL", so the column must exist or
-- the build worker's polling loop fails on every tick.
--
-- ADD COLUMN IF NOT EXISTS keeps 000032 a no-op for instances where 000031 has
-- already been re-created with the column included.

ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS deleted_at timestamp with time zone;
