-- Keep the v1 row and checksum unchanged for old binaries after rollback.
-- New deliveries atomically persist both the semantic v1 assessment and the
-- opt-in composite v2 extension.

CREATE TABLE argus_catalog.composite_impact_assessments (
    change_set_id BIGINT NOT NULL,
    impact_api_version TEXT NOT NULL,
    analyzer_version TEXT NOT NULL,
    impact_status TEXT NOT NULL,
    file_status TEXT NOT NULL,
    file_warnings TEXT[] NOT NULL DEFAULT '{}',
    impact_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_composite_impact_assessments PRIMARY KEY (change_set_id),
    CONSTRAINT fk_composite_impact_assessments_semantic FOREIGN KEY (change_set_id)
        REFERENCES argus_catalog.openapi_impact_assessments (change_set_id) ON DELETE CASCADE,
    CONSTRAINT ck_composite_impact_assessments_version CHECK (
        impact_api_version = 'argus.dev/capability-impact/v2'
        AND analyzer_version = 'argus-component-roots/v1+argus-openapi/v1+libopenapi/v0.38.7'
    ),
    CONSTRAINT ck_composite_impact_assessments_status CHECK (impact_status IN ('complete', 'partial') AND file_status IN ('complete', 'partial')),
    CONSTRAINT ck_composite_impact_assessments_sha256 CHECK (impact_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE TABLE argus_catalog.change_file_impacts (
    change_set_id BIGINT NOT NULL REFERENCES argus_catalog.composite_impact_assessments (change_set_id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    previous_file_path TEXT,
    change_kind TEXT NOT NULL,
    coverage TEXT NOT NULL,
    capability_keys TEXT[] NOT NULL DEFAULT '{}',
    component_roots TEXT[] NOT NULL DEFAULT '{}',
    unmapped_paths TEXT[] NOT NULL DEFAULT '{}',
    CONSTRAINT pk_change_file_impacts PRIMARY KEY (change_set_id, file_path),
    CONSTRAINT ck_change_file_impacts_path CHECK (
        char_length(file_path) BETWEEN 1 AND 4096
        AND file_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
    ),
    CONSTRAINT ck_change_file_impacts_previous_path CHECK (
        previous_file_path IS NULL OR (
            previous_file_path <> file_path AND char_length(previous_file_path) BETWEEN 1 AND 4096
            AND previous_file_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
        )
    ),
    CONSTRAINT ck_change_file_impacts_kind CHECK (change_kind IN ('added', 'modified', 'deleted', 'renamed', 'copied')),
    CONSTRAINT ck_change_file_impacts_coverage CHECK (coverage IN ('openapi', 'component-root', 'unmapped')),
    CONSTRAINT ck_change_file_impacts_mapping CHECK (
        (coverage = 'openapi' AND cardinality(capability_keys) = 0 AND cardinality(component_roots) = 0 AND cardinality(unmapped_paths) = 0)
        OR (coverage = 'component-root' AND cardinality(capability_keys) > 0 AND cardinality(component_roots) > 0 AND cardinality(unmapped_paths) = 0)
        OR (coverage = 'unmapped' AND cardinality(unmapped_paths) > 0)
    )
);

COMMENT ON TABLE argus_catalog.change_file_impacts IS
    'Immutable explicit file coverage and component-root provenance for capability-impact v2.';
COMMENT ON TABLE argus_catalog.composite_impact_assessments IS
    'Optional v2 extension of an unchanged semantic OpenAPI v1 assessment.';
