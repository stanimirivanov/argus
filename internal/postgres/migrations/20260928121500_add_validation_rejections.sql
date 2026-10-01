-- Persist trustworthy policy rejections separately from successful validation
-- evidence. Infrastructure and integrity failures never enter these tables.

CREATE TABLE argus_catalog.adaptation_validation_rejections (
    validation_id TEXT PRIMARY KEY,
    rejection_api_version TEXT NOT NULL,
    validation_policy_version TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    proposal_policy_version TEXT NOT NULL,
    repository_id BIGINT NOT NULL,
    repository_owner_name TEXT NOT NULL,
    repository_name TEXT NOT NULL,
    revision_algorithm TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    suite_key TEXT NOT NULL,
    test_key TEXT NOT NULL,
    test_name TEXT NOT NULL,
    adapter_id TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    capabilities TEXT[] NOT NULL,
    edit_path TEXT NOT NULL,
    edit_before_sha256 TEXT NOT NULL,
    edit_start_byte INTEGER NOT NULL,
    edit_end_byte INTEGER NOT NULL,
    edit_original TEXT NOT NULL,
    edit_replacement TEXT NOT NULL,
    edit_semantic_role TEXT NOT NULL,
    candidate_sha256 TEXT NOT NULL,
    negative_sha256 TEXT NOT NULL,
    negative_control_path TEXT NOT NULL,
    rejected_phase TEXT NOT NULL,
    rejection_reason TEXT NOT NULL,
    expected_outcome TEXT NOT NULL,
    actual_outcome TEXT NOT NULL,
    rejection_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT fk_adaptation_validation_rejections_repository
        FOREIGN KEY (repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_adaptation_validation_rejections_versions
        CHECK (
            rejection_api_version = 'argus.dev/validation-rejection/v1'
            AND validation_policy_version = 'argus.dev/validation-policy/functional-api-endpoint-rename/v1'
            AND proposal_policy_version = 'argus.dev/adaptation-policy/functional-api-endpoint-rename/v1'
        ),
    CONSTRAINT ck_adaptation_validation_rejections_hashes
        CHECK (
            validation_id ~ '^[0-9a-f]{64}$'
            AND proposal_id ~ '^[0-9a-f]{64}$'
            AND edit_before_sha256 ~ '^[0-9a-f]{64}$'
            AND candidate_sha256 ~ '^[0-9a-f]{64}$'
            AND negative_sha256 ~ '^[0-9a-f]{64}$'
            AND rejection_sha256 ~ '^[0-9a-f]{64}$'
        ),
    CONSTRAINT ck_adaptation_validation_rejections_revision
        CHECK (
            (revision_algorithm = 'git-sha1' AND revision_digest ~ '^[0-9a-f]{40}$')
            OR (revision_algorithm = 'git-sha256' AND revision_digest ~ '^[0-9a-f]{64}$')
        ),
    CONSTRAINT ck_adaptation_validation_rejections_keys
        CHECK (
            suite_key ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND test_key ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND adapter_id ~ '^[a-z][a-z0-9._-]{0,62}$'
            AND cardinality(capabilities) BETWEEN 1 AND 5000
        ),
    CONSTRAINT ck_adaptation_validation_rejections_text
        CHECK (
            char_length(repository_owner_name) BETWEEN 1 AND 255
            AND char_length(repository_name) BETWEEN 1 AND 255
            AND char_length(test_name) BETWEEN 1 AND 255
            AND char_length(adapter_version) BETWEEN 1 AND 127
        ),
    CONSTRAINT ck_adaptation_validation_rejections_edit
        CHECK (
            char_length(edit_path) BETWEEN 1 AND 4096
            AND edit_path !~ '^[/\\]'
            AND edit_path !~ '\\'
            AND edit_start_byte >= 0
            AND edit_end_byte > edit_start_byte
            AND octet_length(edit_original) = edit_end_byte - edit_start_byte
            AND edit_original <> edit_replacement
            AND edit_semantic_role = 'request-target'
        ),
    CONSTRAINT ck_adaptation_validation_rejections_control
        CHECK (negative_control_path ~ '^/__argus_negative_control__/'),
    CONSTRAINT ck_adaptation_validation_rejections_reason
        CHECK (
            (rejection_reason = 'original-passed' AND rejected_phase = 'original'
                AND expected_outcome = 'failed' AND actual_outcome = 'passed')
            OR (rejection_reason = 'candidate-failed' AND rejected_phase = 'candidate'
                AND expected_outcome = 'passed' AND actual_outcome = 'failed')
            OR (rejection_reason = 'negative-control-passed' AND rejected_phase = 'negative-control'
                AND expected_outcome = 'failed' AND actual_outcome = 'passed')
        )
);

CREATE TABLE argus_catalog.adaptation_validation_rejection_runs (
    validation_id TEXT NOT NULL,
    run_ordinal SMALLINT NOT NULL,
    phase TEXT NOT NULL,
    source_sha256 TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    outcome TEXT NOT NULL,
    failure_code TEXT,
    failure_message TEXT,
    CONSTRAINT pk_adaptation_validation_rejection_runs PRIMARY KEY (validation_id, run_ordinal),
    CONSTRAINT fk_adaptation_validation_rejection_runs_rejection
        FOREIGN KEY (validation_id)
        REFERENCES argus_catalog.adaptation_validation_rejections (validation_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_adaptation_validation_rejection_runs_ordinal CHECK (run_ordinal BETWEEN 0 AND 2),
    CONSTRAINT ck_adaptation_validation_rejection_runs_phase
        CHECK (phase IN ('original', 'candidate', 'negative-control')),
    CONSTRAINT ck_adaptation_validation_rejection_runs_hash
        CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_adaptation_validation_rejection_runs_time CHECK (completed_at >= started_at),
    CONSTRAINT ck_adaptation_validation_rejection_runs_outcome CHECK (outcome IN ('passed', 'failed')),
    CONSTRAINT ck_adaptation_validation_rejection_runs_failure
        CHECK (
            (outcome = 'passed' AND failure_code IS NULL AND failure_message IS NULL)
            OR (
                outcome = 'failed'
                AND failure_code ~ '^[a-z][a-z0-9._-]{0,62}$'
                AND char_length(failure_message) BETWEEN 1 AND 2000
            )
        )
);

COMMENT ON TABLE argus_catalog.adaptation_validation_rejections IS
    'Immutable negative validation evidence for candidates disproved by a trustworthy policy gate.';
