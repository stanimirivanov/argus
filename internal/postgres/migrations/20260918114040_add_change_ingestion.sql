-- Persist normalized, immutable pull-request change observations. A provider
-- delivery ID is claimed once; exact retries are no-ops and conflicting signed
-- bodies cannot overwrite the first accepted observation.

CREATE TABLE argus_catalog.change_sets (
    change_set_id BIGINT GENERATED ALWAYS AS IDENTITY,
    delivery_provider TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    payload_sha256 TEXT NOT NULL,
    delivery_event TEXT NOT NULL,
    delivery_action TEXT NOT NULL,
    source_repository_id BIGINT NOT NULL,
    source_owner_name TEXT NOT NULL,
    source_repository_name TEXT NOT NULL,
    pull_request_number BIGINT NOT NULL,
    base_revision_algorithm TEXT NOT NULL,
    base_revision_digest TEXT NOT NULL,
    head_revision_algorithm TEXT NOT NULL,
    head_revision_digest TEXT NOT NULL,
    change_set_api_version TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    files_truncated BOOLEAN NOT NULL,
    change_set_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_change_sets PRIMARY KEY (change_set_id),
    CONSTRAINT uq_change_sets_delivery UNIQUE (delivery_provider, delivery_id),
    CONSTRAINT fk_change_sets_source_repository
        FOREIGN KEY (source_repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_change_sets_delivery_provider CHECK (delivery_provider = 'github'),
    CONSTRAINT ck_change_sets_delivery_id CHECK (char_length(delivery_id) BETWEEN 1 AND 255),
    CONSTRAINT ck_change_sets_payload_sha256 CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_change_sets_delivery_event CHECK (delivery_event = 'pull_request'),
    CONSTRAINT ck_change_sets_delivery_action
        CHECK (delivery_action IN ('opened', 'reopened', 'synchronize', 'ready_for_review')),
    CONSTRAINT ck_change_sets_source_owner_name CHECK (char_length(source_owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_change_sets_source_repository_name CHECK (char_length(source_repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_change_sets_pull_request_number CHECK (pull_request_number > 0),
    CONSTRAINT ck_change_sets_base_revision
        CHECK (
            (base_revision_algorithm = 'git-sha1' AND base_revision_digest ~ '^[0-9a-f]{40}$')
            OR
            (base_revision_algorithm = 'git-sha256' AND base_revision_digest ~ '^[0-9a-f]{64}$')
        ),
    CONSTRAINT ck_change_sets_head_revision
        CHECK (
            (head_revision_algorithm = 'git-sha1' AND head_revision_digest ~ '^[0-9a-f]{40}$')
            OR
            (head_revision_algorithm = 'git-sha256' AND head_revision_digest ~ '^[0-9a-f]{64}$')
        ),
    CONSTRAINT ck_change_sets_distinct_revisions
        CHECK (
            base_revision_algorithm <> head_revision_algorithm
            OR base_revision_digest <> head_revision_digest
        ),
    CONSTRAINT ck_change_sets_api_version CHECK (change_set_api_version = 'argus.dev/change-set/v1'),
    CONSTRAINT ck_change_sets_sha256 CHECK (change_set_sha256 ~ '^[0-9a-f]{64}$')
);

COMMENT ON TABLE argus_catalog.change_sets IS
    'Immutable normalized pull-request comparisons claimed by authenticated provider delivery identity.';
COMMENT ON COLUMN argus_catalog.change_sets.payload_sha256 IS
    'SHA-256 of the exact signed webhook body, used to reject delivery-ID reuse.';
COMMENT ON COLUMN argus_catalog.change_sets.change_set_sha256 IS
    'SHA-256 of the canonical normalized change set, used to distinguish exact retries from conflicts.';
COMMENT ON COLUMN argus_catalog.change_sets.files_truncated IS
    'True when provider changes exceeded the bounded file evidence retained by Argus.';

CREATE INDEX ix_change_sets_repository_revisions
    ON argus_catalog.change_sets (
        source_repository_id,
        base_revision_algorithm,
        base_revision_digest,
        head_revision_algorithm,
        head_revision_digest
    );

CREATE TABLE argus_catalog.change_files (
    change_set_id BIGINT NOT NULL,
    file_path TEXT NOT NULL,
    previous_file_path TEXT,
    change_kind TEXT NOT NULL,
    additions INTEGER NOT NULL,
    deletions INTEGER NOT NULL,
    patch_text TEXT,
    patch_status TEXT NOT NULL,
    CONSTRAINT pk_change_files PRIMARY KEY (change_set_id, file_path),
    CONSTRAINT fk_change_files_change_set
        FOREIGN KEY (change_set_id)
        REFERENCES argus_catalog.change_sets (change_set_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_change_files_path
        CHECK (
            char_length(file_path) BETWEEN 1 AND 4096
            AND file_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
        ),
    CONSTRAINT ck_change_files_previous_path
        CHECK (
            previous_file_path IS NULL
            OR (
                char_length(previous_file_path) BETWEEN 1 AND 4096
                AND previous_file_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
            )
        ),
    CONSTRAINT ck_change_files_kind
        CHECK (change_kind IN ('added', 'modified', 'deleted', 'renamed', 'copied')),
    CONSTRAINT ck_change_files_previous_path_by_kind
        CHECK (
            (change_kind IN ('renamed', 'copied') AND previous_file_path IS NOT NULL AND previous_file_path <> file_path)
            OR
            (change_kind IN ('added', 'modified', 'deleted') AND previous_file_path IS NULL)
        ),
    CONSTRAINT ck_change_files_line_counts CHECK (additions >= 0 AND deletions >= 0),
    CONSTRAINT ck_change_files_patch_size CHECK (patch_text IS NULL OR octet_length(patch_text) <= 65536),
    CONSTRAINT ck_change_files_patch_status
        CHECK (patch_status IN ('complete', 'unavailable', 'truncated', 'budget-exhausted')),
    CONSTRAINT ck_change_files_patch_presence
        CHECK (
            (patch_status IN ('complete', 'truncated') AND patch_text IS NOT NULL)
            OR
            (patch_status IN ('unavailable', 'budget-exhausted') AND patch_text IS NULL)
        )
);

COMMENT ON TABLE argus_catalog.change_files IS
    'Bounded changed-file and patch evidence owned by one immutable change set.';
COMMENT ON COLUMN argus_catalog.change_files.patch_status IS
    'Explains whether patch evidence is complete, omitted by GitHub, truncated, or excluded by the aggregate budget.';
