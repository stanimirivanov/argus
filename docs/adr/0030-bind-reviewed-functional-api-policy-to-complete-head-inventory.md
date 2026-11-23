# ADR-0030: Bind reviewed functional API policy to complete head inventory

- Status: Proposed
- Date: 2026-10-15
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep the published functional API manifest v1 unchanged.
- Opt in to reviewed policy v2 through trusted CI policy and head-inventory files.
- Bind both inputs by SHA-256 in execution-manifest v4; bind mapping evidence to
  the immutable base catalog and change observation time.
- Require complete inventory. Missing base tests, absent rules, or unsupported
  mapping states fail closed; stale or conflicting affected mappings require
  full-suite fallback.
- New and modified tests, must-run rules, pins, and direct or supported impact
  override early-stage exclusions. Every early omission retains a full-suite
  obligation.

## Context

ADR-0009 uses an approved base catalog for the initial functional API subset.
It deliberately deferred owner must-run rules, pins, exclusions, and mapping
precedence. M04 now retains source and configuration impact as v2 evidence.
The base catalog cannot know tests introduced in a separate test repository's
head revision, so an empty or stale inventory cannot justify an omission.

## Decision

The optional `select -policy-file ... -head-inventory-file ...` path reads
bounded JSON from trusted CI. The policy is scoped to the stable source
repository identity. Head inventory names immutable test-repository revisions
and explicitly declares completeness. A head-only test must be marked new and
cannot grant itself a capability mapping. Base tests absent from the head
inventory and policy rules naming absent tests fail before publication.

The selector uses persisted composite impact, the approved base descriptor,
and catalog edges evaluated at the immutable change observation time.
Conflicting or stale affected edges, refutation of an approved base mapping,
or an edge to a test absent from complete inventory requires all candidates.
Supported evidence may add an affected mapping, with observation keys in the
decision reason. Neither head metadata nor directory proximity can add an
authoritative capability mapping.

In targeted mode the precedence is new/modified test, must-run rule, explicit
pin, direct base mapping, supported catalog edge, then early-stage exclusion.
An exclusion can only omit an otherwise optional test; omitted tests remain
required in the later full-suite control. The separate v4 manifest records
policy and inventory content digests and retains the existing change, impact,
and base-catalog revisions. The v1-v3 contracts retain their published meaning.

## Alternatives considered

### Put policy in the pull-request head descriptor

This lets a change redefine the rules judging itself. The reviewed CI input
keeps owner authority separate from untrusted PR content.

### Infer new tests from the base catalog

The base catalog cannot observe future test-repository changes. Complete head
discovery is an explicit prerequisite for a selective result.

### Resolve mapping conflicts by confidence score

Evidence confidence is producer-specific. A universal threshold would invent
policy and can convert contradictory evidence into a false omission.

## Consequences and limits

The new manifest is selection output for review and evaluation. The reference
framework integration in the next M05 slice will connect complete discovery
and v4 execution; existing v1 execution commands remain unchanged. The head
inventory's `complete` assertion is trusted CI evidence and must be produced
by an independently reviewed discovery command, not accepted from PR-authored
content. A removed base test currently fails closed pending explicit deletion
semantics.

## Validation

Table-driven policy tests cover precedence, new and modified tests, stale and
conflicting edges, missing inventory, unavailable evidence, and deterministic
ordering. Effect-authored schemas and boundary tests verify v4 isolation from
published v1 manifests. The repository gate is `make validate`.
