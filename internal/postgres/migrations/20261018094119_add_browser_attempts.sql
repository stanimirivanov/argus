-- Browser attempts have a distinct protocol and preserve project/environment
-- coordinates without changing functional API execution-attempt v1.
CREATE TABLE argus_catalog.browser_execution_attempts (
    attempt_id TEXT PRIMARY KEY,
    attempt_api_version TEXT NOT NULL,
    manifest_sha256 TEXT NOT NULL,
    test_repository_id BIGINT NOT NULL REFERENCES argus_catalog.repositories(repository_id) ON DELETE RESTRICT,
    project_key TEXT NOT NULL,
    environment_key TEXT NOT NULL,
    attempt_outcome TEXT NOT NULL,
    attempt_sha256 TEXT NOT NULL,
    payload JSONB NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT ck_browser_attempt_id CHECK (attempt_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,126}$'),
    CONSTRAINT ck_browser_attempt_version CHECK (attempt_api_version = 'argus.dev/browser-execution-attempt/v1'),
    CONSTRAINT ck_browser_attempt_manifest CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_browser_attempt_project CHECK (project_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_browser_attempt_environment CHECK (environment_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_browser_attempt_outcome CHECK (attempt_outcome IN ('passed','failed','incomplete','error')),
    CONSTRAINT ck_browser_attempt_sha CHECK (attempt_sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX ix_browser_execution_attempts_group ON argus_catalog.browser_execution_attempts(test_repository_id,project_key,environment_key);
COMMENT ON TABLE argus_catalog.browser_execution_attempts IS 'Immutable browser attempt envelope and test-bound artifact metadata. External artifact retention is not asserted.';
