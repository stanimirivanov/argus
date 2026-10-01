-- Persist immutable normalized functional API attempts and their child evidence.
-- Attempt identity is caller-supplied and globally idempotent; the stored
-- fingerprint distinguishes exact retries from conflicting reuse. Artifact
-- rows are references only and do not assert that external bytes still exist.

CREATE TABLE argus_catalog.execution_attempts (
    execution_attempt_id BIGINT GENERATED ALWAYS AS IDENTITY,
    attempt_id TEXT NOT NULL,
    attempt_api_version TEXT NOT NULL,
    manifest_api_version TEXT NOT NULL,
    manifest_sha256 TEXT NOT NULL,
    execution_stage TEXT NOT NULL,
    test_repository_id BIGINT NOT NULL,
    test_repository_owner_name TEXT NOT NULL,
    test_repository_name TEXT NOT NULL,
    test_revision_algorithm TEXT NOT NULL,
    test_revision_digest TEXT NOT NULL,
    adapter_id TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    attempt_outcome TEXT NOT NULL,
    attempt_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_execution_attempts PRIMARY KEY (execution_attempt_id),
    CONSTRAINT uq_execution_attempts_attempt_id UNIQUE (attempt_id),
    CONSTRAINT fk_execution_attempts_test_repository
        FOREIGN KEY (test_repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_execution_attempts_attempt_id
        CHECK (attempt_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,126}$'),
    CONSTRAINT ck_execution_attempts_api_version
        CHECK (attempt_api_version = 'argus.dev/execution-attempt/v1'),
    CONSTRAINT ck_execution_attempts_manifest_api_version
        CHECK (manifest_api_version = 'argus.dev/execution-manifest/v1'),
    CONSTRAINT ck_execution_attempts_manifest_sha256
        CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_execution_attempts_stage
        CHECK (execution_stage IN ('selected', 'full-suite')),
    CONSTRAINT ck_execution_attempts_repository_owner_name
        CHECK (char_length(test_repository_owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_execution_attempts_repository_name
        CHECK (char_length(test_repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_execution_attempts_test_revision
        CHECK (
            (test_revision_algorithm = 'git-sha1' AND test_revision_digest ~ '^[0-9a-f]{40}$')
            OR
            (test_revision_algorithm = 'git-sha256' AND test_revision_digest ~ '^[0-9a-f]{64}$')
        ),
    CONSTRAINT ck_execution_attempts_adapter_id
        CHECK (adapter_id ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_execution_attempts_adapter_version
        CHECK (char_length(adapter_version) BETWEEN 1 AND 127),
    CONSTRAINT ck_execution_attempts_time_range
        CHECK (completed_at >= started_at),
    CONSTRAINT ck_execution_attempts_outcome
        CHECK (attempt_outcome IN ('passed', 'failed', 'incomplete', 'error')),
    CONSTRAINT ck_execution_attempts_sha256
        CHECK (attempt_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE TABLE argus_catalog.execution_test_results (
    execution_attempt_id BIGINT NOT NULL,
    suite_key TEXT NOT NULL,
    test_key TEXT NOT NULL,
    test_outcome TEXT NOT NULL,
    duration_ms BIGINT NOT NULL,
    failure_code TEXT,
    failure_message TEXT,
    CONSTRAINT pk_execution_test_results
        PRIMARY KEY (execution_attempt_id, suite_key, test_key),
    CONSTRAINT fk_execution_test_results_attempt
        FOREIGN KEY (execution_attempt_id)
        REFERENCES argus_catalog.execution_attempts (execution_attempt_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_execution_test_results_suite_key
        CHECK (suite_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_execution_test_results_test_key
        CHECK (test_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_execution_test_results_outcome
        CHECK (test_outcome IN ('passed', 'failed', 'skipped', 'error')),
    CONSTRAINT ck_execution_test_results_duration
        CHECK (duration_ms BETWEEN 0 AND 86400000),
    CONSTRAINT ck_execution_test_results_failure
        CHECK (
            (
                test_outcome IN ('passed', 'skipped')
                AND failure_code IS NULL
                AND failure_message IS NULL
            )
            OR
            (
                test_outcome IN ('failed', 'error')
                AND failure_code ~ '^[a-z][a-z0-9._-]{0,62}$'
                AND char_length(failure_message) BETWEEN 1 AND 2000
            )
        )
);

CREATE TABLE argus_catalog.execution_artifacts (
    execution_attempt_id BIGINT NOT NULL,
    artifact_key TEXT NOT NULL,
    artifact_kind TEXT NOT NULL,
    artifact_uri TEXT NOT NULL,
    artifact_sha256 TEXT NOT NULL,
    CONSTRAINT pk_execution_artifacts
        PRIMARY KEY (execution_attempt_id, artifact_key),
    CONSTRAINT fk_execution_artifacts_attempt
        FOREIGN KEY (execution_attempt_id)
        REFERENCES argus_catalog.execution_attempts (execution_attempt_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_execution_artifacts_key
        CHECK (artifact_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_execution_artifacts_kind
        CHECK (artifact_kind ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_execution_artifacts_uri
        CHECK (
            char_length(artifact_uri) BETWEEN 1 AND 2048
            AND artifact_uri ~ '^[A-Za-z][A-Za-z0-9+.-]*:'
        ),
    CONSTRAINT ck_execution_artifacts_sha256
        CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$')
);

COMMENT ON TABLE argus_catalog.execution_attempts IS
    'Immutable normalized functional API attempts bound to canonical manifest bytes and a test revision.';
COMMENT ON TABLE argus_catalog.execution_test_results IS
    'Framework-neutral per-test outcomes used for full-suite shadow comparison.';
COMMENT ON TABLE argus_catalog.execution_artifacts IS
    'Immutable artifact references; object existence and retention are owned by the external artifact system.';
