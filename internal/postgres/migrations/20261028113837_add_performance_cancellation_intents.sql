-- Apply before new callers. Retain intent/schema on rollback; no backfill or provider effects.
CREATE FUNCTION argus_catalog.performance_cancellation_event_key(TEXT,TEXT,TEXT,TEXT,TEXT)
RETURNS BYTEA LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT sha256(convert_to($1,'UTF8')||decode('00','hex')||convert_to($2,'UTF8')||decode('00','hex')||
        convert_to($3,'UTF8')||decode('00','hex')||convert_to($4,'UTF8')||decode('00','hex')||convert_to($5,'UTF8'));
$$;
CREATE TABLE argus_catalog.performance_cancellations (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    request_issuer TEXT NOT NULL, event_id TEXT NOT NULL, request_sha256 TEXT NOT NULL,
    payload BYTEA NOT NULL, fingerprint TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    event_key BYTEA GENERATED ALWAYS AS (argus_catalog.performance_cancellation_event_key(issuer,principal_issuer,principal_id,request_issuer,event_id)) STORED,
    CONSTRAINT pk_performance_cancellations PRIMARY KEY (position),
    CONSTRAINT uq_performance_cancellation_event UNIQUE (event_key),
    CONSTRAINT fk_performance_cancellation_scope FOREIGN KEY (issuer,principal_issuer,principal_id,position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer,principal_issuer,principal_id,position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_cancellation_bounds CHECK (event_id ~ '\S' AND char_length(event_id)<=256
        AND octet_length(payload) BETWEEN 1 AND 65536 AND request_sha256 ~ '^[0-9a-f]{64}$' AND fingerprint ~ '^[0-9a-f]{64}$')
);
COMMENT ON TABLE argus_catalog.performance_cancellations IS 'Immutable authorized caller intent and local first-acquisition cutoff, never provider acknowledgement or termination.';
CREATE FUNCTION argus_catalog.guard_performance_cancellation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE parent argus_catalog.performance_pending_dispatches%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation')<>'read committed' THEN
        RAISE EXCEPTION 'read committed required for cancellation' USING ERRCODE='25000';
    END IF;
    SELECT * INTO parent FROM argus_catalog.performance_pending_dispatches
        WHERE position=NEW.position AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
    IF NOT FOUND OR NEW.request_issuer<>parent.request_issuer OR NEW.request_sha256<>parent.payload_sha256 THEN
        RAISE EXCEPTION 'invalid cancellation binding' USING ERRCODE='23514', CONSTRAINT='ck_performance_cancellation_binding';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(argus_catalog.performance_cancellation_event_key(
        NEW.issuer,NEW.principal_issuer,NEW.principal_id,NEW.request_issuer,NEW.event_id),'hex'),938073));
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_pending_dispatches WHERE issuer=NEW.issuer
        AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id AND request_issuer=NEW.request_issuer AND event_id=NEW.event_id) THEN
        RAISE EXCEPTION 'reused request event' USING ERRCODE='23514', CONSTRAINT='ck_performance_cancellation_event';
    END IF;
    NEW.recorded_at:=clock_timestamp();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_cancellation_binding BEFORE INSERT ON argus_catalog.performance_cancellations
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_cancellation();
CREATE TRIGGER trg_performance_cancellation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON argus_catalog.performance_cancellations
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE FUNCTION argus_catalog.guard_performance_dispatch_cutoff() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF current_setting('transaction_isolation')<>'read committed' THEN
        RAISE EXCEPTION 'read committed required for acquisition' USING ERRCODE='25000';
    END IF;
    PERFORM 1 FROM argus_catalog.performance_pending_dispatches WHERE position=NEW.position FOR UPDATE;
    IF NOT EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_claims WHERE position=NEW.position)
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_cancellations WHERE position=NEW.position) THEN
        RAISE EXCEPTION 'first acquisition cancelled' USING ERRCODE='23514', CONSTRAINT='ck_performance_cancellation_cutoff';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_dispatch_cutoff BEFORE INSERT ON argus_catalog.performance_dispatch_claims
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_dispatch_cutoff();
CREATE FUNCTION argus_catalog.guard_performance_request_cancel_event() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE key BYTEA:=argus_catalog.performance_cancellation_event_key(NEW.issuer,NEW.principal_issuer,NEW.principal_id,NEW.request_issuer,NEW.event_id);
BEGIN
    IF current_setting('transaction_isolation')<>'read committed' THEN
        RAISE EXCEPTION 'read committed required for request events' USING ERRCODE='25000';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(encode(key,'hex'),938073));
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_cancellations WHERE event_key=key) THEN
        RAISE EXCEPTION 'reused cancellation event' USING ERRCODE='23514', CONSTRAINT='ck_performance_cancellation_event';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_dispatch_cancel_event BEFORE INSERT ON argus_catalog.performance_pending_dispatches
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_request_cancel_event();
