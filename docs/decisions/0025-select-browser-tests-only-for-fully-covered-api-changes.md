# ADR-0025: Select browser tests only for fully covered API changes

- Status: Proposed
- Date: 2026-10-02
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Select cataloged `functional-ui` tests from explicit capability mappings
  only for OpenAPI-only changes with complete semantic impact.
- Compare the immutable provider changed-file set with analyzed documents;
  any other file or truncated list requires every UI candidate.
- Publish a separate `execution-manifest/v2`; keep functional API v1 unchanged.
- Browser discovery, UI route/component analysis, execution, and release-gate
  authority remain future work.

## Context

The catalog already accepts functional UI suites with stable declared test IDs
and capability mappings, but the initial selection policy and manifest are
functional-API-only. The existing OpenAPI analyzer can prove affected
capabilities for API documents, yet its `complete` status does not mean a
simultaneously changed browser component was analyzed. Using that status alone
to omit browser tests would create false confidence.

## Decision

The `select` command defaults to the unchanged functional API v1 behavior.
`-family functional-ui` selects cataloged UI candidates from the same approved
base-revision descriptor and emits `argus.dev/execution-manifest/v2` with the
`functional-ui-capability/v1` policy. A complete, mapped OpenAPI assessment
may require only tests sharing affected capabilities and place all omissions
in the later full-suite control.

The UI policy additionally reads the immutable change set for the same
delivery. Targeted selection is permitted only when provenance matches, the
provider file list is complete, and its paths and rename predecessors exactly
match the set of analyzed OpenAPI documents. Any extra source file, missing
document, truncated list, partial assessment, or unmapped operation uses
`fallback` and requires all cataloged UI tests. Inconsistent durable evidence
fails closed without a manifest. Bounded catalog scanning remains unchanged.

The new manifest is selection evidence, not permission to run repository code
or a release gate. The functional API planner, runner, and attempt contracts
continue to accept only v1. UI execution requires a separate reviewed adapter
and evidence protocol.

## Alternatives considered

### Reuse execution-manifest v1 with a second family

- Benefits: One schema and fewer converters.
- Costs and risks: Changes a published field and policy meaning in place;
  existing v1 planners could misinterpret browser tests.
- Reason not selected: A distinct version makes unsupported consumers reject
  rather than silently execute or drop a family.

### Target UI tests whenever OpenAPI impact is complete

- Benefits: More opportunities to shorten the early run.
- Costs and risks: A PR can change an analyzed API document and an unanalyzed
  UI component together; complete API impact does not cover the component.
- Reason not selected: Safety requires a changed-file coverage proof.

### Wait for UI route and component analysis before any browser decision

- Benefits: A richer browser-specific impact model from the outset.
- Costs and risks: Delays a useful, conservative cross-interface decision for
  repositories that already declare capability mappings.
- Reason not selected: A narrow API-only proof has a safe fallback and can
  coexist with later UI-specific producers.

## Consequences

### Positive

- Existing UI catalog mappings now produce an explained, versioned decision.
- Mixed or unknown changes cannot silently narrow the browser candidate set.
- Functional API v1 consumers and selection behavior remain unchanged.

### Negative

- Most UI implementation changes run every cataloged UI test until a trusted
  UI route/component impact producer exists.
- This adds a second manifest version and a second persisted evidence read for
  UI selection.

### Neutral or follow-up

- Playwright discovery, project/tag/owner metadata, browser result ingestion,
  route/component impact, and locator repair remain M07 work.
- A future UI impact producer must define versioned completeness and selection
  semantics; it cannot reinterpret this OpenAPI-only policy in place.

## Compatibility and migration

No database migration or change to repository-descriptor v1 is required.
Existing `select` invocations continue emitting execution-manifest v1. UI
callers opt into `-family functional-ui` and must use a v2-aware consumer.
Unsupported downstream v1 planners and runners reject v2; operators should
not route v2 manifests to the functional API execution workflow.

## Security and operations

Selection reads only existing immutable database evidence and does not run a
browser, repository command, or model. Unknown evidence requires broader
execution. Database credentials remain environment-only. The current direct-
database command remains transitional pending the M10 authenticated API.

## Validation

Effect Schema and Go tests verify v2 compatibility, family separation, and
round trips. Application and bridge tests cover targeted API-only changes,
mixed-file fallback, truncated-file fallback, provenance mismatch, and
preservation of v1 behavior. Repository validation covers generated schema
reproducibility and architecture boundaries.
