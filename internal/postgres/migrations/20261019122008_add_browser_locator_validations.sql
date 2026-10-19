-- Browser locator validation evidence is immutable and separate from endpoint
-- and field validation contracts. Artifact references retain their checksums;
-- external byte retention remains the CI worker's responsibility.
CREATE TABLE argus_catalog.browser_locator_validations (
    validation_id TEXT PRIMARY KEY,
    validation_api_version TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    test_repository_id BIGINT NOT NULL REFERENCES argus_catalog.repositories(repository_id) ON DELETE RESTRICT,
    project_key TEXT NOT NULL,
    environment_key TEXT NOT NULL,
    validation_status TEXT NOT NULL,
    payload_sha256 TEXT NOT NULL,
    payload JSONB NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT ck_browser_locator_validation_id CHECK (validation_id ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_browser_locator_validation_version CHECK (validation_api_version = 'argus.dev/browser-locator-validation/v1'),
    CONSTRAINT ck_browser_locator_validation_proposal CHECK (proposal_id ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_browser_locator_validation_project CHECK (project_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_browser_locator_validation_environment CHECK (environment_key ~ '^[a-z][a-z0-9._-]{0,62}$'),
    CONSTRAINT ck_browser_locator_validation_status CHECK (validation_status IN ('validated', 'rejected')),
    CONSTRAINT ck_browser_locator_validation_payload_sha CHECK (payload_sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX ix_browser_locator_validations_group ON argus_catalog.browser_locator_validations(test_repository_id,project_key,environment_key);
COMMENT ON TABLE argus_catalog.browser_locator_validations IS 'Immutable browser locator six-run validation or trustworthy policy rejection, with checksummed artifact references.';
