# ADR-0038: Publish validated browser locator repairs for human review

- Status: Proposed
- Date: 2026-10-19
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers and browser test owners
- Supersedes: None
- Superseded by: None

## TL;DR

Only a complete browser locator validation may open a deterministic draft PR. Argus reloads immutable source, materializes the exact validated edit, and delegates publication to the existing GitHub adapter with separate source-read and PR-write credentials. Terminal review capture uses read-only observation and source access. A merged reviewer change is recorded as a bounded locator correction only when all other source bytes are unchanged.

## Context

ADR-0037 produces browser-specific proof and trustworthy rejection evidence. Validation alone does not authorize a source change. Review must preserve the human decision, while preventing a changed assertion or navigation flow from being classified as a corrected locator.

## Decision

Publication accepts `browser-locator-validation/v1` in `validated` status, confirms the failed attempt's GitHub test repository and immutable revision, reloads the file, checks the proposal preimage, and matches the materialized candidate SHA-256 to the validation. A browser-specific review ID and branch distinguish it from endpoint and field repairs. The existing GitHub provider adapter supplies idempotent draft creation, branch divergence checks, and partial-failure recovery. The draft body contains identifiers, test and source coordinates, and a six-run summary. Artifact references and raw browser structure stay in the validation evidence under its access controls.

The existing `adaptation-review/v1` and `review-outcome/v1` envelopes carry provider state without changing endpoint or field semantics. The browser validation ID and proposal ID bind them to their family-specific proof. Outcome capture checks these identities and the deterministic branch. For merged edits, the complete provider comparison must contain exactly one modified source file. A read-only source reader loads both generated and final immutable revisions; every byte outside the selected locator call must match, and the replacement must use the same supported call kind and role. Captured reviewer edits represent human decisions, not a new six-run proof. Rejected reviews retain their terminal provider disposition and reason.

Argus does not mark a draft ready, merge it, approve a visual baseline, or relabel an unsupported reviewer edit as a safe correction. The generic immutable review outcome store can persist the browser outcome because its repository, revision, decision, and bounded edit fields already cover this evidence.

## Compatibility and recovery

Existing review contracts and database tables keep their v1 meaning. Repeating publication uses the deterministic review ID and branch. A stale source, changed candidate digest, divergent provider branch, incomplete comparison, unsupported reviewer edit, or unavailable final source stops the operation. Terminal capture requires a human-supplied reason consistent with the provider-derived decision.

## Verification

The browser review fixture checks contract round-trip, exact candidate publication, stale source refusal, redacted draft metadata, accepted locator correction, assertion-change refusal, and trustworthy validation rejection. Existing GitHub adapter tests cover deterministic publication recovery and branch divergence. Architecture and command smoke gates cover the new composition points; browser, API, and database gates cover the integrated workflow.
