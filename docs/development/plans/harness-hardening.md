# Harness hardening execution plan

## TL;DR

- Deliver one reviewable slice at a time through the issue, branch, and pull-request workflow owned by the reviewer.
- First, detect accidental drift in CI job names, runners, and verification commands with an offline policy check.
- Later slices add a workflow retry regression test and a task-scoped verification runner. Host-enforced isolation is a deployment follow-up.

## Outcome and boundaries

The harness should give contributors fast, actionable feedback and make the
pull request's checks easy for a reviewer to inspect. The current merge
workflow is manual: the reviewer waits for all applicable CI jobs and checks
their results before merging. The repository has no required-status merge
rule, and this plan does not change that setting. An in-repository policy test
detects accidental workflow drift; it cannot stop someone from disabling its
own workflow or merging despite a failure.

The existing Go dependency test, contract checks, and `make verify` remain
authoritative. This work does not add an unmeasured performance threshold.
Production telemetry, representative load baselines, and operational
thresholds remain part of M10.

## Ordered pull-request slices

### PR 1: Detect validation workflow drift

- **Objective & scope:** Add an offline check that compares the validation workflow with a versioned policy for job names, supported runners, and the Make command behind each job. Keep the check in the ordinary Go suite and expose it as a clear CI job for manual review.
- **Target layers / files:** `tools/ci-policy-check/`, `.github/workflows/validate.yml`, `Makefile`, and the harness and developer guides.
- **Contracts & boundaries:** The existing validation jobs and their Linux/Windows matrix remain visible under stable names. The check rejects missing pull-request triggers, unsupported matrix expansion, skipped or error-tolerant required steps, Make execution overrides, and renamed or removed Make targets. A reviewer still confirms actual CI results before merge.
- **Harness gate:** `make ci-policy-check`, `make docs-check`, `make verify`, `make validate`, and `git diff --check` pass. The issue and branch are created by the reviewer after this slice is handed off.

### PR 2: Prove change-workflow retry safety

- **Objective & scope:** Add a focused synchronous workflow regression test for durable acknowledgement, exact retry after analysis failure, completed replay, and conflicting payload reuse. Correct any defect the test demonstrates in the same slice.
- **Target layers / files:** `internal/change/workflow/service_test.go` and owning implementation only if a defect is demonstrated.
- **Contracts & boundaries:** A delivery is acknowledged only after impact persists; an exact replay returns stored evidence without repeated analysis; a conflicting replay preserves the original evidence.
- **Harness gate:** Focused workflow tests and `make validate` pass on the PR branch.

### PR 3: Record task-scoped agent verification

- **Objective & scope:** Add audit and implementation task profiles with a runner that validates requested paths and named offline Make checks, limits child environment variables, records bounded output and exit status, and prints failure output for self-correction.
- **Target layers / files:** `tools/agent-harness/`, `AGENTS.md`, `docs/README.md`, and the harness guide.
- **Contracts & boundaries:** The runner checks a host scope declaration but cannot attest to or provide filesystem and network isolation. Deploying those restrictions in a trusted agent host is a separate M10 follow-up.
- **Harness gate:** Runner tests, `make docs-check`, and `make validate` pass. The documentation must distinguish checked declarations from host-enforced isolation.

Each slice is handed off before the next begins, remains independently
reviewable, and leaves the repository buildable. GitHub issues and milestones
track delivery; this plan records technical sequencing.

## Promotion and recovery

For each slice, report passing, failing, and unavailable checks exactly. A
reviewer confirms all applicable CI jobs before merge; a green policy check
alone does not establish that the other jobs passed. Changes to job names,
runners, or commands must update the workflow and policy together. If a new
sensor is noisy, fix its inputs and diagnostics before relying on it in
review.

The task runner's future host scope declaration is an integration contract,
not proof of enforcement. The host integration follow-up must demonstrate
actual writable-root and network restrictions before anyone claims task
isolation. Keep existing GitHub credential compatibility behavior until a
separately reviewed M10 migration verifies scoped token distribution.
