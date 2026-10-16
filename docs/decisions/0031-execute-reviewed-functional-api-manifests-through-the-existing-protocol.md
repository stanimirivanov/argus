# ADR-0031: Execute reviewed functional API manifests through the existing protocol

- Status: Proposed
- Date: 2026-10-16
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Permit functional API manifest v4 references in the existing plan, adapter
  request, attempt, and shadow-report envelopes.
- Keep all existing v1 manifest outputs and protocol fields unchanged.
- Dispatch manifest import and canonical digest by version at the planner and
  runner boundary; reject browser manifests and unknown versions.
- Use the independently reviewed Playwright test map and complete discovery as
  the reference producer of trusted head inventory.

## Context

ADR-0030 made reviewed selection available as manifest v4, but the execution
stack accepted only manifest v1. The plan and adapter request already carry a
versioned manifest reference and exact digest; execution and shadow evaluation
otherwise depend on normalized test identities and revisions, not the policy
shape. Introducing parallel copies of every execution document would add
duplicate lifecycle code without a different execution meaning.

## Decision

The accepted functional API execution envelopes continue to use their v1
`apiVersion`. Their `manifest.apiVersion` reference now permits v1 or v4.
Producer code emits v4 references only for an explicitly supplied reviewed v4
manifest; existing v1 calls continue to emit v1. Canonical digest calculation
uses the full version-specific manifest, preserving policy and inventory
digests, evidence keys, and every decision. Import rejects unknown and browser
manifest versions before planning or execution.

The Playwright reference adapter is a reviewed command in the test repository
checkout. Stable IDs come from a reviewed map and exact discovery; names and
native Playwright IDs are not authority. Discovery can mark changed tests but
cannot grant a capability mapping. Full-suite control remains mandatory.

## Compatibility and consequences

The wire shape of the execution envelopes is unchanged, and v1 manifest
behavior is unchanged. Older strict consumers of those envelopes may reject a
v4 manifest reference, so CI must deploy the updated planner, runner, and
reporting binaries together before opting into v4. The reference campaign
checks exact cross-version digest correlation and rejects malformed or
incomplete evidence. PostgreSQL needs a forward constraint migration to admit
v4 references; existing rows need no rewrite or backfill.
