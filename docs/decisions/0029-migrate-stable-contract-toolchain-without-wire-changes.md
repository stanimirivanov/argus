# ADR-0029: Migrate the stable contract toolchain without wire changes

- Status: Proposed
- Date: 2026-10-13
- Milestone: M10 - Production readiness
- Deciders: Argus maintainers
- Supersedes: ADR-0003's Effect v3 version constraint only
- Superseded by: None

## TL;DR

- Use the requested stable Effect 4.0.2 and TypeScript 7.0.2 releases while retaining Node 24 and `@types/node` 24.12.1.
- Preserve the Effect-source / generated Draft 2020-12 / Go-domain boundary of ADR-0003 and all existing contract majors.
- Use native Effect 4 checks, result decoders, string timestamp codecs and strict JSON Schema export. Unicode-mode regexes are necessary for pattern export.
- Upgrade the opt-in DBOS runtime to SDK 1.6.0/schema 123; explicit migration and exact-version startup checks remain mandatory. This is not production adoption.

## Context

The requested dependency upgrades are already on main. Effect 3 APIs no longer compile under Effect 4, TypeScript 7 requires explicit Node type discovery, and the DBOS 1.5 schema guard rejects the schema created by SDK 1.6. Merely regenerating artifacts can omit patterns and accidentally weaken Go validation. Accepted ADR-0003 records the original v3 decision; its historical text is not rewritten to imply it originally selected v4.

## Decision

The current toolchain uses Effect 4.0.2, TypeScript 7.0.2 and Node 24.18.0 with Node 24 type declarations. Explicit `types: ["node"]` and disposable-resource declarations retain strict checking without `skipLibCheck` or a Node runtime upgrade.

Contracts use native `.check`, `.annotate`, array-form unions/literals, `decodeUnknownResult`, and `DateTimeUtcFromString`. Optional JSON keys use `optionalKey`; nullable fields remain required unless explicitly optional. The exporter uses `Schema.toJsonSchemaDocument` with `onExcessProperty: "error"`, retains local definitions and emits Draft 2020-12. Patterns use Unicode-mode regexes so they are included by the compiler. Fixture tests and export regressions guard keys, digests, paths, timestamps, bounds and nested unknown fields.

No public field, identity, contract major or domain rule changes. Regenerated ordering and compiler descriptions are build-artifact differences, not new wire semantics. Go domain code remains independent of Effect.

The review comparison resolves local definitions, ignores annotation/order
differences and normalizes equivalent unions; it reports zero validation-
constraint differences across the generated contracts. Timestamp JSON remains
the legacy plain-string schema, while Effect codecs and Go decoding retain
calendar validation. No new `format` restriction is silently added to a major.

The Go dependency upgrades retain existing adapters: libopenapi 0.41.5 and opt-in DBOS 1.6.0. DBOS startup requires schema 123 and never migrates it automatically. The evaluation matrix builds a checksum-pinned historical 1.5 worker with guard 121 through a temporary overlay, then migrates with the current 1.6 worker and verifies refused rollback. Archived October 10 measurements remain immutable historical evidence. Default synchronous processing and the defer-production verdict remain unchanged.

## Alternatives considered

- Roll back all upgrades: avoids migration but contradicts the requested stable-version direction.
- Use compatibility wrappers or disable strict checks: hides API migration and risks weaker validation; rejected.
- Upgrade Node to 26: not required by this delivered toolchain and explicitly excluded.

## Consequences

Native current APIs remove reliance on the old authoring surface. The migration requires regenerated artifacts and a maintained two-SDK evaluation build. Effect's best-effort compiler still needs fixture and export checks; compilation alone is not compatibility evidence. This decision does not select an orchestration runtime for Perfeng or Argus SRE.

## Compatibility and migration

Existing JSON producers and Go DTOs retain their majors and meaning. For an existing opt-in DBOS database, drain old-version work with its exact binary before applying `go run ./cmd/migrate --dbos-evaluation` from the new build. Runtime then requires schema 123. Old binaries requiring 121 cannot roll back in place after migration; do not decrement the ledger or discard history. No Argus catalog migration is changed.

## Security and operations

Keep exact pins and frozen lockfile installation. Required license, vulnerability, fixture, strict type, race and integration checks remain enabled. No new credentials, production endpoint or automatic cleanup authority is introduced.

## Validation

Run `make fmt`, `make validate`, `make db-validate` and `make dbos-evaluate`. Regenerate schemas, inspect constraint differences, test both Effect and Go against the shared corpus, and guard exported patterns, unknown fields, numeric/array bounds and timestamp strings. Record any unavailable platform check rather than claiming it passed.
