# ADR-0037: Validate browser locator repairs with artifact-bound runs

- Status: Proposed
- Date: 2026-10-19
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers and browser test owners
- Supersedes: None
- Superseded by: None

## TL;DR

Browser locator proposals require two reproduced locator failures, two repaired passes, and two negative-control locator failures in the same project, environment, and test revision. Every run uses the existing browser execution protocol and verified trace, screenshot, and accessibility artifacts. The validator restores the original source after every run. Only a complete proof or two agreeing policy mismatches become immutable evidence.

## Context

ADR-0036 permits a narrow proposal based on prior and current structural evidence. A proposal does not establish that the original failure is reproducible, that the patch preserves the test oracle, or that a passing result is stable. Endpoint and field validation use different controls and cannot give their evidence contracts browser meaning.

## Decision

Validation runs in an explicitly disposable checkout at the failed attempt's immutable test revision. The candidate changes only the proposal's byte span. The negative control replaces that same locator with `__argus_negative_locator_<first-12-proposal-id-characters>`, preserving its role or test-ID call shape. The unchanged, candidate, and negative variants run in that order, each twice. The required outcomes are `failed, failed, passed, passed, failed, failed`; both failures in a successful phase must be `locator-not-found`.

The browser adapter request and result remain `browser-adapter-request/v1` and `browser-adapter-result/v1`. Argus generates a deterministic attempt ID per validation phase and repeat, verifies exact test, project, environment, revision, adapter version, result membership, and artifact bytes, then retains the normalized attempt in `browser-locator-validation/v1`. The public document flattens attempts through the existing browser execution contract adapter. The domain model keeps request and result distinct.

Source restoration uses a non-cancelled bounded context after every run. Mutated source, restore failure, missing or tampered artifacts, malformed results, timeouts, cancellation, run overlap, and repeat disagreement produce no validation or policy-rejection evidence. Two agreeing original passes, candidate locator failures, or negative-control passes produce a trustworthy rejection prefix. Other failure classes, including application and assertion failures, are execution-quality errors rather than repair labels.

An additive PostgreSQL table stores either complete evidence or a trustworthy rejection under a deterministic validation ID. Exact payload retries are no-ops; divergent payloads conflict. The table stores artifact references and checksums, not artifact bytes. The CI worker must retain the referenced bytes under its existing artifact access controls. Publication and reviewer authority remain later work.

## Compatibility and recovery

No existing endpoint, field, or browser execution contract changes meaning. The browser validation command requires a disposable checkout, matching adapter environment, retained artifact directory, and database URL. An interrupted run may dirty only the disposable checkout; callers discard it. A database write failure prevents evidence emission, and a retry may replay the same complete evidence.

## Alternatives considered

- Reuse field validation evidence: rejected because field identity, failure class, and negative-control semantics differ.
- Accept one passing candidate run: rejected because a transient pass can hide a flaky locator.
- Omit artifact verification: rejected because references alone do not establish structural or visual evidence integrity.
- Publish automatically: rejected because validation does not grant review or merge authority.

## Verification

Unit tests cover six-run success, policy rejections, flaky disagreement, product failures, artifact failures, cancellation, source mutation, and restoration. Browser integration executes the real Playwright fixture. PostgreSQL integration checks exact retry, conflict, and restart. `make validate`, `make db-validate`, and `make ui-validate` are the merge gates.
