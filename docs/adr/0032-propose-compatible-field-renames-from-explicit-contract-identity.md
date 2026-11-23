# ADR-0032: Propose compatible field renames from explicit contract identity

- Status: Proposed
- Date: 2026-10-17
- Milestone: M06 - Validated adaptation
- Deciders: Argus maintainers and functional API test owners
- Supersedes: None
- Superseded by: None

## TL;DR

- A compatible field proposal requires `x-argus-renamed-from` on the new OpenAPI JSON property.
- Argus compares the complete operation at immutable base and head revisions after reversing only the declared rename.
- A reviewed framework adapter locates one syntax node; field proposals use separate contracts and cannot enter the endpoint validation or publication path.

## Context

An added property resembling a removed property does not establish identity or
compatibility. A field change may also alter type, requiredness, defaults,
responses, or asserted meaning. The endpoint proposal contract only permits a
request-target edit and its validation negative control is endpoint-specific.

## Decision

The source owner explicitly declares the old field identity with
`x-argus-renamed-from` on one new inline `application/json` object property.
Argus reads both OpenAPI blobs from full Git revisions in the checked-out source
repository. It requires the same path, method, operation ID, capability mapping,
and one declared rename. It reverses the new property name, required-list name,
and rename annotation in a copy of the head operation. The complete normalized
operation must then equal the base operation. References, multiple declarations,
and other semantic edits abstain.

The resulting proof includes both blob digests, source document and operation
identity, direction, response status when applicable, capabilities, and old/new
field names. The candidate test must be in the reviewed execution manifest and
intersect that capability mapping. A repository-owned adapter may identify one
direct request-body key or one response field access inside the named test. A
byte-addressed edit may replace only that identifier. No assertion operator,
expected value, fixture, or additional file may change.

Field request, result, and proposal contracts have separate versions from the
published endpoint contracts. `PATCH_AND_VALIDATE` means only that an isolated
field-specific validation may later be attempted. Existing endpoint validation
and GitHub publication commands reject this new proposal shape.

## Alternatives considered

### Infer a rename from property similarity

Rejected because naming and equal types do not prove business identity.

### Reuse endpoint proposal and negative-control contracts

Rejected because a field-access edit has different semantics and needs a
different discriminating negative control and repeated validation policy.

## Consequences

The supported proposal class is deliberately narrow: direct inline JSON object
properties and reviewed JavaScript syntax. Schema references, nested paths,
computed access, multiple candidates, and non-JSON media types abstain. Owners
can extend support with evidence and a separately reviewed policy version.

## Compatibility and migration

Endpoint proposal v1 and all downstream evidence retain their current meaning.
No migration is required. Field proposal v1 is a new read-only output until its
class-specific validation and review workflow is delivered.

## Security and operations

CI must supply a trusted source checkout and test checkout at immutable
revisions and an explicit adapter command. Git reads are bounded and
shell-free. The proposal command performs no source mutation or provider write.

## Validation

Contract comparison tests cover declared and undeclared renames, type and
requiredness changes, ambiguous declarations, and operation drift. Adapter
checks cover request and response syntax, stale source, dynamic references,
exact byte spans, and unchanged expected values. The standard repository
validation suite remains the merge gate.
