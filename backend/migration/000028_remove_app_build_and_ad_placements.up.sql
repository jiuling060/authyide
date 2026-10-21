-- The user center records only one ad network and one app id per app. The
-- per-platform placement mapping is no longer delivered, so drop the column.
-- Keeping the column would leave dead JSON that nothing writes to.
ALTER TABLE apps DROP COLUMN IF EXISTS ad_placements;

-- App packaging is not delivered in this release. Drop the job table with its
-- indexes; the apps-level build counters stay because they are cheap and the
-- entitlement schema already references them.
DROP TABLE IF EXISTS app_build_jobs;
