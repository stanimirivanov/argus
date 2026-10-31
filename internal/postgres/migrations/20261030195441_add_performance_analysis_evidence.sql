-- Additive evidence expansion; no backfill, data rewrite, grants or consumer activation.
-- Application rollback keeps this table. Writers serialize pending rows before event keys.
CREATE TABLE argus_catalog.performance_analysis_events (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    admission_fingerprint TEXT NOT NULL, request_sha256 TEXT NOT NULL, run_id TEXT NOT NULL,
    event_id TEXT NOT NULL, event_sha256 BYTEA NOT NULL, fingerprint TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL, published_submicro_ns SMALLINT NOT NULL,
    publication BYTEA NOT NULL, review BYTEA NOT NULL, report BYTEA NOT NULL,
    publication_sha256 TEXT NOT NULL, review_sha256 TEXT NOT NULL, report_sha256 TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT pk_performance_analysis_events PRIMARY KEY (position, event_sha256),
    CONSTRAINT uq_performance_analysis_event UNIQUE (issuer, event_sha256),
    CONSTRAINT fk_performance_analysis_scope FOREIGN KEY (issuer, principal_issuer, principal_id, position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer, principal_issuer, principal_id, position) ON DELETE RESTRICT,
    CONSTRAINT fk_performance_analysis_admission FOREIGN KEY (position)
        REFERENCES argus_catalog.performance_dispatch_outcomes (position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_analysis_bounds CHECK (
        admission_fingerprint ~ '^[0-9a-f]{64}$' AND request_sha256 ~ '^[0-9a-f]{64}$' AND fingerprint ~ '^[0-9a-f]{64}$'
        AND CHAR_LENGTH(run_id) BETWEEN 1 AND 256 AND BTRIM(run_id) <> ''
        AND CHAR_LENGTH(event_id) BETWEEN 1 AND 256 AND BTRIM(event_id) <> '' AND OCTET_LENGTH(event_sha256)=32
        AND isfinite(published_at) AND isfinite(recorded_at) AND published_submicro_ns BETWEEN 0 AND 999
        AND OCTET_LENGTH(publication) BETWEEN 1 AND 65536 AND OCTET_LENGTH(review) BETWEEN 1 AND 2097152
        AND OCTET_LENGTH(report) BETWEEN 1 AND 1048576
        AND convert_from(publication,'UTF8')::jsonb IS NOT NULL AND convert_from(review,'UTF8')::jsonb IS NOT NULL
        AND convert_from(report,'UTF8')::jsonb IS NOT NULL),
    CONSTRAINT ck_performance_analysis_digests CHECK (
        publication_sha256=encode(sha256(publication),'hex') AND review_sha256=encode(sha256(review),'hex')
        AND report_sha256=encode(sha256(report),'hex'))
);
COMMENT ON TABLE argus_catalog.performance_analysis_events IS 'Scoped append-only reviewed analysis evidence; original bytes, independent outcomes, no aggregate verdict. No automatic deletion.';
COMMENT ON COLUMN argus_catalog.performance_analysis_events.published_submicro_ns IS '0..999 nanoseconds added to microsecond-truncated published_at; never round original publication time.';
COMMENT ON COLUMN argus_catalog.performance_analysis_events.recorded_at IS 'Database ingestion time, distinct from provider publication instant.';
CREATE FUNCTION argus_catalog.guard_performance_analysis() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE admission argus_catalog.performance_dispatch_outcomes%ROWTYPE; frozen_sha TEXT; requested TIMESTAMPTZ;
BEGIN
    -- Snapshot writers cannot observe rows committed while waiting on event locks.
    IF current_setting('transaction_isolation')<>'read committed' THEN
        RAISE EXCEPTION 'analysis guards require read committed' USING ERRCODE='25000', CONSTRAINT='ck_performance_analysis_isolation';
    END IF;
    NEW.event_sha256 := sha256(convert_to(NEW.event_id,'UTF8'));
    IF TG_TABLE_NAME<>'performance_analysis_events' THEN
        -- Existing ownership/lifecycle triggers already hold pending row then this event key.
        IF EXISTS (SELECT 1 FROM argus_catalog.performance_analysis_events WHERE issuer=NEW.issuer AND event_sha256=NEW.event_sha256) THEN
            RAISE EXCEPTION 'conflicting provider event' USING ERRCODE='23505', CONSTRAINT='uq_performance_analysis_event';
        END IF;
        IF TG_TABLE_NAME='performance_lifecycle_events' THEN
            IF EXISTS (SELECT 1 FROM argus_catalog.performance_analysis_events WHERE position=NEW.position AND run_id<>NEW.run_id) THEN
                RAISE EXCEPTION 'conflicting analysis run' USING ERRCODE='55000', CONSTRAINT='ck_performance_analysis_binding';
            END IF;
        END IF;
        RETURN NEW;
    END IF;
    NEW.recorded_at := clock_timestamp();
    SELECT payload_sha256,requested_at INTO frozen_sha,requested FROM argus_catalog.performance_pending_dispatches
        WHERE position=NEW.position AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
    IF NOT FOUND OR frozen_sha<>NEW.request_sha256 OR NEW.published_at<requested THEN
        RAISE EXCEPTION 'invalid analysis request' USING ERRCODE='55000', CONSTRAINT='ck_performance_analysis_binding';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.issuer||encode(NEW.event_sha256,'hex'),938071));
    SELECT * INTO admission FROM argus_catalog.performance_dispatch_outcomes WHERE position=NEW.position;
    IF NOT FOUND OR admission.kind='rejected' OR admission.fingerprint<>NEW.admission_fingerprint THEN
        RAISE EXCEPTION 'invalid analysis admission' USING ERRCODE='55000', CONSTRAINT='ck_performance_analysis_binding';
    END IF;
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes WHERE issuer=NEW.issuer AND event_sha256=NEW.event_sha256)
        OR EXISTS (SELECT 1 FROM argus_catalog.performance_lifecycle_events WHERE issuer=NEW.issuer AND event_sha256=NEW.event_sha256) THEN
        RAISE EXCEPTION 'conflicting provider event' USING ERRCODE='23505', CONSTRAINT='uq_performance_analysis_event';
    END IF;
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_analysis_events WHERE position=NEW.position AND run_id<>NEW.run_id)
        OR EXISTS (SELECT 1 FROM argus_catalog.performance_lifecycle_events WHERE position=NEW.position AND run_id<>NEW.run_id) THEN
        RAISE EXCEPTION 'conflicting analysis run' USING ERRCODE='55000', CONSTRAINT='ck_performance_analysis_binding';
    END IF;
    IF (SELECT count(*) FROM argus_catalog.performance_analysis_events WHERE position=NEW.position)>=1000 THEN
        RAISE EXCEPTION 'analysis history capacity' USING ERRCODE='55000', CONSTRAINT='ck_performance_analysis_capacity';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_analysis_guard BEFORE INSERT ON argus_catalog.performance_analysis_events
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_analysis();
-- Alphabetical names run after existing pending/event lock acquisition in both families.
CREATE TRIGGER trg_performance_outcome_zz_analysis BEFORE INSERT ON argus_catalog.performance_dispatch_outcomes
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_analysis();
CREATE TRIGGER trg_performance_lifecycle_z_analysis BEFORE INSERT ON argus_catalog.performance_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_analysis();
CREATE TRIGGER trg_performance_analysis_immutable BEFORE UPDATE OR DELETE ON argus_catalog.performance_analysis_events
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE TRIGGER trg_performance_analysis_truncate BEFORE TRUNCATE ON argus_catalog.performance_analysis_events
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
