# ADR-0036: Propose browser locator edits from correlated structure

- Status: Proposed
- Date: 2026-10-18
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Browser locator diagnosis consumes one prior passing attempt and one later
  failed attempt for the same test revision, project, environment, and test.
- The browser adaptation policy may read the browser execution domain. Only
  contract and CLI adapters read files or translate its public documents.
- A candidate requires fresh artifact bytes, the same page URL, a unique prior
  target, and one current visible target linked by unchanged test ID or
  unchanged role and accessible name.
- The proposal changes one supported locator call only. It does not apply,
  validate, publish, or merge the patch.

## Context

Playwright failures can arise from application behavior, navigation, assertions,
timeouts, or a stale locator. A generic failed result cannot authorize a test
edit. Existing browser attempts already bind test identity, project,
environment, revision, and artifact SHA-256, but their accessibility artifact
was plain text and lacked element identity and visibility needed to distinguish
an equivalent target.

## Decision

The reference Playwright fixture emits a bounded `browser-structure/v1`
accessibility artifact, including the page URL, capture time, ARIA snapshot,
and supported elements' role, accessible name, test ID, and visibility. A
failed attempt is labelled `locator-not-found` only when Playwright reports a
missing `getByRole` or `getByTestId` target. That label is diagnostic, not a
repair authorization.

The `browserlocator` application policy accepts the exact bytes of a prior
passing and later failed attempt's accessibility artifacts. It validates the
attempts, test identity, SHA-256 binding, capture times, page URL, source
preimage, reviewed test-registry file mapping, supported source syntax, and
exact failure locator. It offers one
edit only when a stable identity maps a unique visible old target to a unique
visible new target. Hidden and duplicate elements count toward ambiguity.
Candidate output contains source digest and byte range, old and replacement
locator calls, structural target evidence, and both attempt/artifact identities.
An unsupported or unproved case yields a reasoned abstention.

The adaptation capability is allowed to depend on the browser execution
application model for this use case. Execution remains unaware of adaptation.
The CLI composes saved attempt conversion, filesystem access, and the policy;
the policy itself does not read files, execute Playwright, or use transport DTOs.
Public snapshot, diagnosis, and proposal contracts are versioned separately
from the existing execution-attempt v1 contract.

## Alternatives considered

### Rewrite on any Playwright locator timeout

Application regressions and navigation errors can also leave a locator absent.
Without a matching prior target and a stable identity, there is no safe edit.

### Ask a model for a selector

A model cannot establish the exact preimage, uniqueness, or target equivalence
required for this constrained class. It may assist a future review-first class,
but does not participate in this policy.

### Broaden to positional selectors or arbitrary CSS

`first`, `nth`, broad CSS, chained filters, and assertion edits can make a
test pass against a different target. They remain unsupported.

## Consequences

CI must retain the baseline and failed artifact bytes under its existing
browser-evidence access controls. The reference structural capture supports
simple role/name and test-ID cases; unsupported accessibility computations
cause abstention. Candidate generation alone does not establish a successful
repair. Reproduction, repeated repaired runs, negative controls, restoration,
and reviewer publication remain separate M07 steps.
