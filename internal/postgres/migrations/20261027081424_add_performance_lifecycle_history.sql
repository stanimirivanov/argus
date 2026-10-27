-- Additive, append-only lifecycle evidence. No backfill or worker activation.
-- Writers lock pending work before settlement/history; old admission writers remain valid.
CREATE TABLE argus_catalog.performance_lifecycle_events (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    admission_fingerprint TEXT NOT NULL, request_sha256 TEXT NOT NULL, run_id TEXT NOT NULL,
    event_id TEXT NOT NULL, event_sha256 BYTEA NOT NULL, fingerprint TEXT NOT NULL,
    revision BIGINT NOT NULL, state TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL,
    payload BYTEA NOT NULL, recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT pk_performance_lifecycle_events PRIMARY KEY (position, event_sha256),
    CONSTRAINT uq_performance_lifecycle_event UNIQUE (issuer, event_sha256),
    CONSTRAINT fk_performance_lifecycle_scope FOREIGN KEY (issuer, principal_issuer, principal_id, position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer, principal_issuer, principal_id, position) ON DELETE RESTRICT,
    CONSTRAINT fk_performance_lifecycle_admission FOREIGN KEY (position)
        REFERENCES argus_catalog.performance_dispatch_outcomes (position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_lifecycle_bounds CHECK (
        admission_fingerprint ~ '^[0-9a-f]{64}$' AND request_sha256 ~ '^[0-9a-f]{64}$' AND fingerprint ~ '^[0-9a-f]{64}$'
        AND CHAR_LENGTH(run_id) BETWEEN 1 AND 256 AND BTRIM(run_id) <> ''
        AND CHAR_LENGTH(event_id) BETWEEN 1 AND 256 AND BTRIM(event_id) <> '' AND OCTET_LENGTH(event_sha256)=32
        AND revision BETWEEN 0 AND 9007199254740991 AND state IN ('ACCEPTED','RUNNING','CANCELLING','COMPLETED','FAILED','CANCELLED')
        AND isfinite(occurred_at) AND isfinite(recorded_at) AND OCTET_LENGTH(payload) BETWEEN 1 AND 65536
        AND convert_from(payload, 'UTF8')::jsonb IS NOT NULL)
);
COMMENT ON TABLE argus_catalog.performance_lifecycle_events IS 'Scoped immutable lifecycle observations; adapter-validated run binding, never analysis PASS. No automatic retention deletion.';
COMMENT ON COLUMN argus_catalog.performance_lifecycle_events.occurred_at IS 'Provider event instant, including late evidence; distinct from database recording time.';
CREATE FUNCTION argus_catalog.guard_performance_lifecycle() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE admission argus_catalog.performance_dispatch_outcomes%ROWTYPE; frozen_sha TEXT; created TIMESTAMPTZ;
    states TEXT[] := ARRAY['ACCEPTED','RUNNING','CANCELLING','COMPLETED','FAILED','CANCELLED'];
BEGIN
    NEW.event_sha256 := sha256(convert_to(NEW.event_id, 'UTF8'));
    IF TG_TABLE_NAME='performance_dispatch_outcomes' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended(NEW.issuer||encode(NEW.event_sha256,'hex'),938071));
        IF EXISTS (SELECT 1 FROM argus_catalog.performance_lifecycle_events WHERE issuer=NEW.issuer AND event_sha256=NEW.event_sha256) THEN
            RAISE EXCEPTION 'conflicting provider event' USING ERRCODE='23505', CONSTRAINT='uq_performance_lifecycle_event';
        END IF;
        RETURN NEW;
    END IF;
    NEW.recorded_at := clock_timestamp();
    SELECT payload_sha256,requested_at INTO frozen_sha,created FROM argus_catalog.performance_pending_dispatches
        WHERE position=NEW.position AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
    IF NOT FOUND OR frozen_sha<>NEW.request_sha256 OR NEW.occurred_at<created THEN
        RAISE EXCEPTION 'invalid lifecycle binding' USING ERRCODE='55000', CONSTRAINT='ck_performance_lifecycle_binding';
    END IF;
    -- Share event serialization with admission writers, after the pending-row lock.
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.issuer||encode(NEW.event_sha256,'hex'),938071));
    SELECT * INTO admission FROM argus_catalog.performance_dispatch_outcomes WHERE position=NEW.position;
    IF NOT FOUND OR admission.kind='rejected' OR admission.fingerprint<>NEW.admission_fingerprint THEN
        RAISE EXCEPTION 'invalid lifecycle admission' USING ERRCODE='55000', CONSTRAINT='ck_performance_lifecycle_binding';
    END IF;
    -- Recovery may have settled from a lifecycle snapshot: retain that exact anchor first.
    IF admission.kind='lifecycle' AND NOT EXISTS (SELECT 1 FROM argus_catalog.performance_lifecycle_events WHERE position=NEW.position)
        AND (admission.event_id<>NEW.event_id OR admission.fingerprint<>NEW.fingerprint) THEN
        RAISE EXCEPTION 'missing lifecycle anchor' USING ERRCODE='55000', CONSTRAINT='ck_performance_lifecycle_binding';
    END IF;
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes WHERE issuer=NEW.issuer AND event_sha256=NEW.event_sha256
        AND (position<>NEW.position OR event_id<>NEW.event_id OR fingerprint<>NEW.fingerprint OR payload<>NEW.payload)) THEN
        RAISE EXCEPTION 'conflicting provider event' USING ERRCODE='55000', CONSTRAINT='ck_performance_lifecycle_binding';
    END IF;
    IF (SELECT count(*) FROM argus_catalog.performance_lifecycle_events WHERE position=NEW.position)>=1000
        OR EXISTS (SELECT 1 FROM argus_catalog.performance_lifecycle_events h WHERE h.position=NEW.position AND
            (h.run_id<>NEW.run_id OR (h.revision=NEW.revision AND h.state<>NEW.state)
            OR (h.revision<NEW.revision AND (h.state IN ('COMPLETED','FAILED','CANCELLED') OR array_position(states,h.state)>array_position(states,NEW.state)))
            OR (h.revision>NEW.revision AND (NEW.state IN ('COMPLETED','FAILED','CANCELLED') OR array_position(states,NEW.state)>array_position(states,h.state))))) THEN
        RAISE EXCEPTION 'conflicting lifecycle history' USING ERRCODE='55000', CONSTRAINT='ck_performance_lifecycle_transition';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_lifecycle_guard BEFORE INSERT ON argus_catalog.performance_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_lifecycle();
-- Alphabetical ordering runs after the existing ownership trigger (pending then claim).
CREATE TRIGGER trg_performance_outcome_z_lifecycle BEFORE INSERT ON argus_catalog.performance_dispatch_outcomes
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_lifecycle();
CREATE TRIGGER trg_performance_lifecycle_immutable BEFORE UPDATE OR DELETE ON argus_catalog.performance_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE TRIGGER trg_performance_lifecycle_truncate BEFORE TRUNCATE ON argus_catalog.performance_lifecycle_events
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
