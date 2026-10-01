# ADR-0021: Keep Go contract DTOs product-internal

- Status: Proposed
- Date: 2026-10-01
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Effect Schema and generated JSON Schema remain the portable contract surface.
- Argus's hand-written Go wire DTOs and validators belong under
  `internal/contracts`; they are not a versioned Go SDK.
- The root `contracts` Go package only embeds generated schemas for offline
  validation. Existing wire bytes, schema paths, and fixtures do not change.
- External Go consumers of the old import path must validate the portable
  schemas and own their bindings until a real SDK is intentionally released.

## Context

ADR-0003 makes Effect Schema authoritative and JSON Schema portable. The Go
control plane needs local DTOs after schema validation, but placing those DTOs
at `github.com/stanimirivanov/argus/contracts` accidentally exposes a Go API.
As additional test families arrive, changing those DTOs should not imply a
cross-product Go release contract. No Argus Go SDK is currently promised.

## Decision

Move hand-written Go transport DTOs, validators, and their corpus tests to
`internal/contracts`. Only explicitly authorized CLI, process-protocol, and
contract-conversion adapters may import that package. Domain and application
packages continue to use their own models, not wire DTOs.

Keep `contracts/source`, `contracts/generated`, and `contracts/fixtures` at
their existing portable paths. The root Go package provides only an embedded
schema reader used by the internal binding. The architecture test enforces
both the internal DTO importer allowlist and the schema-asset access boundary.

## Alternatives considered

### Treat the root Go DTO package as a supported SDK

- Benefits: Existing Go imports remain stable.
- Costs and risks: Requires SDK-grade compatibility, field documentation,
  semantic-versioning and independent consumer evidence now.
- Reason not selected: No external Go consumer or release owner justifies it.

### Copy schemas beneath the internal Go package

- Benefits: No root Go accessor.
- Costs and risks: Duplicates every generated artifact or adds a second
  generated output tree that must be kept synchronized.
- Reason not selected: One embedded source preserves the existing portable
  artifact path and exact bytes.

## Consequences

### Positive

- Go implementation changes remain product-internal while wire compatibility
  stays governed by Effect Schema, versioned JSON Schema, and shared fixtures.
- Deployed binaries validate offline against the checked-in schema corpus.
- Architecture checks prevent a future domain or persistence import of DTOs.

### Negative

- The former public Go import path no longer exposes DTO symbols. Consumers
  using it must migrate; this repository provides no compatibility aliases.
- The narrow schema reader in the root package remains importable Go code and
  must be documented and tested as such.

### Neutral or follow-up

- A Go SDK can be proposed once an actual independently released consumer
  needs it, with a separate compatibility and ownership decision.
- This does not split the repository or change runtime deployment topology.

## Compatibility and migration

No JSON wire shape, schema identifier, fixture, persisted record, or command
output changes. In-repository Go callers switch to `internal/contracts`.
External Go callers cannot import the internal package; they should use the
portable schema and their own DTOs. A future SDK would have its own versioned
package rather than promoting this implementation package by default.

## Security and operations

Schema validation remains local and does not fetch remote `$id` URLs. No new
credentials, network calls, migrations, or deployment components are added.

## Validation

Run the shared contract corpus, architecture boundary tests, and complete
`make validate` suite. Verify that a built binary retains schema validation
without files from the source checkout.
