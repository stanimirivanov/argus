-- Extend the immutable attempt reference to the reviewed functional API
-- manifest. Existing v1 attempts remain valid and require no rewrite.

ALTER TABLE argus_catalog.execution_attempts
    DROP CONSTRAINT ck_execution_attempts_manifest_api_version;

ALTER TABLE argus_catalog.execution_attempts
    ADD CONSTRAINT ck_execution_attempts_manifest_api_version
        CHECK (manifest_api_version IN (
            'argus.dev/execution-manifest/v1',
            'argus.dev/execution-manifest/v4'
        ));
