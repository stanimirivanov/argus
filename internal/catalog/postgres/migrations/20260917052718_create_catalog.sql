-- Create the immutable repository catalog. Repository coordinates are mutable
-- lookup metadata; snapshots and all snapshot-owned declarations are append-only.
-- The application writes one complete snapshot per transaction.

CREATE TABLE argus_catalog.repositories (
    repository_id BIGINT GENERATED ALWAYS AS IDENTITY,
    provider TEXT NOT NULL,
    host TEXT NOT NULL,
    provider_repository_id TEXT NOT NULL,
    owner_name TEXT NOT NULL,
    repository_name TEXT NOT NULL,
    first_observed_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    last_observed_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_repositories PRIMARY KEY (repository_id),
    CONSTRAINT uq_repositories_provider_identity
        UNIQUE (provider, host, provider_repository_id),
    CONSTRAINT ck_repositories_provider
        CHECK (provider IN ('github', 'gitlab', 'azure-devops', 'other')),
    CONSTRAINT ck_repositories_host
        CHECK (
            char_length(host) BETWEEN 1 AND 255
            AND host ~ '^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$'
        ),
    CONSTRAINT ck_repositories_provider_repository_id
        CHECK (char_length(provider_repository_id) BETWEEN 1 AND 255),
    CONSTRAINT ck_repositories_owner_name
        CHECK (char_length(owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_repositories_repository_name
        CHECK (char_length(repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_repositories_observation_order
        CHECK (last_observed_at >= first_observed_at)
);

COMMENT ON TABLE argus_catalog.repositories IS
    'Stable provider repository identities with their latest observed display coordinates.';
COMMENT ON COLUMN argus_catalog.repositories.provider_repository_id IS
    'Opaque provider-assigned identity; owner and repository_name are not identity.';

CREATE TABLE argus_catalog.catalog_snapshots (
    snapshot_id BIGINT GENERATED ALWAYS AS IDENTITY,
    source_repository_id BIGINT NOT NULL,
    source_owner_name TEXT NOT NULL,
    source_repository_name TEXT NOT NULL,
    revision_algorithm TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    descriptor_api_version TEXT NOT NULL,
    descriptor_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_catalog_snapshots PRIMARY KEY (snapshot_id),
    CONSTRAINT fk_catalog_snapshots_source_repository
        FOREIGN KEY (source_repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT uq_catalog_snapshots_identity
        UNIQUE (
            source_repository_id,
            revision_algorithm,
            revision_digest,
            descriptor_api_version
        ),
    CONSTRAINT ck_catalog_snapshots_source_owner_name
        CHECK (char_length(source_owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_catalog_snapshots_source_repository_name
        CHECK (char_length(source_repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_catalog_snapshots_revision
        CHECK (
            (revision_algorithm = 'git-sha1' AND revision_digest ~ '^[0-9a-f]{40}$')
            OR
            (revision_algorithm = 'git-sha256' AND revision_digest ~ '^[0-9a-f]{64}$')
        ),
    CONSTRAINT ck_catalog_snapshots_descriptor_api_version
        CHECK (descriptor_api_version = 'argus.dev/repository-descriptor/v1'),
    CONSTRAINT ck_catalog_snapshots_descriptor_sha256
        CHECK (descriptor_sha256 ~ '^[0-9a-f]{64}$')
);

COMMENT ON TABLE argus_catalog.catalog_snapshots IS
    'Immutable normalized descriptors bound to ingestion-verified source revisions.';
COMMENT ON COLUMN argus_catalog.catalog_snapshots.descriptor_sha256 IS
    'SHA-256 of the canonical normalized snapshot used to distinguish exact retries from conflicts.';

CREATE TABLE argus_catalog.capabilities (
    snapshot_id BIGINT NOT NULL,
    capability_key TEXT NOT NULL,
    capability_name TEXT NOT NULL,
    CONSTRAINT pk_capabilities PRIMARY KEY (snapshot_id, capability_key),
    CONSTRAINT fk_capabilities_snapshot
        FOREIGN KEY (snapshot_id)
        REFERENCES argus_catalog.catalog_snapshots (snapshot_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_capabilities_key
        CHECK (
            char_length(capability_key) BETWEEN 1 AND 63
            AND capability_key ~ '^[a-z][a-z0-9._-]*$'
        ),
    CONSTRAINT ck_capabilities_name
        CHECK (char_length(capability_name) BETWEEN 1 AND 255)
);

CREATE TABLE argus_catalog.components (
    snapshot_id BIGINT NOT NULL,
    component_key TEXT NOT NULL,
    component_root TEXT NOT NULL,
    CONSTRAINT pk_components PRIMARY KEY (snapshot_id, component_key),
    CONSTRAINT fk_components_snapshot
        FOREIGN KEY (snapshot_id)
        REFERENCES argus_catalog.catalog_snapshots (snapshot_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_components_key
        CHECK (
            char_length(component_key) BETWEEN 1 AND 63
            AND component_key ~ '^[a-z][a-z0-9._-]*$'
        ),
    CONSTRAINT ck_components_root
        CHECK (
            char_length(component_root) BETWEEN 1 AND 1024
            AND component_root !~ '(^/|\\|(^|/)\.\.(/|$)|(^|/)\.(/|$)|//|/$|^[A-Za-z]:)'
        )
);

CREATE TABLE argus_catalog.component_capabilities (
    snapshot_id BIGINT NOT NULL,
    component_key TEXT NOT NULL,
    capability_key TEXT NOT NULL,
    CONSTRAINT pk_component_capabilities
        PRIMARY KEY (snapshot_id, component_key, capability_key),
    CONSTRAINT fk_component_capabilities_component
        FOREIGN KEY (snapshot_id, component_key)
        REFERENCES argus_catalog.components (snapshot_id, component_key)
        ON DELETE CASCADE,
    CONSTRAINT fk_component_capabilities_capability
        FOREIGN KEY (snapshot_id, capability_key)
        REFERENCES argus_catalog.capabilities (snapshot_id, capability_key)
        ON DELETE CASCADE
);

CREATE TABLE argus_catalog.test_suites (
    snapshot_id BIGINT NOT NULL,
    test_repository_id BIGINT NOT NULL,
    suite_key TEXT NOT NULL,
    test_repository_owner_name TEXT NOT NULL,
    test_repository_name TEXT NOT NULL,
    test_family TEXT NOT NULL,
    adapter_name TEXT NOT NULL,
    CONSTRAINT pk_test_suites
        PRIMARY KEY (snapshot_id, test_repository_id, suite_key),
    CONSTRAINT fk_test_suites_snapshot
        FOREIGN KEY (snapshot_id)
        REFERENCES argus_catalog.catalog_snapshots (snapshot_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_test_suites_repository
        FOREIGN KEY (test_repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_test_suites_key
        CHECK (
            char_length(suite_key) BETWEEN 1 AND 63
            AND suite_key ~ '^[a-z][a-z0-9._-]*$'
        ),
    CONSTRAINT ck_test_suites_repository_owner_name
        CHECK (char_length(test_repository_owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_test_suites_repository_name
        CHECK (char_length(test_repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_test_suites_family
        CHECK (
            test_family IN (
                'unit',
                'component',
                'contract',
                'integration',
                'functional-api',
                'functional-ui',
                'end-to-end',
                'performance',
                'security',
                'resilience',
                'other'
            )
        ),
    CONSTRAINT ck_test_suites_adapter_name
        CHECK (char_length(adapter_name) BETWEEN 1 AND 127)
);

CREATE INDEX ix_test_suites_repository
    ON argus_catalog.test_suites (test_repository_id);

CREATE TABLE argus_catalog.tests (
    snapshot_id BIGINT NOT NULL,
    test_repository_id BIGINT NOT NULL,
    suite_key TEXT NOT NULL,
    test_key TEXT NOT NULL,
    test_name TEXT NOT NULL,
    CONSTRAINT pk_tests
        PRIMARY KEY (snapshot_id, test_repository_id, suite_key, test_key),
    CONSTRAINT fk_tests_suite
        FOREIGN KEY (snapshot_id, test_repository_id, suite_key)
        REFERENCES argus_catalog.test_suites (
            snapshot_id,
            test_repository_id,
            suite_key
        )
        ON DELETE CASCADE,
    CONSTRAINT ck_tests_key
        CHECK (
            char_length(test_key) BETWEEN 1 AND 63
            AND test_key ~ '^[a-z][a-z0-9._-]*$'
        ),
    CONSTRAINT ck_tests_name
        CHECK (char_length(test_name) BETWEEN 1 AND 255)
);

CREATE TABLE argus_catalog.test_capabilities (
    snapshot_id BIGINT NOT NULL,
    test_repository_id BIGINT NOT NULL,
    suite_key TEXT NOT NULL,
    test_key TEXT NOT NULL,
    capability_key TEXT NOT NULL,
    CONSTRAINT pk_test_capabilities
        PRIMARY KEY (
            snapshot_id,
            test_repository_id,
            suite_key,
            test_key,
            capability_key
        ),
    CONSTRAINT fk_test_capabilities_test
        FOREIGN KEY (snapshot_id, test_repository_id, suite_key, test_key)
        REFERENCES argus_catalog.tests (
            snapshot_id,
            test_repository_id,
            suite_key,
            test_key
        )
        ON DELETE CASCADE,
    CONSTRAINT fk_test_capabilities_capability
        FOREIGN KEY (snapshot_id, capability_key)
        REFERENCES argus_catalog.capabilities (snapshot_id, capability_key)
        ON DELETE CASCADE
);

COMMENT ON TABLE argus_catalog.component_capabilities IS
    'Explicit repository-declared component-to-capability mappings.';
COMMENT ON TABLE argus_catalog.test_capabilities IS
    'Explicit repository-declared stable-test-to-capability mappings.';
