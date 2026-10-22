# ADR-0051: Gate staged promotion by owner policy

- Status: Proposed
- Date: 2026-10-22
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

Staged API/browser execution and observational safety reports exist, but neither
establishes an acceptable owner risk envelope. Arbitrary report scalars, sparse
cohorts, stale decisions or controls reused as prospective evidence cannot
justify automatically replacing independent full controls with remaining work.

## Decision

Introduce reviewed repository/family promotion policy and immutable decision v1.
Recompute chronological selection/calibration/cost from original archives and
prospective shadow metrics from complete original runner inputs. Require exact
portfolio, model and calibration identities, distinct historical control IDs,
shadow execution after replay completion and explicit owner thresholds for
sample/failure counts, empirical miss rate, calibration error/drift, estimated
duration/resource work and expiry. Do not invent policy defaults.

The gate is a stateless current-evidence evaluation. Missing or breached evidence
returns a reason-coded shadow decision. API and browser planners require a
trusted digest-pinned current decision and matching reviewed policy for staged
requests; otherwise they return shadow plans. Malformed documents fail without
output. The planning timestamp must equal the evaluation timestamp, preventing
reuse at a later evaluation instant even within the expiry window.

CI authenticates owner review, original evidence, exact current time and complete
archive membership. A digest proves byte identity, not authorization. Regenerate
the decision for every planning invocation and include newly available evidence;
there is no autonomous filesystem watcher or durable promotion state.

## Consequences and compatibility

**BEHAVIOR-ADJACENT:** previously explicit `-mode staged` could build an
experimental plan without promotion evidence. It now falls back to shadow when
authority is absent or inapplicable. V1 selected/full behavior and v2 runner
formats stay compatible; existing immutable experimental attempts remain valid.
Runners still consume trusted reviewed plans, not untrusted caller authority.

Authority is deliberately limited to one exact evaluated portfolio. A new
revision/portfolio/model requires another review and evidence evaluation; a
matching repository/family alone cannot transfer approval. Mixed-scope plans
fall back entirely rather than partially promoting unproven groups. Neither
decision nor subset changes release gates or removes the exact remaining-later
obligation. The owner policy in this slice cannot opt out of that obligation.

Empirical miss rate is misses divided by observed failing candidates, not a
statistical upper bound or future guarantee. Repeated executions are exposures,
not independent samples. Zero failures deny promotion. Cost remains estimated
duration/resource units, not billed spend. M10 owns deployment and alerting.

## Verification and rollback

Test both executable families, boundary thresholds, sample counts, missing or
late evidence, wrong identities, temporal overlap, reused controls, expiry and
re-evaluation after breaches. Verify exact CLI portfolio/model provenance and
planner fallback. Run `make generate-contracts`, `make predictive-validate` and
`make validate`. Rollback removes authority configuration and requests shadow;
no database migration or historical-evidence rewriting is required.
