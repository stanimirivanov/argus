-- Expand immutable browser attempts for explicit staged protocol v2.
-- NULL stage/provenance means legacy v1. V2 requires every correlation field.
-- No backfill or row rewrite: add nullable columns, then validate a conditional
-- constraint under ALTER TABLE locks with a header-table scan. Apply before
-- enabling v2 writers/readers; rollback disables opt-in and retains evidence.
ALTER TABLE argus_catalog.browser_execution_attempts
    ADD COLUMN schedule_sha256 TEXT,
    ADD COLUMN episode_id TEXT,
    ADD COLUMN group_key TEXT,
    ADD COLUMN execution_mode TEXT,
    ADD COLUMN discovery_sha256 TEXT,
    ADD COLUMN execution_stage TEXT,
    DROP CONSTRAINT ck_browser_attempt_version,
    ADD CONSTRAINT ck_browser_attempt_staged_envelope CHECK (
        (
            attempt_api_version = 'argus.dev/browser-execution-attempt/v1'
            AND schedule_sha256 IS NULL AND episode_id IS NULL AND group_key IS NULL
            AND execution_mode IS NULL AND discovery_sha256 IS NULL AND execution_stage IS NULL
        ) OR (
            attempt_api_version = 'argus.dev/browser-execution-attempt/v2'
            AND schedule_sha256 IS NOT NULL AND schedule_sha256 ~ '^[0-9a-f]{64}$'
            AND discovery_sha256 IS NOT NULL AND discovery_sha256 ~ '^[0-9a-f]{64}$'
            AND episode_id IS NOT NULL AND episode_id ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND group_key IS NOT NULL AND group_key ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND execution_mode IS NOT NULL AND execution_stage IS NOT NULL AND (
                (execution_mode = 'shadow' AND execution_stage IN ('relevant-now', 'shadow-control'))
                OR (execution_mode = 'staged' AND execution_stage IN ('relevant-now', 'remaining-later'))
            )
        )
    );

COMMENT ON COLUMN argus_catalog.browser_execution_attempts.discovery_sha256 IS
    'SHA-256 of complete independent browser-discovery/v1 file bytes; NULL only for legacy v1.';
COMMENT ON COLUMN argus_catalog.browser_execution_attempts.execution_mode IS
    'Required shadow/staged pass semantics, not promotion authority; project/environment remain independently pinned.';
