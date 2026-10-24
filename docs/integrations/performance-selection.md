# Reviewed performance selection

## TL;DR

- M09-PR5a publishes reviewed mapping, source context and recommendation contracts.
- Every recommendation pins source, impact, catalog and reviewed policy evidence.
- Candidates require complete deployment provenance and explicit owner approval.
- Use `select-performance -inspect` before reviewing a policy, then select with `-policy`.

## Published boundaries

The [Effect definition](../../contracts/source/performance-selection-v1.ts) owns
three closed, bounded wire schemas. The existing generator publishes:

- [Mapping policy v1](../../contracts/generated/performance-mapping-policy/v1/performance-mapping-policy.schema.json).
- [Selection context v1](../../contracts/generated/performance-selection-context/v1/performance-selection-context.schema.json).
- [Selection v1](../../contracts/generated/performance-selection/v1/performance-selection.schema.json).

The [Go bindings](../../internal/contracts/performance_selection.go) validate
embedded schemas before decoding. These are transport values, not domain selection
or authorization services. Existing source identity, change and timestamp formats
are reused. Unknown fields, unsupported versions and execution-authority fields
are rejected.

## Evidence and review pins

Context retains the immutable change comparison, analyzer version, impact digest,
catalog reference/fingerprint/schema digest, authorized scope and catalog expiry.
The impact digest identifies the canonical public v1/v2 export encoded by Go
`json.Marshal`, including composite file evidence. The catalog fingerprint covers
the complete authorized snapshot. The policy digest identifies original reviewed
file bytes, including whitespace; arbitrary JSON reserialization changes that pin.
Consumers must preserve the corresponding artifacts.

A mapping policy binds repository/head revision, impact/catalog digests, review
record/reviewer and validity window. It declares the reviewed head capability
inventory, exact allowed profiles and explicit workload/profile/environment/policy/
baseline mappings. Deployment bindings retain environment, deployment reference,
service identity, image digest and source revision. Reference identity includes
issuer, ID, revision and SHA-256, rather than a display label.

All fields are required. Arrays can be explicitly empty; null arrays are invalid.
Baseline and input deployment reference/service/image can be null to express missing
evidence. Candidate output requires a non-null baseline, owner route and complete
deployment provenance. Every output requires `ownerApprovalRequired:true`; a result
never authorizes execution. Omitted and review decisions preserve reason codes.

## Semantic obligations for consumers

Schema validity does not authenticate reviewers or establish freshness, digest
correctness, unique identities, reference relationships or lifecycle eligibility.
Consumers must verify policy origin, reviewer authority, the head inventory and
image/source association. Timestamp strings remain portable on the Go boundary;
consumers must evaluate actual instants and validity windows.

The selector must treat incomplete impact, uncovered capabilities, conflicts,
forbidden profiles, unsupported or stale references, absent approved baselines and
unbound deployments as review. Removed API operations do not establish capability
deletion. Each explicit workload/environment fan-out needs its own exact binding.
Any unresolved decision requires overall review; this relationship is a domain
invariant rather than a schema-derived status. No run-all fallback is permitted.

Later request creation must combine identical targets while retaining reasons and
revalidate current source/catalog/policy and owner authorization. Perfeng owns
admission, execution, baseline approval and statistics. Selection provides no
dispatch, prediction or persistence writes.

## Deterministic domain selection

The [performance domain](../../internal/performance/selection.go) evaluates validated
inward impact/catalog values and reviewed policy at an explicit trusted instant.
It performs no I/O, wall-clock lookup or dispatch. Source evidence and returned
evidence pointers are read-only and must not be mutated concurrently.

Mappings are ordered by stable ID; uncovered affected capability keys follow in
sorted order. Unaffected mappings are retained as omissions. A removed API
operation still affects its capability when that capability exists at the reviewed
head. Explicit workload/environment fan-out is permitted with separate exact
deployment bindings; rules sharing capability/workload/profile/environment identity
conflict even when their resource versions differ.

Candidates require matching source/impact/catalog pins, fresh catalog/review,
allowed profile, active exact references and owner route, reciprocal workload/profile
support, a published target tuple, approved matching baseline, and complete
deployment/service/image/source proof. Every result requires owner approval and
expires at the earlier of catalog and policy expiry. Expired evidence retains its
original expiry in review output. A complete producer flag cannot hide unmapped
operations or an empty impact. Any unresolved decision requires overall review;
otherwise eligible mapping decisions remain visible without permitting partial
automatic dispatch. Malformed input, future source observations and duplicate
mapping, environment or catalog identities return an error with no partial result.

This domain API does not authenticate review metadata or calculate artifact hashes.
The pinned evidence adapter supplies independently validated, digest-bound evidence;
the operator command supplies the entry point without persistence writes.

## Pinned evidence conversion

The [selection I/O adapter](../../internal/performance/adapters/selectionio/selection.go)
composes existing public change converters and the pinned Perfeng catalog importer.
`Inspect` creates owned, opaque evidence: callers cannot independently replace its
domain inputs and portable provenance. It hashes the complete canonical public
impact export, including v2 file evidence, without mutating the source.

Stored catalogs are revalidated against independent issuer/principal/producer
expectations, the pinned schema, full page/relationship closure and the stored
fingerprint. Revalidation uses the source generation instant, retaining supported
microsecond precision and UTC offsets. Historical expiry does not prevent integrity
inspection within accepted schema/producer pins; unsupported pins require explicit
compatibility review rather than silent re-pinning. `Evaluate` checks freshness at
the caller's trusted use-time instant.

`Context` validates the portable inspection response. `Evaluate` accepts at most
16 MiB of original policy bytes, validates their schema and parses actual review/
expiry instants, then imports separate domain values. It retains the exact policy
byte digest and validates exported candidates, omissions and review decisions before
returning output. Unknown versions/fields, malformed or oversized policy, mismatched
stored evidence and zero evidence fail without partial results. Opaque evidence can
be reused concurrently; returned DTOs do not expose its private source projection.

These checks establish consistency with authenticated stored evidence, not reviewer
authentication or deployment-claim truth. Operators/CI must supply trusted policy
origin and independent catalog expectations. The adapter performs no network or
database I/O and grants no dispatch authority. The command composition root calls
it and injects a validated operation into its argument/output adapter.

## Operator inspection and selection

The [select-performance command](../../cmd/select-performance/main.go) is opt-in.
Configure `ARGUS_DATABASE_URL` through your secret manager/environment, using a
read-only database role for the existing impact and performance catalog stores.
No provider URL or token is required: this command makes no provider requests.

```sh
go run ./cmd/select-performance -delivery-id delivery-42 -inspect \
  -issuer https://provider.example/identity \
  -principal-issuer https://auth.example/tenant -principal-id argus-service \
  -producer-version 1.0.0
```

Issuer, principal and producer expectations must come from trusted operator/CI
configuration, independently of the stored pages. `-producer-id` defaults to
`perfeng-control-plane`. These arguments select and verify an authorized stored
view; they do not authenticate the caller. Database access and scope authorization
remain deployment responsibilities. Impact lookup uses the exact GitHub delivery
ID and verifies that the returned evidence belongs to that delivery. Catalog lookup
uses the exact issuer/principal tuple; missing evidence never becomes an empty view.

Preserve the inspection JSON and its referenced canonical impact and catalog
artifacts. Have an authorized reviewer construct the mapping policy described above,
including exact pins, head inventory, target/deployment bindings and validity window.
Then repeat the invocation, replacing `-inspect` with `-policy reviewed-policy.json`.
Policy input is limited to 16 MiB; its original bytes define the policy digest.
Operators/CI must establish policy origin and reviewer authority before invoking it.

Selection evaluates freshness using actual time after both stored reads and evidence
validation. There is no historical-clock override. Inspection can retain historical
expired provenance; selection conservatively requires review for stale evidence.
Separate immutable reads require no shared write transaction: a later current-pointer
update cannot alter the loaded snapshot, and the policy pins must still match it.
Later request creation must revalidate current evidence and owner authorization.

Success writes one schema-validated JSON document plus a newline and exits 0;
`review-required` is a successful decision requiring human action. Syntax/missing
options exit 2. Storage, validation, policy-file, cancellation and output failures
exit 1. No JSON is written before successful evaluation and a final cancellation
check. A stdout failure can leave a partial document: consume output only after exit
0. Database/file failures use bounded diagnostics without credentials or policy bytes.
Ctrl+C/SIGTERM cancels database reads; no retry, write, execution lease or dispatch is
performed. `--help` and `--version` work without database configuration.

## Delivery and verification

The [approved delivery sequence](../development/plans/m09-pr5-performance-selection.md)
separates contracts, selector, evidence adapters and CLI. Shared contracts support
those consumers and later M09 request/projection work without runtime stubs.
Run `make generate-contracts` and `make validate`; Go and TypeScript conformance
tests cover closed shapes, explicit emptiness, approval and candidate provenance.
Run `go test -vet=off ./internal/performance/... ./internal/architecture` for focused
selector, evidence conversion and dependency-boundary verification. Run
`go test -vet=off ./cmd/select-performance ./internal/performance/adapters/cli/selectioncli`
for scoped read and CLI failure coverage; the command smoke suite also checks metadata
and exit codes. No database-specific gate is needed because this slice changes no
SQL, schema or persistence writes. See
[ADR-0055](../decisions/0055-select-reviewed-performance-workload-targets.md).
