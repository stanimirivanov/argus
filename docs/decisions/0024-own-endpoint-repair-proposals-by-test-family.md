# ADR-0024: Own endpoint repair proposals by test family

- Status: Proposed
- Date: 2026-10-02
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep immutable test identity, byte-addressed edit structure, and review
  evidence shared within adaptation.
- Move functional API endpoint-rename proposal rules into a family-owned domain
  package, independent of adapters and use-case orchestration.
- Preserve all published Effect/JSON contracts and CLI behavior.
- A later UI proposal must establish its own proof and edit semantics rather
  than weakening the endpoint policy in a shared model.

## Context

The first repair class placed endpoint-rename evidence, process request/result
rules, proposal policy, and shared adaptation evidence in one root Go package.
That was sufficient for one family, but the next UI adaptation class has a
different proof and semantic edit. Keeping both in the root model would invite
conditional validation and a broad proposal type that can represent invalid
cross-family combinations.

## Decision

`internal/adaptation/endpointrepair` owns the functional API proposal,
endpoint-rename evidence, adapter request/result, classification, decision,
policy version, canonicalization, and validation. It is a domain package; it
imports only shared adaptation values, catalog identity, and change evidence.
The proposal, validation, review, outcome, and contract adapters consume its
typed proposal directly. No compatibility alias remains in the shared root.

`internal/adaptation` retains shared test identity, structural `TextEdit`
validation, stable errors, and existing validation/review/outcome evidence.
The current validation contract still explicitly requires a `request-target`
edit and its admitted proposal policy version. Structural edit validity does
not grant a semantic role: the endpoint policy independently requires
`request-target` and the exact old/new endpoint paths.

This is a code-ownership boundary, not a new service, repository, public Go
SDK, or published contract version.

## Alternatives considered

### Keep all proposal policy in the shared adaptation package

- Benefits: Fewer Go packages and import edits.
- Costs and risks: New test families would accumulate unrelated proposal
  fields and branching validation in one shared model.
- Reason not selected: It obscures which proof authorizes which edit.

### Move every validation and review value at once

- Benefits: A fully family-local repair pipeline in one change.
- Costs and risks: It moves established immutable evidence and persistence
  contracts unnecessarily before a second family demonstrates their variance.
- Reason not selected: Proposal policy is the demonstrated variation point;
  evidence ownership can be decided with the first UI slice.

## Consequences

### Positive

- Endpoint-specific invariants have a single named owner and cannot be
  mistaken for generic adaptation rules.
- The shared edit validator can be reused without accepting an arbitrary
  semantic role as a valid endpoint repair.
- UI proposal work can define its own proof without editing endpoint policy.

### Negative

- Application and adapter packages import both the shared adaptation package
  and the endpoint repair policy package.
- The existing validation evidence remains endpoint-specific despite its
  current location; moving it later requires a separate compatibility review.

### Neutral or follow-up

- A second family should establish whether validation and review evidence can
  remain shared or need family-owned types. Do not generalize them speculatively.

## Compatibility and migration

All wire API versions, JSON fields, policy strings, identities, error classes,
CLI invocations, and persisted evidence retain their meaning. Only product-
internal Go imports change. Existing consumers of published portable schemas
require no migration.

## Security and operations

The adapter remains untrusted and gains no source mutation or review authority.
Semantic edit restrictions and validation gates remain mandatory. No new
credentials, deployment units, or database changes are introduced.

## Validation

Architecture import checks classify the new package as adaptation domain.
Existing proposal, validation, review, outcome, process-adapter, contract, and
CLI tests exercise the same successful and rejected evidence paths. The full
repository validation suite checks generated contracts and command behavior.
