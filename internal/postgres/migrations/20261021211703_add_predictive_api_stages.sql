-- Expand immutable functional API attempts for opt-in staged protocol v2.
-- The existing table/evidence lifecycle and global attempt identity are retained.
-- NULL provenance means v1 only; v2 requires complete schedule/scope metadata.
-- Additive nullable columns need no backfill or row rewrite. Constraint validation
-- scans the header table under ALTER TABLE locks; apply before enabling v2 writes.
-- Old applications may keep writing v1, but cannot read new v2 attempts. Rollback
-- disables opt-in and retains this forward schema and all immutable evidence.

ALTER TABLE argus_catalog.execution_attempts
    ADD COLUMN schedule_sha256 TEXT,
    ADD COLUMN episode_id TEXT,
    ADD COLUMN group_key TEXT,
    ADD COLUMN environment_key TEXT,
    ADD COLUMN execution_mode TEXT,
    DROP CONSTRAINT ck_execution_attempts_api_version,
    DROP CONSTRAINT ck_execution_attempts_stage,
    ADD CONSTRAINT ck_execution_attempts_staged_envelope CHECK (
        (
            attempt_api_version = 'argus.dev/execution-attempt/v1'
            AND execution_stage IN ('selected', 'full-suite')
            AND schedule_sha256 IS NULL AND episode_id IS NULL
            AND group_key IS NULL AND environment_key IS NULL AND execution_mode IS NULL
        ) OR (
            attempt_api_version = 'argus.dev/execution-attempt/v2'
            AND schedule_sha256 IS NOT NULL AND schedule_sha256 ~ '^[0-9a-f]{64}$'
            AND episode_id IS NOT NULL AND episode_id ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND group_key IS NOT NULL AND group_key ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND environment_key IS NOT NULL AND environment_key ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND execution_mode IS NOT NULL AND (
                (execution_mode = 'shadow' AND execution_stage IN ('relevant-now', 'shadow-control'))
                OR (execution_mode = 'staged' AND execution_stage IN ('relevant-now', 'remaining-later'))
            )
        )
    );

COMMENT ON COLUMN argus_catalog.execution_attempts.schedule_sha256 IS
    'SHA-256 of complete argus.dev/predictive-portfolio/v1 bytes; NULL only for legacy v1 attempts.';
COMMENT ON COLUMN argus_catalog.execution_attempts.execution_mode IS
    'Required staged/shadow pass semantics; does not grant promotion or release authority.';
