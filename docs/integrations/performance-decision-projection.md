# Public performance decision projection v1

## TL;DR

- `argus.dev/performance-decision-projection/v1` describes one admitted request.
- Publish only independently authorized metadata; structural validity grants no access.
- Preserve independent quality, SLO and regression outcomes and all conflicting reports.
- Empty known history is successful emptiness; unavailable/corrupt reads emit no document.
- The authorized exporter is joint-plan PR11; this contract starts no runtime.

## Wire boundary

The authoritative [Effect source](../../contracts/source/performance-decision-projection-v1.ts)
generates the [Draft 2020-12 schema](../../contracts/generated/performance-decision-projection/v1/performance-decision-projection.schema.json).
Decode Effect input with `onExcessProperty: "error"`; generated objects are closed
recursively, including unions. No additional public Go SDK or shared provider
message is introduced. This is joint-plan PR10, distinct from the earlier request
cancellation/rerun delivery slice with the same local PR number.

| Field | Allowlisted meaning |
|:--|:--|
| `scope` | Independently authorized provider and principal issuer/ID. |
| `provenance` | Existing change reference plus SHA-256 pins for impact, canonical selection, original mapping-policy bytes, catalog, canonical request review and the retained intent fingerprint. |
| `request` | Frozen request/attempt identities, original request event, exact payload checksum and request/deadline instants. |
| `target` | Reviewed service, capability, deployment, source/image, workload, profile, environment, policy and nullable baseline pins from the frozen request. |
| `run` | Public provider Run identity established by verified retained admission. |
| `lifecycle` | Nullable latest retained provider observation: event/fingerprint, native revision, state and provider observation time. It is execution state, never a performance verdict. |
| `analysis` | Every retained publication's event/fingerprint, exact report checksum, provider publication time, lateness and independent per-rule outcomes/selectors; explicit conflict flag. |

References identify evidence; do not fetch their issuers as URLs or treat hashes
as authentication. A null baseline means the frozen request had no baseline;
a null lifecycle means no retained lifecycle observation, not successful execution.
A request without verified admission cannot be represented as an admitted Run.

## Producer and consumer invariants

A future producer must authorize scope from protected host policy, verify retained
intent/request/admission and read analysis from one consistent snapshot before
emitting anything. Bind the change head, selection/review/intent pins, complete
target context and public Run to that same child; never reconstruct them from
current mutable catalog/deployment data. Request and attempt must differ, creation
must precede deadline, and issuer/principal namespaces must match the retained
claims. Lifecycle observations, when present, must refer to this same Run/request.
These cross-field, provenance and authorization checks are not proven by a schema.

Order decisions by provider publication instant then event ID. Require unique event
IDs and unique rule IDs within an event; preserve every distinct publication.
`late` means `publishedAt >= request.deadline`, not ingestion time or completion.
`conflict` means more than one distinct report-byte SHA-256, including differences
in report metadata. No newest-wins, winner field, aggregate PASS or approval exists.
The schema requires at least two decisions for a conflict; digest equality and
correct conflict/lateness claims remain producer/consumer semantic checks.

Reuse the verified native meanings: failed/unstable/unknown quality cannot produce
a decisive SLO/regression comparison. Non-PASS dimensions require reasons. Missing,
INCONCLUSIVE and NOT_EVALUATED evidence must not become PASS. Successful execution
or schema decoding confers neither baseline approval nor operational authority.

Bound the **complete encoded UTF-8 document** to 8 MiB before publication or parsing,
including correlation metadata. Also cap history at 1,000 events, rules per event
at 1,000 and reasons per dimension at 1,000; text has explicit schema bounds.
Historical values exceeding a public bound fail export; never truncate history,
selectors or reasons. Byte limits, calendar validity, equality and ordering require
explicit validation beyond portable structure. Reused timestamp codecs retain
calendar validation in Effect; generated timestamp fields retain their existing
plain-string wire schema. No existing contract is narrowed.

## Disclosure, verification and rollout

Only the listed fields may leave the producer. Raw reports, request/provider
payloads, approval bundles, database positions, storage paths/URIs, signed URLs,
credentials and arbitrary vendor errors are excluded. Published reasons are the
verified bounded native reason values, not appended logs or exception messages.
Even this metadata needs scoped authorization and protected CI artifact access;
this contract authorizes no anonymous publication or public HTTP endpoint.

Run `make generate-contracts`, `pnpm contracts:check`, `pnpm test`, `make verify`
and `make validate`. Contract tests exercise mixed/empty/conflicting/late history,
closed disclosure boundaries and malformed/bounded outcomes. PR11 must add the
independent authorization, read-only snapshot and all-or-nothing output gates;
PR8 must correlate these pins with actual execution and consumption. No storage,
runtime switch, migration or deployment change is made here. Reverting the new
schema removes this unused public family without changing retained evidence.

## Retained snapshot conversion

`selectionio.ProjectDecision` binds independently approved intent/request checksums
and scope before constructing the explicit Go wire DTO. It verifies original
selection/review/policy hashes, authoritative admission and frozen source context;
it uses no current catalog or expiry renewal. The read-only snapshot has already
revalidated all original report/review bytes. The converter rechecks ordering,
conflict and lateness, then validates the generated public schema and the complete
8 MiB output including newline. Failure/cancellation returns no bytes. This pure
operation grants no scope: the protected command must supply host-issued pins.
