# Schedule an offline predictive portfolio

## TL;DR

- Use the [chronological archive](predictive-replay.md) and the same complete [reviewed constraints](predictive-cost.md) as the cost estimator.
- Run `go run ./cmd/predictive-portfolio -index index.json -constraints constraints.json -budget-ms 600000`.
- Inspect required coverage, selected timings, omissions, fallback reasons and budget overruns. This is offline evidence; it cannot dispatch tests or authorize production adoption.

## Selection and timing

The positive integer budget is milliseconds per held-out episode, at most 86,400,000. The scheduler reuses frozen training-only p90 duration estimates and the existing [abstention gate](predictive-abstention.md). Held-out control labels and durations never influence scheduling.

An eligible episode admits every archived `RUN_REQUIRED` test and its transitive prerequisites first, in stable ID/dependency order. Mandatory coverage overrides budget. If it exceeds budget, mode is `required-over-budget`, with no optional additions.

Remaining candidates are ranked by calibrated failure probability divided by estimated test duration, with ascending stable ID ties. A first pass admits candidates whose unselected prerequisite closure introduces an uncovered decision-time capability. A second pass fills available budget in the same risk/cost order. An oversized closure is skipped without blocking smaller candidates. Capability names come from immutable observations; they do not alter test identity or existing archive contracts. This greedy baseline does not promise maximum coverage or a global optimum.

Every admission uses the complete marginal **serial** cost, including prerequisite tests and setup once per newly used environment. Selected tests execute one at a time in dependency order; setup is charged before the first test in each environment. This obeys reviewed resource capacity without assuming concurrency. Reported start/end offsets are estimated milliseconds from stage start, not actual runtime guarantees. Lower bounds from the cost report are never treated as proof of budget feasibility.

## Safety, evidence and limits

Abstention selects the complete control set regardless of budget. Optimization is bounded to 1,000,000 dependency visits, closure entries and capability inspections per episode. Reaching that limit discards the partial selection and produces full control with `fallbackReason: "optimizer-limit"`. The bounded reviewed graph still supplies a serial fallback schedule. Full-control reports leave `coveredCapabilities` empty because diversity was not evaluated; this does not mean the tests cover no capabilities.

The additive `argus.dev/predictive-portfolio/v1` report contains:

- `budgetMs`, complete `costs` (including the exact review file SHA-256), and complete `abstention` evidence;
- an episode mode, fallback reason (`abstention`, `optimizer-limit`, or empty), `estimatedDurationMs` and `overBudget`;
- selected test IDs, admission reasons (`required`, `dependency`, `diversity`, `ranked`, `fallback`) and serial offsets;
- required transitive closure, explicitly omitted candidate IDs, and sorted covered capabilities.

Selected and omitted IDs partition every reviewed candidate. Full-control requires all candidates. The portable validator checks this partition, mandatory closure, timing, dependencies, capacity and fallback consistency. The archive remains necessary to audit original mandatory decisions and capability mappings. Review provenance is declared; source CI must authenticate it.

Invalid budgets, archives or reviews emit no partial output. Both review input and complete output are bounded to 16 MiB. Output size failure is explicit; no selected test is silently truncated. File I/O failures are returned to the caller. There are no network calls, persistent state or retry side effects.

This PR does not implement live execution, parallel optimization, promotion, or the remaining-later queue. Omitted IDs must feed the explicit remaining/full-suite policy in the subsequent execution slice; the offline report is not permission to drop them. See [ADR-0046](../decisions/0046-schedule-diverse-budgeted-portfolios.md). Rollback removes the additive command and contract without data migration.
