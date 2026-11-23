# ADR-0087: Verify reviewed analysis conformance

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Bind exact report bytes to admitted publication and protected native review.
- Require complete policy rules/sections and approved baseline associations.
- Preserve independent outcomes without recalculating statistics or overall PASS.

## Decision and consequences

PR9d2b2c adds a callable verifier over opaque admission/review values and bounded
report bytes. Recheck request/admission/native scope so a valid review cannot be
transplanted. Verify original byte size/SHA-256, then decode the native report and
match Run/test/policy/mode, reviewed producer/contracts provenance and candidate Run.
Reject self-references and credential-bearing/mutable artifact addresses.

Require one evaluation per policy rule and exact selectors/configured sections.
A quality PASS needs measurements explicitly required by configured quality checks.
Every reference must equal the admitted baseline; policy baseline ID/version must
match it. Missing reference may remain inconclusive, while absent native baseline
allows no comparison evidence. Numeric artifact sizes compare with bounded exact
rationals; their original syntax and report bytes remain untouched.

Creation cannot precede retained acceptance or follow publication. Lifecycle
recovery exposes only request time as an independent lower bound; do not invent
admission time from later snapshots. Allow valid late/terminal evidence without
lifecycle projection. Return owned opaque conformance with original admission,
review and report pins, or zero evidence/safe errors on failure/cancellation.

This checks associations and section completeness, not numerical quality/SLO/
regression decisions, statistical effects or an aggregate verdict. PR9d2c must
authenticate retrieval/origin/redirects and response limits; PR9d3 owns durable
decision projection. No network, clock, persistence, dependency, migration,
execution/baseline approval, blocking or runtime activation is added.
