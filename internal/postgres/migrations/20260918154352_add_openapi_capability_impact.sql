-- Persist the explainable result of semantic OpenAPI analysis. The assessment
-- is immutable for a normalized change set and all detail rows are typed so
-- operators can query evidence without decoding opaque JSON.

CREATE TABLE argus_catalog.openapi_impact_assessments (
    assessment_id BIGINT GENERATED ALWAYS AS IDENTITY,
    change_set_id BIGINT NOT NULL,
    impact_api_version TEXT NOT NULL,
    analyzer_version TEXT NOT NULL,
    impact_status TEXT NOT NULL,
    impact_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_openapi_impact_assessments PRIMARY KEY (assessment_id),
    CONSTRAINT uq_openapi_impact_assessments_change_set UNIQUE (change_set_id),
    CONSTRAINT fk_openapi_impact_assessments_change_set
        FOREIGN KEY (change_set_id)
        REFERENCES argus_catalog.change_sets (change_set_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_openapi_impact_assessments_api_version
        CHECK (impact_api_version = 'argus.dev/capability-impact/v1'),
    CONSTRAINT ck_openapi_impact_assessments_analyzer
        CHECK (analyzer_version = 'argus-openapi/v1+libopenapi/v0.38.7'),
    CONSTRAINT ck_openapi_impact_assessments_status CHECK (impact_status IN ('complete', 'partial')),
    CONSTRAINT ck_openapi_impact_assessments_sha256 CHECK (impact_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE TABLE argus_catalog.openapi_document_impacts (
    assessment_id BIGINT NOT NULL,
    document_path TEXT NOT NULL,
    previous_document_path TEXT,
    change_kind TEXT NOT NULL,
    total_changes INTEGER NOT NULL,
    breaking_changes INTEGER NOT NULL,
    CONSTRAINT pk_openapi_document_impacts PRIMARY KEY (assessment_id, document_path),
    CONSTRAINT fk_openapi_document_impacts_assessment
        FOREIGN KEY (assessment_id)
        REFERENCES argus_catalog.openapi_impact_assessments (assessment_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_openapi_document_impacts_path
        CHECK (
            char_length(document_path) BETWEEN 1 AND 4096
            AND document_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
        ),
    CONSTRAINT ck_openapi_document_impacts_previous_path
        CHECK (
            previous_document_path IS NULL
            OR (
                char_length(previous_document_path) BETWEEN 1 AND 4096
                AND previous_document_path <> document_path
                AND previous_document_path !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
            )
        ),
    CONSTRAINT ck_openapi_document_impacts_kind CHECK (change_kind IN ('added', 'modified', 'removed')),
    CONSTRAINT ck_openapi_document_impacts_counts
        CHECK (total_changes > 0 AND breaking_changes BETWEEN 0 AND total_changes),
    CONSTRAINT ck_openapi_document_impacts_kind_values
        CHECK (
            (change_kind = 'added' AND previous_document_path IS NULL AND breaking_changes = 0)
            OR (change_kind = 'modified')
            OR (change_kind = 'removed' AND previous_document_path IS NULL AND breaking_changes = total_changes)
        )
);

CREATE TABLE argus_catalog.openapi_operation_impacts (
    assessment_id BIGINT NOT NULL,
    document_path TEXT NOT NULL,
    http_method TEXT NOT NULL,
    operation_path TEXT NOT NULL,
    operation_id TEXT,
    change_kind TEXT NOT NULL,
    potentially_breaking BOOLEAN NOT NULL,
    CONSTRAINT pk_openapi_operation_impacts
        PRIMARY KEY (assessment_id, document_path, http_method, operation_path),
    CONSTRAINT fk_openapi_operation_impacts_document
        FOREIGN KEY (assessment_id, document_path)
        REFERENCES argus_catalog.openapi_document_impacts (assessment_id, document_path)
        ON DELETE CASCADE,
    CONSTRAINT ck_openapi_operation_impacts_method
        CHECK (http_method IN ('GET', 'PUT', 'POST', 'DELETE', 'OPTIONS', 'HEAD', 'PATCH', 'TRACE', 'QUERY')),
    CONSTRAINT ck_openapi_operation_impacts_path
        CHECK (char_length(operation_path) BETWEEN 1 AND 2048 AND left(operation_path, 1) = '/'),
    CONSTRAINT ck_openapi_operation_impacts_id
        CHECK (operation_id IS NULL OR char_length(operation_id) BETWEEN 1 AND 255),
    CONSTRAINT ck_openapi_operation_impacts_kind CHECK (change_kind IN ('added', 'modified', 'removed'))
);

CREATE TABLE argus_catalog.openapi_operation_capabilities (
    assessment_id BIGINT NOT NULL,
    document_path TEXT NOT NULL,
    http_method TEXT NOT NULL,
    operation_path TEXT NOT NULL,
    capability_key TEXT NOT NULL,
    CONSTRAINT pk_openapi_operation_capabilities
        PRIMARY KEY (assessment_id, document_path, http_method, operation_path, capability_key),
    CONSTRAINT fk_openapi_operation_capabilities_operation
        FOREIGN KEY (assessment_id, document_path, http_method, operation_path)
        REFERENCES argus_catalog.openapi_operation_impacts (
            assessment_id, document_path, http_method, operation_path
        )
        ON DELETE CASCADE,
    CONSTRAINT ck_openapi_operation_capabilities_key
        CHECK (capability_key ~ '^[a-z][a-z0-9._-]{0,62}$')
);

CREATE TABLE argus_catalog.openapi_impact_warnings (
    assessment_id BIGINT NOT NULL,
    warning_ordinal SMALLINT NOT NULL,
    warning_text TEXT NOT NULL,
    CONSTRAINT pk_openapi_impact_warnings PRIMARY KEY (assessment_id, warning_ordinal),
    CONSTRAINT fk_openapi_impact_warnings_assessment
        FOREIGN KEY (assessment_id)
        REFERENCES argus_catalog.openapi_impact_assessments (assessment_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_openapi_impact_warnings_ordinal CHECK (warning_ordinal BETWEEN 0 AND 99),
    CONSTRAINT ck_openapi_impact_warnings_text CHECK (char_length(warning_text) BETWEEN 1 AND 512)
);

COMMENT ON TABLE argus_catalog.openapi_impact_assessments IS
    'Immutable semantic OpenAPI impact derived from one normalized pull-request change set.';
COMMENT ON TABLE argus_catalog.openapi_operation_capabilities IS
    'Explicit x-argus-capabilities mappings retained as explainable impact evidence.';
