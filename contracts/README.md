# Argus contract workspace

## TL;DR

- JSON Schema Draft 2020-12 is the language-neutral source of truth.
- `manifest.json` owns every schema and compatibility fixture in this workspace.
- `make generate-contracts` updates checked-in Go and Python bindings;
  `make check` proves regeneration is byte-for-byte reproducible.
- Consumers MUST validate at their boundary. The generated models describe
  structure; the Go and Python wrappers also enforce cross-field invariants.
- Add feature-specific contracts with their first consumer. Do not turn this
  kernel into a speculative model of the entire product.

## Purpose and layout

This workspace defines the smallest shared vocabulary needed before Argus can
catalog repositories and tests. It keeps wire compatibility independent from
the internal types of any Go or Python component.

| Path | Responsibility |
|:--|:--|
| `manifest.json` | Bundle version, contract inventory, generated outputs, and expected fixture outcomes. |
| `schemas/<contract>/vN/` | Canonical JSON Schemas, grouped by contract major. |
| `fixtures/<contract>/vN/` | Positive, structurally invalid, and semantically invalid compatibility examples. |
| `generated/go/` | Checked-in Go models. Do not edit them manually. |
| `generated/python/` | Checked-in Python models plus the hand-written consumer boundary. |
| `kernel.go` | Go structural validation, decoding, and semantic invariants. |
| `scripts/validate.py` | Offline manifest, schema, format, and fixture validation. |
| `tests/` | Python compatibility tests over the same fixture corpus as Go. |

The initial `kernel/v1` document is a conformance envelope, not an application
message. It exercises repository, immutable revision, component, capability,
suite, and stable test identities together with producer provenance,
confidence, observation and expiry, validation errors, and compatibility
metadata. Later contracts reference these concepts without having to copy a
language implementation.

## Identity and evidence semantics

Identifiers are opaque values with readable namespaces. Consumers MUST compare
the complete value and MUST NOT infer authorization, storage keys, ownership,
or hierarchy from its text.

- A repository ID identifies the hosted repository, never a local checkout
  path. Its locator is an HTTPS URI used for display and resolution.
- A revision is a full, lowercase Git SHA-1 bound to one repository. Branches
  and tags are mutable hints and are not revision identities.
- Component and suite identities include their repository reference. A test ID
  belongs to exactly one suite and MUST remain stable across execution order,
  worker, shard, and ordinary source movement.
- Capability IDs describe product behavior and are deliberately not owned by a
  source-file path.
- Provenance says which versioned producer used which method against which
  immutable repository revision.
- Confidence is evidence metadata, not permission to act. A score is in the
  closed interval `[0, 1]` and always includes its basis and rationale.
- `observedAt` and a non-null `expiresAt` are timezone-aware instants.
  Expiry MUST follow observation. `null` means a policy explicitly declared
  the evidence non-expiring; omission is invalid.

JSON Schema validates local structure. Relationships such as matching
repository, suite, revision, expiry, and bundle references are enforced by
`contracts.DecodeKernel` in Go and `argus_contracts.decode_kernel` in Python.
Production consumers MUST use those validated boundaries or implement and
conformance-test an equivalent one; deserializing a generated class alone is
not sufficient.

## Versioning and compatibility

The bundle uses semantic versions. Every contract also has an integer schema
major represented in its name and directory, for example `kernel/v1`.

- Backward-compatible additions require a bundle minor version. With strict
  objects, adding an optional field is compatible for readers but producers
  MUST NOT emit it until their declared minimum reader can accept it.
- Documentation, fixture, or generator corrections that do not change accepted
  wire values require a bundle patch version.
- Removing or renaming a field, changing its meaning, narrowing accepted
  values, or otherwise rejecting a previously valid document requires a new
  contract major and a bundle major version.
- A new major lives beside the old major during migration. Producers publish
  the new representation before consumers are required to read it.
- `minimumReaderBundleVersion` makes the producer's compatibility requirement
  explicit. It is not an instruction to fetch code dynamically.

Unknown fields are rejected. This prevents misspellings and producers that
silently outrun consumers. The manifest MUST list every schema and fixture
exactly once; unowned files fail validation.

## Generation and validation

Install Go, Python, uv, and GNU Make as described in the
[developer quickstart](../docs/development/developer-quickstart.md). Then run:

~~~sh
make generate-contracts
make fmt
make check
make test
~~~

Generation runs pinned tools from `go.mod` and `uv.lock`. It writes into a
temporary directory first. Verification regenerates both bindings and compares
their bytes with the committed files, so generator, option, schema, or output
drift fails before merge.

The compatibility corpus has two independent expectations:

- `schemaValid` is evaluated by an offline Draft 2020-12 validator with format
  checks enabled;
- `bindingValid` is evaluated through both language consumer boundaries and
  includes semantic invariants that JSON Schema cannot express locally.

Each invalid fixture SHOULD isolate one behavior and MUST remain invalid for
the declared reason. Both languages consume the manifest rather than maintain
separate fixture lists.

## Adding or changing a contract

1. Add or revise the schema under its contract-major directory.
2. Add representative positive and negative fixtures, including semantic
   boundary failures where applicable.
3. Register the schema, generated outputs, and every fixture in the manifest.
4. Add the contract to the generation orchestration and validated language
   boundary with the first real consumer.
5. Run `make generate-contracts`, inspect the generated diff, then run
   `make fmt` and `make validate`.
6. Record a new ADR when the change alters compatibility policy, representation
   technology, ownership, or a cross-product boundary.

Generated files MUST NOT contain machine-specific paths or timestamps.
Hand-written behavior belongs outside the generated file so regeneration never
overwrites invariants.

## Relationship to Perfeng

Perfeng demonstrated a useful pattern: language-neutral schemas, a manifest,
positive and negative examples, generated bindings, and an explicit
compatibility ledger. Argus adopts that pattern but owns its vocabulary and
release lifecycle. It does not import `perfeng-contracts`, copy performance
messages, or create a relative-path build dependency.

A shared package is considered only after two independent consumers prove the
same semantics and lifecycle, following ADR-0001. Until then, alignment is at
the conventions and eventual versioned integration-contract level. M09 will
define the actual Argus–Perfeng messages jointly with their first end-to-end
consumer.
