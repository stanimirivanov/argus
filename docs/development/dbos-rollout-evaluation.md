# DBOS evaluation rollout and rollback

## TL;DR

- This is an opt-in evaluation, not permission to enable DBOS in production.
- DBOS Go v1.5.0 identifies an application version from the executable's bytes by default. A newly built executable has a different version and does **not** resume pending workflows from its predecessor.
- Keep the old executable available and running until its pending work drains. If it was lost, restore that exact version to recover its work; merely disabling the flag or starting a new version does not drain its DBOS history.
- Run the privileged DBOS schema migration separately from runtime startup. Argus requires the exact pinned migration version at startup, including on rollback; do not assume that a DBOS SDK/schema upgrade is backward compatible with a still-running old executable.
- The container-backed rollout test proves version routing, exact redelivery, explicit migration replay, and rollback with one pinned DBOS SDK. A real two-SDK upgrade matrix and load/retention evidence are still required before production promotion.

## Version and evidence ownership

The webhook response remains synchronous and is acknowledged only after Argus's immutable change and impact evidence is present. Those Argus tables remain the business source of truth. The `argus_dbos_eval` schema contains execution checkpoints, not replacement business evidence. Its migration history is owned by the pinned DBOS library and is deliberately separate from Argus's checksummed SQL ledger during the evaluation.

By default DBOS computes its application version from the executable and application name. Its launch recovery selects pending workflows for the current application version. This is useful isolation during a mixed-version rollout, but it means replacing every old worker at once can strand unfinished old-version workflows. The `DBOS__APPVERSION` override is used only by the isolated test to emulate two releases; it is not an operator shortcut for relabeling a new executable as an old version. Doing so could replay old steps under incompatible code.

## Controlled evaluation sequence

1. Prepare an isolated database with the ordinary Argus migrations and the explicit DBOS evaluation migration. Start version A with `ARGUS_DBOS_EVALUATION=true`; establish that webhook delivery and impact evidence are durable.
2. Before replacing A, inspect unfinished DBOS workflow counts by application version. Keep A available while B starts and new deliveries enter B. Do not disable or remove A solely because B responds to new webhooks.
3. Observe the old-version count fall to zero. Investigate each workflow that does not drain; an exact redelivery can complete Argus evidence on B but does not erase A's pending checkpoint. Do not delete that history as a substitute for recovery.
4. If B must be rolled back, stop admitting new B requests, restore the exact A executable, and check both versions' unfinished work. B's unfinished work likewise requires B to recover. Turning the evaluation flag off routes future requests to the default path, but does not recover or delete existing DBOS workflows.
5. For a DBOS SDK upgrade, stage the privileged migration only after separately verifying that the old SDK can still read the upgraded schema for the entire mixed-version window **and** planning the Argus exact-version guard transition. Otherwise drain all old-version workflows before applying the new SDK migration. Runtime remains `SkipMigrations=true`; Argus also checks the ledger version because DBOS v1.5.0 itself tolerates a schema newer than its required version. An older Argus binary therefore fails closed after a new SDK migration, even if DBOS alone would start. No production rollout is authorized by the current same-SDK test.

The read-only inspection query for the isolated evaluation database is:

~~~sql
SELECT application_version, status, count(*) AS workflows
FROM argus_dbos_eval.workflow_status
WHERE status IN ('PENDING', 'ENQUEUED', 'DELAYED')
GROUP BY application_version, status
ORDER BY application_version, status;
~~~

Do not treat an empty result alone as a production readiness signal: investigate terminal failures, verify Argus evidence, and apply the retention policy once it has been measured and approved.

## Automated evidence and limits

`go test -tags=dbose2e -run '^(TestDBOSApplicationVersionRolloutAndRollback|TestDBOSSchemaRolloutFailsClosed)$' -count=1 -timeout=10m ./cmd/control-plane` uses isolated Testcontainers PostgreSQL instances. It kills a version-A worker after the immutable change is stored but before impact, confirms that a version-B worker leaves A's pending checkpoint untouched while completing a fresh delivery, then restores A and checks recovery without repeating provider resolution. It also re-runs the explicit migrations against that history and checks exact redelivery after recovery. A separate disposable-database case removes the DBOS schema, verifies that opt-in startup fails and the established path remains usable, then simulates a newer ledger version and checks that Argus refuses it before restarting on the pinned version.

The test uses two application-version labels within the same test binary and pinned SDK. It does not prove code-shape compatibility between two independently built Argus releases, a DBOS SDK/schema upgrade, safe mixed-version deployment against a shared production database, or acceptable storage cost. Those remain promotion gates, not implied passes.
