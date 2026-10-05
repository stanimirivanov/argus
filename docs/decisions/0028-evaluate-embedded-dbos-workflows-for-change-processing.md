# ADR-0028: Evaluate embedded DBOS workflows for change processing

- Status: Proposed
- Date: 2026-10-04
- Milestone: M10 - Production readiness
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Evaluate DBOS Go on one opt-in change-ingestion-to-impact path; the existing path remains the default and production authority.
- Keep Argus's immutable delivery and impact stores as the business source of truth. DBOS owns only workflow checkpoints in its separate `argus_dbos_eval` schema.
- Migrate the DBOS schema explicitly before enabling the flag; normal application startup verifies it and never creates it.
- Do not make DBOS a portfolio-wide standard or migrate Perfeng based on this experiment alone.

## Context

Argus currently acknowledges a signed webhook only after storing both normalized change evidence and semantic impact. The two steps are independently idempotent, but the process has no durable coordinator between them. Planned longer-lived work will need retries, waiting, and recovery. An embedded Go runtime backed by the already selected PostgreSQL may add that capability without operating a separate Temporal service.

## Decision

This is an evaluation, not production adoption. `ARGUS_DBOS_EVALUATION=true` selects a DBOS workflow for the existing webhook endpoint. Each HTTP delivery attempt starts a distinct workflow instance; immutable provider delivery identity and signed-body digest continue to be enforced by Argus's own stores. The workflow checkpoints ingestion and impact separately. It retains only the small ingestion-created flag as a step result and rereads canonical change evidence from Argus's store. The HTTP response waits for both steps and preserves its existing change-set document and created/retry status on successful attempts.

The DBOS library runs in the control-plane process and uses the existing Argus PostgreSQL database, but a separately owned `argus_dbos_eval` schema. An explicit `cmd/migrate --dbos-evaluation` administrative mode calls the pinned library's migration path; the upstream CLI does not compile on Windows in v1.5.0. Runtime starts with `SkipMigrations=true` and fails closed if the schema is missing or incompatible. No DBOS type enters the change domain or application use cases.

An HTTP timeout or disconnect does not revoke an already started durable workflow. Exact redelivery starts another workflow; the immutable Argus stores make repeated effects safe. A terminal failed workflow does not permanently poison a delivery ID because retries use a new workflow instance.

## Alternatives considered

### Self-host Temporal now

- Benefits: Mature shared operational surface and multi-language workers.
- Costs and risks: A separate service and a broader migration for a currently short Argus path.
- Reason not selected: This slice tests whether embedded coordination suffices before committing to a shared service.

### Replace Argus's evidence stores with DBOS state

- Benefits: Fewer apparent records.
- Costs and risks: Loses business-level immutable provenance and couples domain reads to workflow history.
- Reason not selected: Checkpoints and decision evidence have different authority and retention.

### Keep only the synchronous path

- Benefits: No new dependency or schema.
- Costs and risks: Does not test durable recovery or the operational burden of embedded workflows.
- Reason not selected: The experiment is requested now, ahead of longer-lived work.

## Consequences

### Positive

- A real change path can test embedded recovery and step checkpointing without changing default behavior.
- Perfeng and other products remain independent while the evidence is collected.

### Negative

- The evaluation adds a dependency, checkpoint storage, schema upgrade procedure, and workflow-code versioning obligation.
- DBOS-managed schema versions are outside Argus's checksummed SQL migration ledger. This is an evaluation-only exception; production promotion requires an explicit migration-ownership and upgrade policy under the SQL migration criteria.
- Separate workflow instances for exact redeliveries consume DBOS history; retention and cost must be measured before adoption.
- DBOS may reconstruct failed-step errors from persisted text, so exact transport error mapping after recovery needs additional proof before production promotion.

### Neutral or follow-up

- This does not atomically commit Argus's change evidence and the DBOS workflow checkpoint. Idempotent Argus writes and recovery are the present safety mechanism; a production asynchronous acceptance contract would need a separately verified atomic enqueue or outbox.
- The current synchronous response still imposes a bounded HTTP wait. A future asynchronous contract needs a versioned API decision.

## Compatibility and migration

The flag is off by default and does not alter existing contracts or Argus migration files. Apply ordinary Argus migrations, then run `go run ./cmd/migrate --dbos-evaluation` with a privileged `ARGUS_DATABASE_URL`, and only then enable the flag with the runtime identity. For an isolated workstation database, explicit `--local` mode selects the loopback service in `compose.local.yaml`; it never silently substitutes for missing deployed configuration. Turning the flag off restores the existing composition without deleting checkpoint history. Do not drop the experimental schema while an instance may still be running.

## Security and operations

The DBOS schema is separate from `argus_catalog`; use a privileged identity only for its explicit migration. The runtime identity needs only DBOS runtime privileges and the existing Argus data privileges. Deployed database URLs are provided through environment variables, not command arguments. The explicit workstation mode uses a fixed, loopback-only development URL and refuses an accompanying `ARGUS_DATABASE_URL`. Workflow input contains normalized delivery metadata, not the webhook secret, token, or raw signed body. Restrict and retain DBOS history according to the same sensitivity as repository metadata.

## Validation

Run the normal Argus suite, license and vulnerability gates, and PostgreSQL integration tests with a disposable loopback database. Exercise first delivery, exact retry, conflicting body, assessment failure followed by redelivery, and restart. Before accepting DBOS as production architecture, add a crash-at-each-boundary test, cancellation and rollout/versioning tests, database load/retention measurements, and error-mapping review.
