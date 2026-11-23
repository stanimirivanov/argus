# ADR-0100: Publish artifact-first change evidence

- Status: Accepted
- Date: 2026-11-23
- Milestone: M10 - Production readiness
- Deciders: Workspace owner
- Supersedes: None
- Superseded by: None

## TL;DR

- Argus owns the first change-evidence handoff to Argus SRE.
- Publish a bounded immutable artifact for one verified change.
- Argus SRE verifies and replays the artifact without reconstructing Argus decisions.

## Context

Argus already owns change identity, impact evidence, test selection, and execution observations. Argus SRE needs these together to reconstruct what validation happened before an incident. The [existing performance projection](0098-define-public-performance-decision-projection.md) covers a different boundary: one admitted performance request.

## Decision

The first handoff is an access-controlled, versioned change evidence artifact produced by Argus for one verified Change Set. It binds the exact change identity, scoped impact evidence, selection decisions and reasons, and exact attempt outcomes supported by the retained source evidence. Preserve explicit missing, deferred, and conflicting evidence and retain the original producer identities and timestamps.

Argus owns the contract and artifact integrity. Argus SRE verifies the exact released schema, artifact digest, and relevant scoped bindings before use. It owns its incident interpretation and preserves the published source claims.

## Alternatives and consequences

An authenticated live API could expose fresher individual observations, but a consumer joining them would have to establish that the inputs describe one consistent historical decision. An immutable artifact gives replay a stable, bounded unit and adds publication and retention obligations. A live read can be introduced through a separate reviewed contract when needed.

## Compatibility, security and rollout

A producer must authorize disclosure for the exact scope and publish only the contract's bounded fields. The artifact grants no access to private tables, raw evidence, or the existing protected performance CLI. This record sets the direction for a future implementation and publishes no new wire schema, endpoint, or runtime path.

## Validation

Producer and consumer conformance must cover wrong change and scope, digest or version mismatch, incomplete or oversized input, and replay of an unchanged artifact. Retain proof that later observations do not alter the bytes already used by a published investigation.
