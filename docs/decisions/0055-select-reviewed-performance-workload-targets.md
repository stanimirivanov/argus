# ADR-0055: Define reviewed performance selection boundaries

- Status: Proposed
- Date: 2026-10-23
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Publish additive mapping-policy, context and recommendation contracts first.
- Pin reviewed evidence and require complete provenance for candidate outputs.
- Keep schema validation separate from authorization and domain eligibility.
- Deliver selector, evidence adapters and CLI in three dependent follow-up PRs.

## Context

Authorized catalog publication is complete. Selection needs shared boundaries for
reviewed mappings and reproducible recommendations without acquiring provider
execution authority. The complete prototype exceeded the agreed review size; the
owner approved four dependent slices. This PR delivers their contract foundation,
consumed by multiple later adapters and M09 request/projection work.

## Decision

Use the existing Effect-to-JSON-Schema generator and embedded Go validation to
publish three closed, bounded v1 contracts. Reuse existing repository, Git revision,
change and timestamp formats. Add no runtime service, dependency or migration.

Bind context to the immutable change comparison, canonical public impact-export
digest and full catalog reference/fingerprint/schema digest, scope and expiry.
Define the impact digest over Go `json.Marshal` of the canonical v1/v2 export,
including composite files. Define the reviewed policy digest over original bytes,
including whitespace. Preserve exact artifacts rather than reserializing evidence.

Reviewed policy binds repository/head, review metadata, validity window, head
capability inventory, exact allowed profiles and explicit target/deployment bindings.
Represent known missing baseline or deployment proof as nullable input. Candidate
output requires baseline, owner route, deployment/service/image/source provenance
and `ownerApprovalRequired:true`. Metadata is neither authentication nor launch
permission. Unknown fields and versions fail shape validation.

Consumers must separately enforce trusted provenance, freshness, digest equality,
uniqueness, exact relationships and active/approved lifecycle states. Incomplete or
ambiguous evidence requires review. Removed operations do not establish capability
deletion; explicit fan-out requires separate deployment bindings. Any unresolved
decision makes the overall recommendation require review; never infer a high-load
fallback. Later requests deduplicate targets and revalidate source/catalog/policy
and owner authorization. Perfeng independently owns admission and execution.

## Consequences and compatibility

Published formats are additive; existing contracts and runtime behavior remain
unchanged. Decoders enforce shape without claiming to implement the selector.
The [delivery plan](../development/plans/m09-pr5-performance-selection.md) assigns
semantic checks and runtime flows to PR5b-PR5d. No empty runtime stubs are added.
Published v1 artifacts require coordinated versioned evolution after adoption.

## Verification

`make generate-contracts` and `make validate` check generated parity, Go/TypeScript
conformance, architecture and existing acceptance gates. Tests reject absent
candidate provenance, removed approval requirements, null arrays and unknown
execution fields. No database gate is required because persistence is unchanged.
