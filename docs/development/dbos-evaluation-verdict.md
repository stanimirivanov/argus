# Embedded DBOS evaluation: compatibility, cost, and verdict

## TL;DR

- **Defer production adoption.** Keep the synchronous path enabled and DBOS opt-in. This is a bounded evaluation conclusion, not a portfolio-wide workflow decision.
- The automated matrix builds SDK v1.5.0/schema 121 and v1.6.0/schema 123 independently. It restores the exact old binary to recover interrupted work, then migrates with the candidate SDK, verifies old evidence and new deliveries, and proves that the old binary rejects the upgraded schema.
- Exact redelivery preserves immutable business evidence but creates another DBOS workflow and two checkpoints. Retention is necessary; deletion does not immediately shrink PostgreSQL relation allocation.
- `make dbos-evaluate` owns setup, synthetic credentials, mock GitHub, migrations, workers, teardown, and raw JSON reports. No Compose, database URL, or real token is needed. A working Docker daemon and pinned Go/tool dependencies are still required.
- Before promotion, resolve upgrade/rollback and retention ownership, validate representative load, and prove backup/restore, privileges and operator recovery. Perfeng and Argus SRE require independent evaluations.

## Reproduce and retain evidence

Run `make dbos-evaluate` from the repository root. This is the complete race-enabled tagged suite, including the existing signature, durable acknowledgment, conflict, error-classification, disconnect and process-kill coverage. For non-instrumented timing only, use:

~~~sh
go test -tags=dbose2e -run '^TestDBOSEvaluationCost$' -count=1 -timeout=5m ./cmd/control-plane
~~~

Tests create isolated PostgreSQL 17.11 containers using Testcontainers, inject generated credentials directly into composition and child-process environments, and mock every GitHub request. On local Windows Docker Desktop, they select the Linux socket for Ryuk when no explicit/remote override is configured; the cleanup reaper stays enabled. Production migration files, database configuration and the root SDK pin are not changed.

Reports appear under ignored `.local/dbos-evaluation/` with unique filenames and are also emitted in verbose test output. CI retains only those JSON files for 30 days, including reports produced before another case fails. Absent reports mean missing evidence, not a pass. Source and manifest fingerprints, binary and SDK checksums, application versions, Go/platform/CPU counts, PostgreSQL image identity and server version distinguish runs. They contain synthetic metrics, not URLs, credentials, signed payloads or machine-specific paths.

The compatibility test checks that the two executable dependency graphs differ only in DBOS. The candidate uses an exact release and checksum, not `latest`, a local replacement, or a prerelease. A pinned `govulncheck` binary scan gates the candidate; the normal source-level vulnerability and license suite still gates the root graph. Binary scanning is additional evidence, not equivalent to source-level call-graph analysis. The candidate SDK retains the upstream MIT license and introduces no other compiled dependency change.

## Actual upgrade and rollback matrix

The candidate worker is the same Argus source compiled against SDK v1.6.0, with a **temporary Go overlay** changing only its exact schema guard from 121 to 123. This models a reviewed candidate-release guard transition. The checked-in production guard remains 121. Both test executables use their actual executable-derived application versions; the matrix does not supply `DBOS__APPVERSION`. This is not a test of arbitrary future Argus source changes or historical released binaries.

| Transition | Required observation | Meaning |
|:--|:--|:--|
| Kill old binary after change storage; restore exact old binary | Assessment completes without repeating change resolution | Exact-binary recovery works on schema 121 |
| Candidate before migration | Refuses 121; expects 123 | Startup cannot silently migrate or tolerate unreviewed schema |
| Explicit candidate SDK migration | Ledger reaches 123; every Argus catalog table has the same canonical-row digest | Checkpoint schema migration leaves business evidence intact |
| Candidate after migration | Retrieves the old SDK's completed workflow result; old delivery is an exact retry; new delivery completes mapped impact | Candidate reads old checkpoint results and existing evidence and admits new work |
| Old binary after migration | Refuses 123; expects 121 | In-place binary rollback is **not supported** |

Drain old work **before** upgrading the schema. Schema migration is not a license to terminate old workers with pending work. Restoring an old binary after the upgrade cannot recover that history through the current guard. Do not decrement the ledger, drop checkpoint tables, or relabel the candidate as the old version. A production rollback needs a separately validated recovery strategy preserving post-upgrade business writes; restoring a whole database backup can lose those writes and is not an automatic solution.

Upstream [v1.6.0 release](https://github.com/dbos-inc/dbos-transact-golang/releases/tag/v1.6.0) and its [migration sources](https://github.com/dbos-inc/dbos-transact-golang/tree/v1.6.0/dbos/internal/sysdb/migrations) were inspected on 2026-10-10. This one release pair establishes neither general backward compatibility nor shared-server safety across products.

The [retained compatibility report](../research/evidence/dbos-2026-10-10-compatibility.json) records actual binary hashes, SDK checksums, distinct application versions, completed-history retrieval and the candidate vulnerability gate. The [race-instrumented cost report](../research/evidence/dbos-2026-10-10-cost-race.json) is safety evidence with instrumentation overhead, not the timing baseline below. All three retained reports have the same source and root-module fingerprints.

## Workload and measurements

Each mode uses a fresh database, identical synthetic OpenAPI change, serial loopback HTTP, one warmup delivery/redelivery pair, 20 measured first/exact-redelivery pairs, and one injected assessment failure followed by recovery. Both compositions perform the real Argus migrations, signed webhook handling and immutable persistence. The synchronous comparison does not create a DBOS schema or runtime. Median and p95 use nearest rank; all chronological nanosecond samples are retained.

Assertions require exactly 40 metadata reads and 40 document reads in each measured batch, zero additional change-resolution reads during assessment recovery, and one change/impact pair per delivery. The DBOS retention control holds one real assessment pending, waits for terminal statuses, selects only this application's `SUCCESS`/`ERROR` workflow IDs, and deletes them through the SDK. It fingerprints **every** Argus catalog table before/after cleanup, checks that pending recovery remains possible, then redelivers a deleted-history delivery without repeating provider I/O or business writes.

`workflow_row_bytes` sums PostgreSQL row sizes for status/input/output tables; `step_row_bytes` measures operation-output rows. These are live logical-row estimates, not WAL, traffic or billing metrics. `schema_relation_bytes_including_indexes` includes physical table/index/TOAST allocation and does not promise immediate disk reclamation after DELETE. No VACUUM FULL, schema reset or production cleanup command is introduced.

The [raw non-race timing report](../research/evidence/dbos-2026-10-10-cost.json) was recorded on 2026-10-10 with Go 1.26.9, Windows 11 Home (host-reported build 10.0.22621), amd64 and four logical CPUs. Docker Desktop Engine 29.8.0 used a four-CPU Linux VM with 16,543,649,792 bytes of memory; PostgreSQL 17.11 containers had no additional per-container resource limits. Other acceptance/build activity ran on the same host, so scheduling was not controlled. The report retains the precise image identity and source/manifest fingerprints.

| Non-race loopback measurement (20 samples) | Synchronous | DBOS v1.5.0 |
|:--|--:|--:|
| First-delivery median / p95 | 34.97 / 143.70 ms | 59.83 / 96.09 ms |
| Exact-redelivery median / p95 | 15.07 / 34.49 ms | 39.60 / 82.48 ms |
| Measured-batch metadata / document reads | 40 / 40 | 40 / 40 |
| Recovery additional change-resolution reads | 0 | 0 |

The DBOS fixture retained 45 workflows and 89 checkpoints: warmup, 40 measured delivery attempts, failed/recovered attempts, and one pending control. Terminal deletion removed 44 workflows, leaving one workflow/one checkpoint and exactly the same Argus catalog-row digest. Live workflow-plus-step row bytes fell from 70,095 to 1,302, while allocated relation bytes stayed at 720,896. After pending completion and another exact redelivery, two workflows/four checkpoints remained, with no duplicate business effects. The synchronous fixture created no DBOS schema.

These are smoke-scale observations, not SLOs or capacity claims. The first-delivery p95 ordering differs from the median ordering: **do not claim a consistent slowdown, speedup or statistically significant effect** from this sample. Race instrumentation, host load, Docker VM scheduling, parser versions and cache state affect timings. A single fixed-order serial run cannot establish statistical significance or a production budget; representative repeated/concurrent workloads and repository sizes are required before promotion.

## Retention and adoption boundary

The supported **evaluation-only** cleanup operation selects an explicit terminal snapshot in an isolated database while retaining pending work and all Argus evidence. The SDK's `DeleteWorkflows` API also permits deletion of active workflows; it supplies no pending-work protection by itself. Do not expose it as an operator action without authorization, ownership, resume/delete concurrency rules, retention age, audit/export requirements and recovery tests. A fixed 30-day CI artifact expiry is unrelated to production workflow retention.

The short webhook path gains durable between-step recovery but also incurs checkpoint I/O, history per redelivery, schema ownership and exact-binary recovery obligations. **Defer**, rather than adopt or reject the library outright: retain the opt-in experiment and synchronous default. No shared workflow runtime, default-flag change, SDK upgrade or service extraction follows from this PR.

The next product-development slice can resume without another DBOS evaluation micro-task. If maintainers elect to promote durable coordination, use one substantive M10 production slice covering the chosen rollback strategy, migration/runtime identities, representative-load evidence, observable recovery and bounded terminal retention. Do not migrate Perfeng or Argus SRE merely because this Argus experiment passed.
