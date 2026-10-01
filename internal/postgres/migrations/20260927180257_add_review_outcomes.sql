-- Persist one immutable terminal adaptation outcome per published review.
-- The semantic fingerprint excludes observed_at so a later exact observation
-- converges on the first durable record. Reviewer patches are complete bounded
-- evidence, not source artifacts or merge authorization.

CREATE TABLE argus_catalog.adaptation_review_outcomes (
    review_outcome_id BIGINT GENERATED ALWAYS AS IDENTITY,
    outcome_id TEXT NOT NULL,
    outcome_api_version TEXT NOT NULL,
    review_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    validation_id TEXT NOT NULL,
    repository_id BIGINT NOT NULL,
    repository_owner_name TEXT NOT NULL,
    repository_name TEXT NOT NULL,
    pull_request_number INTEGER NOT NULL,
    pull_request_url TEXT NOT NULL,
    review_decision TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    reason_note TEXT,
    generated_revision_algorithm TEXT NOT NULL,
    generated_revision_digest TEXT NOT NULL,
    final_revision_algorithm TEXT NOT NULL,
    final_revision_digest TEXT NOT NULL,
    closed_at TIMESTAMPTZ NOT NULL,
    merged_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL,
    outcome_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_adaptation_review_outcomes PRIMARY KEY (review_outcome_id),
    CONSTRAINT uq_adaptation_review_outcomes_outcome_id UNIQUE (outcome_id),
    CONSTRAINT uq_adaptation_review_outcomes_review_id UNIQUE (review_id),
    CONSTRAINT fk_adaptation_review_outcomes_repository
        FOREIGN KEY (repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_adaptation_review_outcomes_api_version
        CHECK (outcome_api_version = 'argus.dev/review-outcome/v1'),
    CONSTRAINT ck_adaptation_review_outcomes_hashes
        CHECK (
            outcome_id ~ '^[0-9a-f]{64}$'
            AND review_id ~ '^[0-9a-f]{64}$'
            AND proposal_id ~ '^[0-9a-f]{64}$'
            AND validation_id ~ '^[0-9a-f]{64}$'
            AND outcome_sha256 ~ '^[0-9a-f]{64}$'
        ),
    CONSTRAINT ck_adaptation_review_outcomes_repository_coordinates
        CHECK (
            char_length(repository_owner_name) BETWEEN 1 AND 255
            AND char_length(repository_name) BETWEEN 1 AND 255
        ),
    CONSTRAINT ck_adaptation_review_outcomes_pull_request
        CHECK (
            pull_request_number > 0
            AND char_length(pull_request_url) BETWEEN 1 AND 2048
            AND pull_request_url ~ '^https://'
        ),
    CONSTRAINT ck_adaptation_review_outcomes_reason_note
        CHECK (
            reason_note IS NULL
            OR (
                char_length(reason_note) BETWEEN 1 AND 1000
                AND btrim(reason_note) = reason_note
            )
        ),
    CONSTRAINT ck_adaptation_review_outcomes_revisions
        CHECK (
            generated_revision_algorithm = final_revision_algorithm
            AND (
                (
                    generated_revision_algorithm = 'git-sha1'
                    AND generated_revision_digest ~ '^[0-9a-f]{40}$'
                    AND final_revision_digest ~ '^[0-9a-f]{40}$'
                )
                OR
                (
                    generated_revision_algorithm = 'git-sha256'
                    AND generated_revision_digest ~ '^[0-9a-f]{64}$'
                    AND final_revision_digest ~ '^[0-9a-f]{64}$'
                )
            )
        ),
    CONSTRAINT ck_adaptation_review_outcomes_times
        CHECK (
            observed_at >= closed_at
            AND (merged_at IS NULL OR merged_at <= closed_at)
        ),
    CONSTRAINT ck_adaptation_review_outcomes_decision
        CHECK (
            (
                review_decision = 'accepted-as-proposed'
                AND reason_code = 'approved'
                AND merged_at IS NOT NULL
            )
            OR
            (
                review_decision = 'accepted-with-edits'
                AND reason_code = 'corrected'
                AND merged_at IS NOT NULL
            )
            OR
            (
                review_decision = 'rejected'
                AND reason_code IN (
                    'incorrect-repair',
                    'unsafe-repair',
                    'no-longer-needed',
                    'superseded',
                    'other'
                )
                AND merged_at IS NULL
            )
        ),
    CONSTRAINT ck_adaptation_review_outcomes_other_reason
        CHECK (reason_code <> 'other' OR reason_note IS NOT NULL)
);

CREATE TABLE argus_catalog.adaptation_review_edits (
    review_outcome_id BIGINT NOT NULL,
    path TEXT NOT NULL,
    previous_path TEXT,
    edit_kind TEXT NOT NULL,
    additions BIGINT NOT NULL,
    deletions BIGINT NOT NULL,
    patch TEXT NOT NULL,
    CONSTRAINT pk_adaptation_review_edits
        PRIMARY KEY (review_outcome_id, path),
    CONSTRAINT fk_adaptation_review_edits_outcome
        FOREIGN KEY (review_outcome_id)
        REFERENCES argus_catalog.adaptation_review_outcomes (review_outcome_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_adaptation_review_edits_path
        CHECK (
            char_length(path) BETWEEN 1 AND 4096
            AND path !~ '^[/\\]'
            AND path !~ '\\'
            AND (
                previous_path IS NULL
                OR (
                    char_length(previous_path) BETWEEN 1 AND 4096
                    AND previous_path !~ '^[/\\]'
                    AND previous_path !~ '\\'
                )
            )
        ),
    CONSTRAINT ck_adaptation_review_edits_kind
        CHECK (edit_kind IN ('added', 'modified', 'deleted', 'renamed', 'copied')),
    CONSTRAINT ck_adaptation_review_edits_previous_path
        CHECK (
            (edit_kind = 'renamed' AND previous_path IS NOT NULL AND previous_path <> path)
            OR
            (edit_kind <> 'renamed' AND previous_path IS NULL)
        ),
    CONSTRAINT ck_adaptation_review_edits_changes
        CHECK (additions >= 0 AND deletions >= 0 AND additions + deletions > 0),
    CONSTRAINT ck_adaptation_review_edits_patch
        CHECK (octet_length(patch) BETWEEN 1 AND 65536)
);

COMMENT ON TABLE argus_catalog.adaptation_review_outcomes IS
    'One immutable terminal outcome per published adaptation review; merge state is evidence, not correctness authority.';
COMMENT ON TABLE argus_catalog.adaptation_review_edits IS
    'Complete bounded reviewer-authored patches between the generated and final review heads.';
