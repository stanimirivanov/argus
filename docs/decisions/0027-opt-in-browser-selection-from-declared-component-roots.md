# ADR-0027: Opt in to browser selection from declared component roots

- Status: Proposed
- Date: 2026-10-03
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Offer `select -family functional-ui -ui-impact components` as an opt-in source-change policy.
- Resolve every changed path, including rename/copy predecessors, against the
  immutable base catalog's declared component roots and capability mappings.
- Fall back to all browser tests for any unmapped path or truncated file list.
- Emit execution-manifest v3; existing API v1 and OpenAPI-only UI v2 retain
  their exact meanings and default behavior.

## Context

ADR-0025 permits browser narrowing only when all changed files are analyzed
OpenAPI documents. This correctly protects mixed changes, but a UI-only source
change always falls back despite the repository descriptor already declaring
component roots and capabilities. The Playwright inventory checker introduced
by ADR-0026 checks test identity locally, not source impact.

## Decision

The `select` command MAY use `-ui-impact components` only with
`-family functional-ui`. This policy reads the verified immutable change set
and the catalog snapshot at its base revision. For each changed path and each
rename/copy predecessor it matches all component roots on exact path-segment
boundaries and unions their declared capabilities. Overlapping roots are
intentionally additive; neither longest-prefix precedence nor a heuristic
route parser is used. The source roots are descriptor declarations, not
inferred ownership.

Targeted selection requires a nonempty, complete provider file list, every
path mapped to at least one declared component, and a nonempty capability
union. Otherwise all cataloged UI candidates are required. Invalid durable
catalog or change evidence fails closed. An unmapped path may not be ignored
even when another file is mapped. Patch content is neither needed nor read.

The policy derives a `component-root-impact/v1` projection at selection time
from these immutable inputs. It does not persist a separate impact artifact.
Execution-manifest v3 identifies this producer and the new policy. This is
selection evidence only: it does not execute browser code or authorize a
release gate. A v2 consumer must reject v3. Existing default v1 and explicit
UI v2 invocations are unchanged.

## Alternatives considered

### Change v2 to accept component roots

- Benefit: One fewer contract version.
- Risk: Silently changes the scope of an existing OpenAPI-only policy.
- Rejected because compatibility requires an explicit version boundary.

### Infer UI routes from source paths or Playwright test names

- Benefit: Fewer descriptor declarations.
- Risk: Convention-dependent false negatives and opaque decisions.
- Rejected until route-to-capability evidence has its own validated contract.

### Require a separately persisted component-impact document first

- Benefit: A standalone audit artifact.
- Cost: Extra write path and migration for a deterministic projection of
  already immutable evidence.
- Deferred until a consumer needs independent impact storage or history.

## Consequences

Repositories with complete component declarations can narrow UI execution for
source-only changes. Incorrect declarations can create false negatives, so
descriptor review remains a trust boundary and omissions retain the later
full-suite obligation. Unknown and global changes fall back conservatively.
This policy does not prove that Playwright test collection matches declarations;
the separate checker must be run in the test repository.

## Compatibility and migration

No database migration or descriptor change is required. The opt-in flag emits
v3 only; omitting it preserves v1 or v2. Functional API planning and execution
continue to reject browser manifests. Consumers MUST handle v3 explicitly.

## Security and operations

The policy performs read-only database queries and never runs repository code.
All changed paths come from verified provider evidence. Full-suite fallback is
used for uncertain coverage. Database credentials remain environment-only.

## Validation

Go tests cover segment boundaries, rename predecessors, truncated and unmapped
paths, provenance, and deterministic decisions. Effect Schema and Go contract
tests prove v2/v3 noninterchangeability. Repository validation checks schema
generation and architectural boundaries.
