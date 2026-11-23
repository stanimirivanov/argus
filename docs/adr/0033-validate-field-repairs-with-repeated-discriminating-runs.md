# ADR-0033: Validate field repairs with repeated discriminating runs

- Status: Proposed
- Date: 2026-10-17
- Milestone: M06 - Validated adaptation
- Deciders: Argus maintainers and functional API test owners
- Supersedes: None
- Superseded by: None

## TL;DR

Compatible request and response field proposals require two unchanged failures,
two candidate passes, and two failures after replacing only the field reference
with an Argus-derived unknown identifier. Source restoration is verified after
every run. Trustworthy repeated policy mismatches are separate rejection
evidence; execution disagreement and infrastructure failures are not learning
labels. Only six-run success permits a guarded draft review.

## Context

ADR-0032 established explicit field identity and proposal-only output. The
endpoint validator's one-run-per-phase protocol and invalid URL control cannot
prove that a test responds to a JSON field rename. A single lucky pass can also
hide flaky behavior, and a response assertion may pass for both the candidate
and an unrelated field. Field validation needs a distinct compatibility
boundary before publication.

## Decision

Argus derives an identifier named
`__argus_negative_field_<first-12-proposal-id-characters>` and substitutes it
for the exact proposed field span. The unchanged, candidate, and negative
variants execute in that order, each twice. Both outcomes in a phase must
agree. The required sequence is `failed, failed, passed, passed, failed,
failed`. Every invocation carries the proposal, validation, test, phase,
repeat ordinal, adapter, and materialized source digest. The adapter cannot
choose a control, another test, or the expected value.

Validation uses an explicitly disposable checkout at the immutable test
revision. It checks the complete preimage and exact byte span, restores source
after each variant execution with a non-cancelled deadline, and verifies the
original digest before emitting evidence. A source mutation, restoration
failure, malformed response, adapter `error`, process failure, timeout, or
repeat disagreement produces no successful or rejected policy evidence.
Consistent original-pass, candidate-fail, and negative-control-pass results
produce `field-validation-rejection/v1` with the complete trustworthy run
prefix; the command still exits nonzero.

Publication requires `field-validation-evidence/v1`, correlates it with the
field proposal, reconstructs the candidate from immutable source, and verifies
the candidate digest before using the existing idempotent GitHub publisher.
It creates only an open draft on a deterministic field branch. Terminal capture
verifies field review identity and uses the existing complete reviewer-diff,
reason-code, and immutable `review-outcome/v1` persistence policy. Human merge
remains separate from semantic correctness.

## Compatibility and recovery

All field request, result, evidence, and rejection documents have separate
versions. Endpoint validation evidence and rejection v1 retain their original
three-run meaning; endpoint commands reject field documents. Shared draft
review and terminal outcome envelopes describe provider state and review edits
without asserting a repair class. Their existing PostgreSQL retry/conflict
semantics apply to field outcomes without a schema change.

An interrupted validation may leave only the disposable checkout dirty; the
caller must discard it. Exact publication retries recover the same draft, while
divergent branch or PR state requires operator review. A changed reviewer
reason or patch conflicts with an already stored outcome rather than replacing
learning evidence.

## Alternatives considered

- Reuse endpoint evidence: rejected because its URL control and three runs have
  different semantics.
- Accept one pass per phase: rejected because one transient result can publish
  an unproven candidate.
- Treat repeat disagreement as rejection: rejected because disagreement is an
  execution-quality problem, not evidence that the candidate is wrong.
- Merge or promote after validation: rejected because a narrow mechanical
  proof does not authorize human review decisions or correctness claims.

## Verification

Service tests cover both field directions, each policy rejection, repeat
disagreement, cancellation restoration, source mutation, and restoration
failure. The disposable Playwright campaign executes both directions and a
false-healing response control. Draft and outcome tests cover proof gating,
reviewer edits, explicit rejection reasons, and pre-provider correlation.
`make validate`, `make db-validate`, and `make api-validate` remain the merge
gates.
