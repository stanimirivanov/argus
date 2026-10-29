-- Add immutable scoped lineage before rerun APIs. No backfill, rewrites or activation.
-- Parent evidence stays authoritative; seed recovery history, assemble successor,
-- then all links in one READ COMMITTED transaction. Existing writers remain valid.
CREATE TABLE argus_catalog.performance_rerun_links (
    parent_position BIGINT NOT NULL, successor_position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    parent_intent_sha256 TEXT NOT NULL, parent_request_sha256 TEXT NOT NULL,
    parent_admission_sha256 TEXT NOT NULL, parent_lifecycle_sha256 TEXT NOT NULL,
    parent_run_id TEXT NOT NULL, parent_revision BIGINT NOT NULL, parent_state TEXT NOT NULL,
    parent_occurred_at TIMESTAMPTZ NOT NULL, parent_deadline TIMESTAMPTZ NOT NULL,
    successor_intent_uuid UUID NOT NULL, successor_intent_sha256 TEXT NOT NULL,
    successor_review_sha256 TEXT NOT NULL, successor_ordinal INTEGER NOT NULL,
    successor_request_sha256 TEXT NOT NULL, fingerprint TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT pk_performance_rerun_links PRIMARY KEY (parent_position),
    CONSTRAINT uq_performance_rerun_successor UNIQUE (successor_position),
    CONSTRAINT fk_performance_rerun_parent_scope FOREIGN KEY (issuer,principal_issuer,principal_id,parent_position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer,principal_issuer,principal_id,position) ON DELETE RESTRICT,
    CONSTRAINT fk_performance_rerun_successor_scope FOREIGN KEY (issuer,principal_issuer,principal_id,successor_position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer,principal_issuer,principal_id,position) ON DELETE RESTRICT,
    CONSTRAINT fk_performance_rerun_intent_scope FOREIGN KEY (issuer,principal_issuer,principal_id,successor_intent_uuid)
        REFERENCES argus_catalog.performance_request_intents (issuer,principal_issuer,principal_id,intent_uuid) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_rerun_bounds CHECK (parent_position>0 AND parent_position<successor_position
        AND successor_ordinal BETWEEN 0 AND 999 AND parent_revision BETWEEN 0 AND 9007199254740991
        AND parent_state IN ('COMPLETED','FAILED','CANCELLED') AND CHAR_LENGTH(parent_run_id) BETWEEN 1 AND 256 AND BTRIM(parent_run_id)<>''
        AND isfinite(parent_occurred_at) AND isfinite(parent_deadline) AND isfinite(recorded_at)),
    CONSTRAINT ck_performance_rerun_digests CHECK (
        parent_intent_sha256 ~ '^[0-9a-f]{64}$' AND parent_request_sha256 ~ '^[0-9a-f]{64}$'
        AND parent_admission_sha256 ~ '^[0-9a-f]{64}$' AND parent_lifecycle_sha256 ~ '^[0-9a-f]{64}$'
        AND successor_intent_sha256 ~ '^[0-9a-f]{64}$' AND successor_review_sha256 ~ '^[0-9a-f]{64}$'
        AND successor_request_sha256 ~ '^[0-9a-f]{64}$' AND fingerprint ~ '^[0-9a-f]{64}$')
);
-- PK/unique indexes serve scoped forward/reverse reads; this index bounds batch completeness.
CREATE INDEX ix_performance_rerun_intent ON argus_catalog.performance_rerun_links (successor_intent_uuid);
COMMENT ON TABLE argus_catalog.performance_rerun_links IS 'Immutable one-successor chains; pins terminal evidence, never approval or provider execution authority. Retain with parent evidence.';
COMMENT ON COLUMN argus_catalog.performance_rerun_links.parent_position IS 'Single direct successor per parent; increasing successor position prevents cycles.';
CREATE FUNCTION argus_catalog.guard_performance_rerun() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE prior argus_catalog.performance_pending_dispatches%ROWTYPE; child argus_catalog.performance_pending_dispatches%ROWTYPE;
    old_intent argus_catalog.performance_request_intents%ROWTYPE; new_intent argus_catalog.performance_request_intents%ROWTYPE;
    admission argus_catalog.performance_dispatch_outcomes%ROWTYPE; terminal argus_catalog.performance_lifecycle_events%ROWTYPE;
BEGIN
    IF current_setting('transaction_isolation')<>'read committed' THEN
        RAISE EXCEPTION 'read committed required for lineage' USING ERRCODE='25000';
    END IF;
    IF NEW.parent_position<=0 OR NEW.parent_position>=NEW.successor_position THEN
        RAISE EXCEPTION 'invalid lineage order' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_bounds';
    END IF;
    -- Same parent-row lock as lifecycle append; ascending positions avoid inverse lock order.
    SELECT * INTO prior FROM argus_catalog.performance_pending_dispatches WHERE position=NEW.parent_position
        AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'invalid lineage parent' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_binding';
    END IF;
    SELECT * INTO child FROM argus_catalog.performance_pending_dispatches WHERE position=NEW.successor_position
        AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'invalid lineage successor' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_binding';
    END IF;
    SELECT * INTO old_intent FROM argus_catalog.performance_request_intents WHERE intent_uuid=prior.intent_uuid;
    SELECT * INTO new_intent FROM argus_catalog.performance_request_intents WHERE intent_uuid=child.intent_uuid;
    IF new_intent.assembly_transaction<>pg_current_xact_id() OR child.intent_uuid<>NEW.successor_intent_uuid THEN
        RAISE EXCEPTION 'sealed lineage assembly' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_assembly';
    END IF;
    SELECT * INTO admission FROM argus_catalog.performance_dispatch_outcomes WHERE position=prior.position;
    IF NOT FOUND OR admission.kind NOT IN ('accepted','lifecycle') OR admission.fingerprint<>NEW.parent_admission_sha256 THEN
        RAISE EXCEPTION 'invalid lineage admission' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_binding';
    END IF;
    IF (old_intent.fingerprint,prior.payload_sha256,new_intent.fingerprint,new_intent.review_sha256,child.ordinal,child.payload_sha256)
        IS DISTINCT FROM (NEW.parent_intent_sha256,NEW.parent_request_sha256,NEW.successor_intent_sha256,NEW.successor_review_sha256,NEW.successor_ordinal,NEW.successor_request_sha256)
        OR old_intent.review_sha256=new_intent.review_sha256 THEN
        RAISE EXCEPTION 'invalid lineage pins' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_binding';
    END IF;
    SELECT * INTO terminal FROM argus_catalog.performance_lifecycle_events WHERE position=prior.position
        ORDER BY revision DESC,event_id COLLATE "C" DESC LIMIT 1;
    IF NOT FOUND OR terminal.state NOT IN ('COMPLETED','FAILED','CANCELLED')
        OR (terminal.fingerprint,terminal.admission_fingerprint,terminal.request_sha256,terminal.run_id,terminal.revision,terminal.state,terminal.occurred_at,prior.deadline)
        IS DISTINCT FROM (NEW.parent_lifecycle_sha256,NEW.parent_admission_sha256,NEW.parent_request_sha256,NEW.parent_run_id,NEW.parent_revision,NEW.parent_state,NEW.parent_occurred_at,NEW.parent_deadline) THEN
        RAISE EXCEPTION 'invalid lineage terminal' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_terminal';
    END IF;
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_pending_dispatches p WHERE p.intent_uuid IN (prior.intent_uuid,child.intent_uuid)
        AND p.position<>child.position AND (
            (p.request_issuer,p.request_id) IN ((child.request_issuer,child.request_id),(child.attempt_issuer,child.attempt_id),(child.request_issuer,child.event_id))
            OR (p.attempt_issuer,p.attempt_id) IN ((child.request_issuer,child.request_id),(child.attempt_issuer,child.attempt_id),(child.request_issuer,child.event_id))
            OR (p.request_issuer,p.event_id) IN ((child.request_issuer,child.request_id),(child.attempt_issuer,child.attempt_id),(child.request_issuer,child.event_id)))) THEN
        RAISE EXCEPTION 'reused lineage identity' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_identity';
    END IF;
    NEW.recorded_at:=clock_timestamp();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_rerun_guard BEFORE INSERT ON argus_catalog.performance_rerun_links
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_rerun();
CREATE FUNCTION argus_catalog.verify_performance_rerun_assembly() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF (SELECT count(*) FROM argus_catalog.performance_rerun_links WHERE successor_intent_uuid=NEW.successor_intent_uuid)
        <> (SELECT child_count FROM argus_catalog.performance_request_intents WHERE intent_uuid=NEW.successor_intent_uuid) THEN
        RAISE EXCEPTION 'incomplete lineage assembly' USING ERRCODE='23514', CONSTRAINT='ck_performance_rerun_assembly';
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER trg_performance_rerun_assembly AFTER INSERT ON argus_catalog.performance_rerun_links
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION argus_catalog.verify_performance_rerun_assembly();
CREATE TRIGGER trg_performance_rerun_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON argus_catalog.performance_rerun_links
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
