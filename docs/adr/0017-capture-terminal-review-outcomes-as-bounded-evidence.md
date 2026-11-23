# ADR-0017: Capture terminal review outcomes as bounded evidence

**Status:** Accepted  
**Date:** 2026-09-27

## TL;DR

Argus derives a terminal adaptation disposition from the closed GitHub pull
request, requires a compatible explicit reviewer reason, and captures the
complete bounded diff from its generated commit to the final reviewed head.
It abstains when the review is open, history diverges, or patch evidence is
incomplete. The result is portable learning evidence, not proof of correctness
or an automatic policy update.

## Context

ADR-0016 stops after creating an open draft PR. That preserves human merge
authority but leaves Argus unable to distinguish an unchanged merge, a merge
that required reviewer correction, and a rejected candidate. Merge state alone
also cannot explain why a candidate was rejected or show what a reviewer
changed.

Learning from review requires a trustworthy, reproducible observation. GitHub
comparison responses can omit patches for binary or oversized files and cap
file lists. Review branches may also be rebased or repointed. Treating partial
or divergent state as complete evidence would train later policy on an
incorrect causal story.

## Decision

### Correlate the full repair chain before provider access

The outcome application accepts the original `AdaptationProposal`, successful
`ValidationEvidence`, and `AdaptationReview`. It validates each document and
requires exact proposal, validation, policy, adapter, edit, test, repository,
and immutable base-revision correlation before reading GitHub.

### Derive disposition and capture reason separately

The provider supplies closed/merged state and the final head revision. Argus
derives exactly one disposition:

- merged at the generated head: `accepted-as-proposed`;
- merged after additional commits: `accepted-with-edits`; or
- closed without merge: `rejected`.

The caller supplies a bounded stable reason code. `approved` is valid only for
unchanged acceptance, `corrected` only for edited acceptance, and rejection
uses `incorrect-repair`, `unsafe-repair`, `no-longer-needed`, `superseded`, or
`other`. `other` requires a note. This prevents callers from rewriting provider
state while retaining human meaning that provider state cannot express.

### Require a complete, linear, bounded final diff

When the final head differs from the generated head, the GitHub adapter
compares those exact commits. The final head must be strictly ahead and the
generated commit must be both the comparison base and merge base. Every file
must have a known normalized kind and a complete unified patch within per-file,
file-count, and aggregate byte limits.

Argus returns incomplete-evidence or conflict errors for missing patches,
unknown kinds, comparison caps, divergent history, or mismatched PR identity.
It does not silently retain a partial diff.

### Emit portable evidence before adding storage or learning

`argus.dev/review-outcome/v1` contains the correlated identities, repository
and PR, generated and final revisions, terminal times, disposition, reason,
and canonical reviewer edits. Its deterministic outcome ID covers terminal
state, the explicit reason, and patches while excluding retry observation time.
This slice emits the document; durable storage,
aggregation, and policy learning remain separate decisions.

## Consequences

- Later evaluation can distinguish generated repairs that merged unchanged
  from those that needed human correction.
- Rejection reasons are explicit and machine-readable without pretending they
  can be inferred from GitHub.
- Binary, very large, or broad reviewer changes can cause capture to abstain;
  completeness is preferred over a misleading partial record.
- Rebased or merged-main histories are rejected because the generated-to-final
  edit cannot be isolated with the required causal boundary.
- A merge is retained as workflow evidence but is not treated as proof that a
  repair was semantically correct.

## Alternatives considered

### Treat merge as acceptance of the generated candidate

Rejected because reviewers can amend the branch before merge. It would
overstate Argus' success and discard the correction that is most useful for
learning.

### Accept GitHub's file list without patches

Rejected because filenames and line counts are insufficient to reconstruct or
classify the correction, and GitHub can omit or truncate evidence.

### Persist and update a model in the same slice

Rejected because capture, durability, aggregation, and policy promotion have
different failure and governance boundaries. Portable evidence permits those
choices to be reviewed independently.

## Security and operations

The capture command uses a repository-scoped token from the environment and
performs read-only GitHub calls. It verifies the cataloged opaque repository
identity before observing the PR. Requests, responses, patches, notes, and
inputs are bounded; tokens are never written to output. CI should retain the
emitted document immutably until durable ingestion exists.

## Validation

Application tests cover all three derived decisions, contradictory reasons,
and pre-provider correlation failure. GitHub HTTP tests cover terminal PR
observation, complete reviewer-diff normalization, open-review rejection, and
missing-patch abstention. Effect Schema and generated JSON Schema fixtures
cover portable contract structure. Architecture tests preserve the application
and adapter dependency direction, and the repository acceptance suite covers
formatting, static analysis, tests, race detection, vulnerabilities, and
licenses.
