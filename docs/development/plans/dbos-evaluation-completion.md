# DBOS evaluation completion plan

## TL;DR

- Two reviewable M10 slices close the opt-in evaluation without silently promoting it to production.
- The first proves application-version rollout/rollback behavior and documents the migration ownership and drain rule.
- The second runs a real two-release/two-SDK compatibility matrix, measures DBOS latency, storage, retry amplification, retention, and cleanup against the established path, then records an evidence-backed adoption decision.
- The combined compatibility/cost slice now provides reproducible automation and a [defer verdict](../dbos-evaluation-verdict.md). A production implementation is a separate task, not an automatic flag change.

## Outcome and invariants

The evaluation determines whether embedded DBOS Go is a suitable durable coordinator for Argus change ingestion and impact assessment. The default synchronous path remains authoritative until an explicit production decision. Argus's immutable evidence stores remain the source of business truth; DBOS checkpoints stay in `argus_dbos_eval`. Webhook signatures, idempotency, conflict handling, and failure classification must not regress. No conclusion for Perfeng or Argus SRE follows automatically.

## Ordered pull-request slices

1. **Rollout and rollback safety.** Exercise interrupted work under two DBOS application-version identities on the same isolated PostgreSQL database; verify new-version admission, old-version non-recovery, migration replay, restoration of the old version, and unchanged Argus evidence. Reject missing or newer DBOS schema versions before runtime launch. Document the operational drain/rollback rule and distinguish this same-SDK test from an actual SDK/schema upgrade.
2. **Compatibility, operational cost, and verdict.** With a selected candidate SDK release, run a real two-build/two-SDK upgrade and rollback matrix against one isolated database. Add a reproducible workload that reports first-delivery and retry latency, workflow counts, checkpoint bytes, and deletion/retention effects, including immutable Argus evidence survival. Run it on a documented host/container configuration, retain results, and compare with the default path. Update ADR-0028 with an adopt/reject/defer verdict, supported rollout and retention policy, unresolved risks, and next action. If a second SDK or representative measurements are unavailable, record a defer/no-go verdict rather than claiming production compatibility. No threshold may be invented from a workstation result alone; production thresholds require representative baselines.

Each slice includes its tests, documentation, and check evidence. No docs-only or single-file issue is planned. The second slice depends on the first because cost is not a sufficient reason to adopt an unsafe rollout.

## Evaluation conclusion

The second slice pins SDK v1.5.0/schema 121 and v1.6.0/schema 123. It compiles two real workers, uses executable-derived versions, recovers and drains the old binary before the candidate migration, verifies preserved Argus evidence and candidate admission, then asserts that an old-binary rollback fails closed. Its serial comparison measures the actual synchronous and DBOS compositions; terminal checkpoint deletion retains a pending assessment and every catalog row. `make dbos-evaluate` and CI retain raw reports with provenance.

The evaluation verdict is **defer production adoption**: the upgrade guard makes in-place rollback unsupported, retention requires an explicitly owned policy, and workstation timings are not representative production baselines. The bounded evaluation has a conclusion; promotion prerequisites are not a queue of more evaluation micro-PRs. Continue product work with the established path. Any chosen production rollout is one separately reviewed M10 vertical slice with the recovery, privilege, retention and load evidence described in the verdict.

## Promotion boundary

Even a positive evaluation does not flip the default flag. Production promotion is a separate M10 vertical slice covering deployment, privileges, observability, recovery, retention, and rollback. A negative or inconclusive evaluation leaves the established path enabled and may remove the experimental runtime in a subsequent reviewed change. GitHub issues and the M10 milestone remain the delivery tracker; this plan records technical sequencing only.
