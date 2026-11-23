# ADR-0034: Persist reviewed browser discovery before targeted selection

- Status: Proposed
- Date: 2026-10-18
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers and browser test owners
- Supersedes: None
- Superseded by: None

## TL;DR

Store immutable Playwright project inventory with explicit route and component
mappings. Targeted UI selection requires complete, fresh discovery at pinned
test revisions, reviewed mappings at the catalog revision, and complete changed
paths. Any missing coverage widens selection to all declared browser tests.

## Context

ADR-0026 checks Playwright inventory locally but cannot reuse it as durable
selection evidence. ADR-0027 permits component-root selection from the
descriptor but does not record route-to-test relationships. Browser tests may
appear in several projects while retaining one stable test key. Titles, tags,
and runner IDs cannot safely stand in for reviewed mapping policy.

## Decision

`browser-discovery/v1` binds a stable observation ID to one immutable catalog
snapshot, collection time and expiry, collector version, every Playwright UI
suite's test repository and revision, and project variants. A separate reviewed
mapping set names source roots and stable test identities. Component roots must
match the descriptor; routes carry an explicit URL pattern and one consistent
root per route key. Incomplete collection or mappings can be persisted for
diagnosis but confer no omission authority. Exact observation retries converge;
different content under the same ID conflicts.

The `playwright-catalog` checker can emit one suite observation from an
unfiltered Playwright list report. CI must verify checkout SHAs and assemble all
suite observations and reviewed mappings before ingestion. The collector's
revision flags are claims, not verification of the checkout or mapping review.

The browser-owned selection policy reads verified changed paths including
rename and copy predecessors. Reviewed CI bindings name every intended test
checkout. Targeted selection is allowed only when the discovery is complete,
unexpired, revision-compatible, and every changed path is mapped. An unmapped
path, missing observation, truncated file list, stale mapping, or test revision
mismatch requires all tests. Omitted tests retain a full-suite obligation. The
manifest references the immutable discovery digest and is distinct from
functional API execution manifests.

## Recovery and compatibility

Use a new observation ID for corrected discovery. Do not rewrite an existing
row or reinterpret an old manifest. A missing observation leads to a full
browser selection; corrupted persisted evidence is an explicit error. Existing
UI v2/v3 policies retain their meanings, and the functional API runner rejects
the new browser manifest. Execution, CI planning, and browser artifact evidence
are separate M07 slices.

## Verification

Domain tests cover duplicate project variants, undeclared or missing tests,
contradictory routes, incomplete collection, stale and mismatched revisions,
unmapped paths, and full-suite fallback. PostgreSQL integration covers
retry/conflict, restart reads, and the additive migration. `make validate` and
`make db-validate` are merge gates.
