# ADR-0016: Publish validated repairs as idempotent draft pull requests

**Status:** Accepted  
**Date:** 2026-09-27

## TL;DR

Argus may publish a repair only after correlating a valid proposal with a
successful validation proof and reconstructing the same candidate from source
at the proposal's immutable revision. GitHub publication uses a deterministic
branch, one preconditioned file update, and an open draft pull request. Exact
retries recover the same review; divergent provider state is a conflict. Argus
does not mark the PR ready, merge it, delete it, or infer reviewer approval.

## Context

ADR-0014 produces a constrained endpoint edit and ADR-0015 proves that the
unchanged test fails, the exact candidate passes, and a negative control fails.
Those portable documents still do not put a reviewable diff in front of a test
owner. A CI retry can also stop after branch creation or commit creation, so a
naive publisher can create duplicate branches and PRs or overwrite human work.

The next M06 boundary must perform an externally visible write while preserving
the distinction between validation and human approval. It must distrust local
files, repository coordinates, stale provider state, and repeated invocations.

## Decision

### Reconstruct the candidate before external writes

The review application requires both the proposal and validation-evidence
documents. It correlates proposal, validation, policy, test, adapter, and exact
edit identities. Through a consumer-owned gateway, it loads the proposed file
from the immutable test revision, verifies the full-file SHA-256 and byte-span
preimage, applies only the validated replacement, and checks the resulting
digest against validation evidence.

Provider calls begin only after these checks succeed. The candidate is never
taken from an untrusted workspace or accepted merely because a local diff
looks equivalent.

### Use deterministic, guarded GitHub state

The first provider is GitHub. Before reading or writing content, the adapter
requires the current owner/name coordinates to resolve to the cataloged opaque
repository ID. It creates
`argus/endpoint-repair-<proposal-prefix>` from the proposal's immutable test
revision and updates only the proposed file using the current Git blob SHA as
an optimistic-concurrency precondition.

The pull request targets an explicit caller-selected base branch and is always
created as a draft. Its body contains proposal and validation identities,
endpoint and test provenance, ordered validation outcomes, and source digests.
The portable `adaptation-review/v1` result records the provider-confirmed base,
head revision, PR identity, draft state, and publication time.

### Make retry recovery strict

An exact retry reuses the deterministic branch. If the validated candidate is
already committed, publication continues to PR creation. If the matching open
draft already exists and its head contains the validated candidate, Argus
returns that review without another write.

A branch containing other bytes, a PR with another base or head, a non-draft
PR, or a closed PR is `ErrReviewConflict`. Argus does not force-update, reopen,
redraft, delete, or replace provider state. Concurrent create/update conflicts
also fail closed unless a subsequent read proves the exact intended state.

### Keep merge and outcome interpretation outside publication

Publication stops at an open draft. It has no merge, auto-merge, ready-for-
review, approval, label, comment, or branch-deletion operation. A later M06
capability will ingest an explicit reviewer outcome, stable reason code, and
the final edited diff. Merge state alone will not be treated as proof that an
unmodified Argus candidate was accepted.

## Consequences

- A successful validation can now become a concrete, small review artifact.
- Retried CI jobs recover from branch/commit/PR partial completion without
  duplicating an exact review.
- Deterministic branch names intentionally serialize one proposal; unexpected
  use of that branch becomes a visible conflict.
- The GitHub token requires contents and pull-request write access in the test
  repository, increasing the importance of repository-scoped credentials.
- A base branch that advanced after the immutable test revision can produce a
  normal GitHub merge conflict for a human to resolve; Argus does not rebase a
  validated candidate onto unvalidated bytes.
- Provider publication is not yet durably recorded by the control-plane store;
  the emitted contract and GitHub state are the current handoff evidence.

## Alternatives considered

### Push the disposable validation checkout

Rejected because its lifecycle and cleanup assumptions make it an unsuitable
source of truth, and other files could have changed during dependency setup or
test execution.

### Use a random branch on every retry

Rejected because retries after partial failure would create duplicate commits
and pull requests with no stable recovery key.

### Force-update a deterministic branch

Rejected because it could erase reviewer edits or unrelated provider state.
Unexpected bytes are a conflict requiring human disposition.

### Open a ready-for-review or auto-merge pull request

Rejected because validation proves only the narrow mechanical repair. Test
owners retain review and merge authority until a separately governed autonomy
policy is introduced.

## Security and operations

The token is accepted only from `ARGUS_GITHUB_TOKEN`, never a CLI flag, output,
or log field. GitHub Enterprise endpoints must use HTTPS except loopback test
servers. Deployments should use a short-lived, repository-scoped credential
with only contents and pull-request write permission. The command has a bounded
timeout and bounded request, response, source, title, body, and commit sizes.

Operators can retry the same command safely while the draft remains open and
unchanged. Conflicts require inspecting the deterministic branch and PR; Argus
does not perform automatic cleanup or destructive rollback.

## Validation

Application tests cover evidence correlation, preimage reconstruction, exact
candidate materialization, and rejection before provider writes. HTTP adapter
tests cover the complete GitHub branch/commit/draft-PR sequence, exact retry,
stable repository identity, and divergent-branch rejection. Contract fixtures
prove draft-only structural compatibility in Effect Schema and generated JSON
Schema. The repository acceptance suite covers architecture, formatting,
static analysis, tests, race detection, vulnerabilities, and licenses.
