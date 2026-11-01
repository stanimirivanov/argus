# Perfeng analysis artifact boundary

## TL;DR

- PR9d1 validates published analysis/v1 claims offline, without fetching artifacts.
- PR9d2a validates the separate request-linked publication claims before correlation.
- Quality, SLO, regression and uncertainty stay independent; no overall PASS exists.
- Authenticate/correlate evidence before later decision consumption or activation.

## Published pins

`perfeng.DecodeAnalysis` consumes analysis/v1 from
[contracts commit 041273947703542f7fe22eab838c176df5517974](https://github.com/stanimirivanov/perfeng-contracts/tree/041273947703542f7fe22eab838c176df5517974),
bundle 0.24.0. Its schema and three published examples are copied byte-for-byte
under `internal/performance/adapters/perfeng/snapshot/analysis/`, covered by the
parent snapshot Apache-2.0 LICENSE and checksum tests. These are external pins,
not Argus-generated Effect contracts. `schemaVersion` selects generation one;
`contractsVersion` records the producer's bundle provenance, not an exact bundle
equality requirement. Published examples legitimately record earlier bundle versions.

## Validation and preserved meaning

The decoder bounds input to 1 MiB, rejects duplicate/unknown keys, invalid Unicode,
trailing/deep JSON and invalid schema/format values. Only embedded schema resources
resolve. Unsupported numeric generations additionally classify as ErrAnalysisVersion;
other failures return ErrAnalysis and zero output without provider diagnostics.
Before schema arithmetic, numeric tokens are limited to 128 bytes and exponents
to -999 through 999, bounding rational expansion while retaining exact accepted tokens.
Calls own returned values and are concurrent-safe, without clocks or I/O.

The published policy semantics require unique rule IDs, reasons for non-pass
outcomes, and no decisive SLO/regression when quality is non-pass. Candidate
artifact run ID must match the report; comparison references must identify listed
reference artifacts, not the candidate. Artifact IDs cannot describe multiple inputs.
Missing sample counts stay missing, exact numeric tokens retain precision, and
reported statistics/effects are never recomputed. SLO PASS can coexist with
regression FAIL; missing references may leave SLO decisive but regression inconclusive.
Only observe/inform with blocking=false is supported. Neither inform nor a completed
run authorizes external statuses/comments, release blocking or an overall pass.

## Authority and remaining work

### Request-linked publication claims

`DecodePublication` and `EncodePublication` validate the separate
performance-analysis/v1 envelope from
[contracts commit cd2442f433ce93d9a886c14ad6ff8aa8488a623b](https://github.com/stanimirivanov/perfeng-contracts/tree/cd2442f433ce93d9a886c14ad6ff8aa8488a623b),
bundle 0.25.0. Schema/example snapshots live in `snapshot/publication/`, covered by
the parent snapshot Apache-2.0 license and exact checksum tests. Older pins remain
unchanged. This family cannot enter the closed integration lifecycle decoder.

The local metadata bound is 64 KiB, separate from the 1 MiB analysis artifact bound.
Reuse unique-key/depth/Unicode validation and the numeric token/exponent limits
above before offline schema arithmetic. Preserve original immutable context, scoped
public Run, explicit native Run/test/workload/policy and nullable baseline identity/
artifact claims. Enforce context/time, source/run issuer, report/native Run and
baseline-null consistency; reject credential-bearing/mutable artifact URLs. Late
publication is valid. Exact numeric syntax and timestamp offsets remain unchanged.

Decoded values own nested data and grant no authenticated reference, admission or
fetch authority. PR9d2b1 supplies admission correlation; PR9d2b2 supplies independently
reviewed frozen native association/rule coverage. PR9d2c
supplies authenticated reference/byte retrieval. See
[ADR-0083](../decisions/0083-pin-request-linked-analysis-publications.md).

Decoded producer/run/workload/policy/reference fields are claims. This artifact is
not an analysis-completed request envelope. Before request-linked consumption,
authenticate delivery and immutable analysis-reference origin, verify artifact
checksum/size, and bind exact admitted run, frozen context, workload, policy and
approved baseline associations through a jointly reviewed provider mapping.
Never infer native workload IDs from catalog labels/aliases. Compare complete rule
coverage and configured/unconfigured sections against the reviewed policy too;
artifact validation alone cannot establish those external facts.

PR9d2 owns these correlation/authority gates; PR9d3 owns durable decision evidence
and explicit runtime consumption. No fetches, persistence, commands, decision
aggregation or runtime activation occur here. URI strings are retained references;
Argus does not read raw k6 or normalized-result bytes. Revert removes only the
callable decoder. See [ADR-0082](../decisions/0082-validate-native-analysis-with-independent-outcomes.md).

## Publication admission binding

`BindPublicationAdmission` takes the exact frozen request, authoritative retained
settlement and independently authenticated provider/principal scope. It reuses the
lifecycle admission binder, compares the entire original context and admitted public
Run, and returns owned exact publication bytes/digest with the frozen request and
settlement pins. Accessors copy bytes; reject the zero value (empty digest). Storage
must compare those pins with its authoritative scoped rows before retaining evidence.

Publication cannot precede a retained acceptance timestamp. Lifecycle-recovery
snapshots do not expose the original admission time and can occur after publication;
their event times are not substituted for it. Valid late evidence does not change
lifecycle or imply a verdict. Inputs are bounded before copying and codecs perform
no I/O, clock lookup or URI access.

This binding grants no native resource/policy/baseline approval or artifact-fetch
authority. PR9d2b2 must establish independently reviewed frozen native associations,
exact policy bytes and complete rule coverage; PR9d2c must authenticate reference/
byte retrieval and verify checksum/size. No identities are inferred from labels or
public Run IDs. See [ADR-0084](../decisions/0084-bind-publications-to-frozen-admission.md).

## Native policy claims

`DecodeNativePolicy` validates policy/v1 from contracts commit
041273947703542f7fe22eab838c176df5517974, bundle 0.24.0. Exact schema and browser/
checkout examples live in `snapshot/policy/`, covered by the parent Apache-2.0
license and checksum tests. This is native performance policy, distinct from
Argus selection policy or an authenticated review record.

The 1 MiB offline decoder reuses Unicode/unique-key/depth and numeric-token limits.
Owned fields preserve metadata, nonblocking mode/missing-data behavior, full rule
IDs/selectors, optional quality/SLO and regression/baseline definitions. Numeric
tokens remain exact; omitted sections and zero thresholds remain distinct. Reject
duplicate IDs/selectors and reversed inclusive SLO ranges, comparing the latter
with bounded exact rationals rather than rounding. No report outcome is recomputed.

Hash the original bytes against an independently trusted frozen policy pin before
consumption; decoding grants no approval or request association. This foundation
serves the protected review-input loader and report conformance verifier below.
Complete report/native/policy/baseline rule checks, artifact
retrieval and durable decisions remain separate. There is no encoder, network,
clock, persistence or activation. See
[ADR-0085](../decisions/0085-validate-frozen-native-policy-claims.md).

## Protected native review bundle

`LoadNativeReview` consumes the selected protected operator/CI source. The trusted
host authenticates issuance, protects the file path/parents/access and supplies
the expected reviewer independently of provider/agent input. Do not authenticate
review authority by echoing a publication or reading its reviewer from the bundle.
File mode or metadata alone is not proof of protection. The host opens and closes
the regular file; the loader accepts its reader without taking ownership.

Generate canonical `json.Marshal(perfeng.NativeReviewBundle)` bytes; outer whitespace
is allowed. Unknown/duplicate/case-aliased or reformatted fields are rejected.
The internal format contains:

| Field | Meaning |
|:--|:--|
| `apiVersion` | `argus.dev/performance-native-review/v1` |
| `reviewId`, `reviewedBy` | Host-issued review identity and independently expected reviewer |
| `provider`, `principal` | Exact authenticated provider and principal issuer/ID |
| `requestSha256` | SHA-256 of the exact frozen request bytes |
| `admissionSha256` | Authoritative retained outcome's `Fingerprint()` |
| `native` | Independently reviewed full native Run/test/workload/policy/baseline binding |
| `policyBytes` | Base64 of exact original native policy bytes; never re-encode before hashing |
| `producer`, `contractsVersion` | Reviewed native analysis producer name/version/image and bundle provenance |

Reads are limited to 2 MiB, policy bytes to 1 MiB. Match all request/admission/scope/
native pins to `PublicationAdmission`, then decode policy and check native ID/version/
SHA-256. Returned opaque evidence owns policy and baseline data and includes SHA-256
of the exact input file bytes, including outer whitespace. Zero review has no digest.
Read errors and cancellation return no partial review or private diagnostics.

This is historical review validation, not OS authentication, current approval
eligibility, artifact access or report verdict verification. PR9d2b2c still checks
complete report/native/policy/baseline/rule associations; PR9d2c authenticates byte
retrieval. No CLI/runtime wiring, clock, network or persistence is introduced.
See [ADR-0086](../decisions/0086-load-protected-native-review-bundles.md).

## Reviewed report conformance

`VerifyReviewedAnalysis` rechecks admission/review pins and exact report size/hash,
then validates native Run/test/policy/mode, reviewed producer/contracts provenance,
candidate Run, time ordering and non-self artifact associations. Compare numeric
artifact sizes exactly, preserving original tokens. Reference metadata must equal
the admitted baseline, and configured policy baseline IDs/versions must match.

Require complete unique rule coverage, exact selectors and configured versus
NOT_EVALUATED sections. Quality PASS requires configured measurements to be present.
Missing reference may remain inconclusive; no native baseline allows no comparison
evidence. Preserve independent quality/SLO/regression/uncertainty outcomes, including
SLO PASS with regression FAIL. Do not recalculate thresholds/statistics/effects or
derive overall PASS. Late/terminal evidence stays valid without lifecycle projection.

Returned opaque evidence owns report bytes and retains admission/review/report pins;
zero evidence grants nothing. This is offline conformance, not authenticated fetch,
current eligibility or durable decision projection. PR9d2c still owns authenticated
origin/redirect/response bounds and PR9d3 durable consumption. See
[ADR-0087](../decisions/0087-verify-reviewed-analysis-conformance.md).

## Authenticated publication lookup

`IntegrationHTTP.LookupPublication` performs one GET of
`/integration/v1/requests/{requestId}/analysis?issuer=...` after validating the
original frozen request and authoritative retained admission. Host configuration
supplies the protected token, provider, principal and trusted endpoint/client.
The separate `snapshot/publication/read-openapi.json` pins API 1.1 from contracts
bundle 0.26.0, commit `5bbeceec61f0ddd8922f27ceb6d4968cbdfef3ac`; older payload,
schema and native pins remain unchanged.

The client disables redirects and ambient cookies, sends no body, and bounds each
exchange to ten seconds and 64 KiB. Only JSON with the full admitted context and
public Run becomes owned `PublicationAdmission` evidence with original bytes.
Failures return no admission. HTTP statuses remain inspectable; positive integer
Retry-After seconds are advisory only for 429/503. The caller owns any retry policy.
A 404 means unavailable within the authorized scope, never terminal state or proof
that execution did not occur. Cancellation and body/read/close errors fail safely.

This callable boundary does not read artifact URIs or activate polling. Separately
authorized S3 access and `VerifyReviewedAnalysis` conformance belong to PR9d2c2;
durable retention/runtime consumption belong to PR9d3. See [ADR-0088](../decisions/0088-fetch-authenticated-analysis-publications.md).

## Scoped S3 report retrieval

`NewAnalysisS3(AnalysisS3Config)` accepts a protected host region, dedicated AWS
credential provider, exact bucket, nonempty slash-terminated key prefix and optional
HTTP client. IAM must independently allow only `s3:GetObject` on the matching object
prefix (for example `arn:aws:s3:::perfeng-artifacts/reports/*`), without list/write
permission. The adapter performs no ambient credential discovery. Credential
refresh and transport implementations must honor contexts and support concurrent
calls. Provider integration Bearer tokens are never used for S3.

`Fetch(ctx, admitted, review)` checks opaque admitted/reviewed scope and size before
one signed GET of the admitted report URI. Only `s3://` in the configured bucket
and directory prefix is accepted; reject dot/empty key segments, controls and
backslashes after one URI decoding. No redirect or candidate/baseline/raw download occurs. Custom endpoints require
the protected host configuration described below. Each call has a ten-second deadline and 1 MiB report
limit, disables SDK retries/cookies, closes its body and invokes
`VerifyReviewedAnalysis` over original bytes. Wrong media, size/checksum, review,
conformance, read/close or provider failures return zero evidence and safe errors;
cancellation remains identifiable. Callers own retries; failure implies no verdict.

This callable adapter is not activated by configuration or a background worker.
Runtime/CLI credential composition and durable consumption remain PR9d3; scoped
IAM provisioning/live S3 verification are deployment follow-ups. See
[ADR-0089](../decisions/0089-retrieve-reviewed-reports-through-scoped-s3-access.md).

## Inward reviewed analysis evidence

`AnalysisObservation(ctx, checked)` converts opaque `ReviewedAnalysis` to
`requestintent.AnalysisEvidence` after rechecking original publication, protected
review and report pins/associations. `NativeReview.Data().Payload` now returns
owned original file bytes, preserving whitespace alongside its existing SHA-256.

The inward value owns all three bounded JSON buffers (64 KiB publication, 2 MiB
review, 1 MiB report), immutable admission/request/Run, principal, event ID and
full-precision UTC publication time. Its versioned fingerprint binds these pins
and exact-byte digests. Every accessor copies mutable buffers. Inward construction
checks bounds/invariants, while vendor validation and original authentication stay
at the adapter boundary. It grants no persistence, execution or baseline authority.

Independent outcomes remain in original report bytes; no aggregate verdict or
lifecycle transition is synthesized. Storage must compare authorized scope and
retained admission before accepting evidence. Atomic retention, scoped projection
and opt-in consumption follow in PR9d3b onward. See
[ADR-0090](../decisions/0090-preserve-inward-reviewed-analysis-evidence.md).

## Analysis retention schema

PR9d3b1 expands `argus_catalog.performance_analysis_events` with scoped original
publication/review/report bytes, exact digests and inward fingerprint. Database
checks require a retained non-rejected admission, original request SHA and matching
Run. Provider event IDs cannot be reused across analysis, admission or lifecycle
families; pending-row/event locks serialize those checks. Distinct analysis events
can be retained up to 1000 per request for later conflict projection. The next
atomic storage slice must reconcile exact retries before trying to append. Guards
require READ COMMITTED, matching runtime writers; direct snapshot writes fail.

Store publication time as microsecond-truncated `published_at` plus
`published_submicro_ns` (0..999); reconstruct by addition without rounding. Keep
`recorded_at` separate. Original-byte size/JSON/digest checks and immutable triggers
do not replace native wire/review/fingerprint validation or authorize access.

The additive transactional migration has no backfill or activation. Deploy it
before later analysis writers; keep the table on application rollback. Trigger
installation briefly locks admission/lifecycle tables. Existing runtime trigger
paths require SELECT on this table: establish default privileges or pause workers,
migrate, reapply existing runtime grants, then resume. Later append writers also
need INSERT; UPDATE/DELETE/TRUNCATE remain prohibited. No roles/grants are created
by this migration. See [ADR-0091](../decisions/0091-guard-append-only-analysis-retention.md).

## Atomic analysis append and exact reads

`PerformanceStore.AppendAnalysis` locks scoped pending work, compares authoritative
stored request/admission, and invokes a pure bounded `AnalysisVerifier` against the
stored frozen child. Configure `perfeng.RevalidateAnalysis` with an independently
trusted expected reviewer. The verifier performs no network/file I/O and returns
revalidated inward evidence; fingerprints and recovery anchors must agree.

An identical event retry returns `false, nil`, including at capacity. Changed reuse
conflicts; corrupted retained evidence fails unavailable. A lost commit response
requires exact retry or `PerformanceStore.AnalysisEvidence` for that event, never a
fresh provider submission. Exact reads use a read-only repeatable-read snapshot,
preserve original bytes/nanoseconds and return not found for unknown/wrong scope or
event, conflict for a known mismatched admission, and no partial evidence on failure.

Each read returns one bounded evidence bundle. Broader history traversal and
independent decision projection follow in PR9d3c; network consumption and opt-in
activation remain later slices. No migration or new approval authority is added.
See [ADR-0092](../decisions/0092-append-and-read-scoped-analysis-evidence-atomically.md).

## Scoped independent analysis projection

`PerformanceStore.AnalysisProjection` traverses one authorized request/admission
under a read-only repeatable-read snapshot. It scans at most 1000 event IDs, then
uses indexed exact-row verification one bundle at a time. Configure the existing
pure verifier and `perfeng.ProjectAnalysisDecision`; neither callback may perform
I/O or publish intermediate results. The existing operation deadline applies.
Concurrent appends appear in the next snapshot. Any failure/corruption returns
zero output; unknown scope is not found and known empty history returns an empty
list, never a synthetic PASS.

Results retain each publication's event/evidence/report pins, full provider time,
lateness and independent per-rule quality/SLO/regression statuses and reasons.
Sort by publication time then event ID, preserving late evidence. Different report
byte identities set `Conflict`, conservatively including metadata differences;
all events remain visible and no newest-wins or aggregate verdict is selected.
Exact duplicates collapse; changed reuse fails. Measurements/effects remain in the
pinned original report available through exact-event reads.

The inward collector copies mutable data and caps serialized compact results at
8 MiB; exceedance fails rather than returning a truncated projection. There is no
runtime activation, migration or new approval/lifecycle authority. See
[ADR-0093](../decisions/0093-project-bounded-scoped-analysis-history.md).

## One-publication consumption

`AnalysisConsumer.Consume` composes existing lookup, protected review loading,
scoped report retrieval, inward conversion and atomic retention. The caller
supplies independently authorized scope, frozen pending work, `AnalysisStore`,
separate provider/report clients and the independently expected reviewer. It
opens a protected regular review file and retains ownership of closing it;
other reader implementations must honor context cancellation. Never source
review authority or credentials from publication metadata.

Read authoritative retained settlement first; at most one authenticated
publication GET and one signed report GET follow. Existing adapter time/byte
limits apply. Failed review prevents report access. Network and file access
finish before storage invokes the pure `RevalidateAnalysis` callback against
its authoritative stored child and admission. A failed stage returns zero
analysis evidence and does not advance subsequent stages.

An append error permits one exact scoped read for the attempted event ID and
admission using the same pure verifier. Only identical evidence fingerprints
resolve the uncertain acknowledgement. Absent, changed or corrupt readback
preserves the append failure; later/newer history cannot prove this append.
There is no provider submission, in-call retry or fallback to latest evidence.
Cancellation after a write may leave retained evidence; callers must reconcile
that exact event rather than assume cancellation rolled back storage.

Terminal execution does not suppress late analysis. Quality, SLO, regression,
uncertainty and conflicts retain their existing independent meanings. This
callable operation starts no worker or polling and grants no execution or
baseline authority. PR9d3e still owns explicit runtime/credential composition;
reverting this consumer leaves retained evidence untouched. See
[ADR-0094](../decisions/0094-consume-one-reviewed-analysis-publication.md).

## Admitted analysis traversal

`PerformanceStore.NextAnalysis(ctx, scope, issuer, after)` selects at most one
original frozen child with retained `accepted` or `lifecycle` admission, under
exact provider/principal and request/attempt issuer. The exclusive database-local
position cursor is not a public Run identity or processing claim. Invalid input
fails before opening storage; missing/end/wrong scope returns `ErrNotFound`, and
all failures return zero work. The existing operation deadline bounds the read.

Calls see currently committed admission at each statement, not one snapshot for
a whole campaign. Callers advance only with the returned position, reset to zero
on a fair later traversal, and apply their own bounded retry policy. Admission
that commits behind a cursor appears only after reset; zero is an explicit
restart, not automatic wrap or proof that all future work is complete. Multiple
readers may select the same request; retention must remain idempotent.

Rejected/unsettled requests are excluded. Terminal execution, cancellation intent,
future cancellation cooldown and prior analysis do not suppress selection: each
may still receive late or conflicting analysis. This selector deliberately does
not reuse `NextLifecycle`'s cooldown filter. Enumeration grants no credentials,
report-read permission, claim, new submission or execution authority; consumers
must still revalidate authoritative admission and protected review.

Use the existing scoped pending-position index and settlement primary key; no
schema or scheduling state is added. PR9d3e owns runtime cursor rotation and
consumption. Revert removes the callable read without modifying retained work.
See [ADR-0095](../decisions/0095-enumerate-admitted-analysis-work.md).

## Protected S3-compatible endpoints

`AnalysisS3Config.Endpoint` optionally selects a host-approved root HTTPS origin,
for example `https://localhost:9443`. Empty retains AWS-derived endpoints.
`UsePathStyle: true` puts the configured bucket in the request path; false keeps
normal SDK addressing, including virtual-host addressing when supported. DNS,
certificate coverage and endpoint policy must account for the bucket subdomain
in virtual-host mode. IP endpoints may use SDK path-style fallback. Region remains
an explicit signing region, such as `us-east-1` for a compatible local store.

Only HTTPS DNS/IP origins with optional valid port and optional trailing root
slash are accepted. Reject userinfo, query/fragment (including empty markers),
proxy path prefixes, encoded paths, zoned IPs and malformed host/port values.
Endpoint/addressing must come from protected operator configuration, never the
publication, report URI or agent input. `s3://bucket/key` evidence references do
not change; configured bucket/prefix and reviewed conformance still gate access.
This is an additive adapter configuration change, not runtime activation.

Use a dedicated `GetObject` identity restricted to the reviewed report prefix.
For a private CA, supply a trusted `http.Client` with a transport whose `RootCAs`
contains the approved CA. Retain certificate and hostname verification. Custom
endpoint configuration rejects standard transports with `InsecureSkipVerify` or
an explicit `ServerName` override; certificate identity follows the resolved host.
Arbitrary RoundTrippers remain trusted host code and must uphold TLS, deadlines
and cancellation themselves. Protect configuration/transports from concurrent
mutation. This adapter installs no CA and discovers no ambient credentials.

The pinned AWS SDK handles endpoint resolution, addressing and SigV4. Existing
single-attempt/ten-second/1 MiB limits, cookie removal, redirect rejection, exact
checksums and review validation remain unchanged. Errors expose no remote details
or partial evidence. Caller-supplied client state is not rewritten.

Runtime enablement, secret delivery, object-store provisioning and real-store
acceptance remain later composition slices. Rollback to AWS-only code requires
removing custom endpoint/addressing configuration first; it cannot make local
objects available through AWS. Retained references/evidence remain untouched.
See [ADR-0096](../decisions/0096-support-protected-s3-compatible-endpoints.md).

## Protected analysis configuration validation

`loadPerformanceAnalysis` in the control-plane composition root validates the
`ARGUS_PERFORMANCE_ANALYSIS_` settings below. Startup now invokes it: explicit
enablement prepares protected review resources and CA trust before database
startup. Disabled mode opens no analysis resources and starts no analysis worker.

| Suffix | Validation and meaning |
|:--|:--|
| `ENABLED` | Empty/`false` returns inert configuration and reads no other settings; only `true` opts into validation. |
| `URL`, `TOKEN` | Explicit integration endpoint and provider credential; reuse the existing integration transport policy. |
| `PROVIDER`, `PRINCIPAL_ISSUER`, `PRINCIPAL_ID`, `REQUEST_ISSUER` | Independent host-selected provider/principal and absolute request issuer; no scope is inferred from retained/provider messages. |
| `EXPECTED_REVIEWER` | Independently trusted reviewer identity, nonblank, no controls, at most 256 bytes; never read from the review bundle. |
| `REVIEW_DIRECTORY` | Required clean absolute non-root path to protected per-request review material. |
| `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY` | Required dedicated report-read credentials; never reuse the integration Bearer token as S3 credentials. |
| `S3_SESSION_TOKEN` | Optional explicit static session token. Credential values are nonblank, control-free and at most 4096 bytes each. |
| `S3_REGION`, `S3_BUCKET`, `S3_PREFIX` | Required signing region, exact bucket and slash-terminated report prefix, validated by the existing S3 adapter. |
| `S3_ENDPOINT`, `S3_PATH_STYLE` | Optional protected HTTPS origin and boolean SDK path-style choice; omitted values retain AWS defaults. |
| `CA_FILE` | Optional clean absolute non-root path to host-approved CA material; omission uses normal verified system trust. |
| `POLL_INTERVAL` | Default `5s`; explicit values must be `1s`..`1m` in whole microseconds. |
| `TIMEOUT` | Default `30s`; same bounds, intended as a total per-consumption deadline, preserving the adapters' shorter deadlines. |
| `MAX_PER_TICK` | Default `1`; integer `1`..`100` to cap work attempted per tick. |

Disabled configuration is independent of dispatch settings. Later analysis reads
must remain possible with new-request dispatch disabled. Secrets and resource
paths retain their original bytes; no trimming, shell expansion, ambient AWS
credential chain, file reads or network calls occur during validation. The
credential provider captures explicit values and honors cancellation. Static
session credentials need explicit configuration replacement when expired; no
refresh/rotation authority is introduced here. Never log configuration objects.

Path syntax is not authentication, protection, file existence or CA validity.
The host must authenticate review issuance and expected
reviewer independently, protect the directory/files and parents from untrusted
writers, open bounded regular review files safely and check exact scoped pins
through the consumer. It must validate and load the selected CA material into a
verified transport before access. Scope and GetObject-only IAM grants must also
be independently provisioned and checked. Do not infer approval from modes,
file metadata or the bundle's self-declared reviewer.

Every enabled validation failure returns zero configuration with constant,
secret-safe diagnostics. The parser does not prove endpoint reachability, grants
or readiness. These requirements constrain later activation; they do not justify
network probes, trust-store writes or credential provisioning during parsing.
See [ADR-0097](../decisions/0097-validate-protected-analysis-configuration.md).

## Bounded analysis runtime

Explicit enablement starts analysis independently of dispatch. Linux startup pins
an owner/root-controlled directory descriptor, rejecting symlink ancestors and
untrusted writers (trusted sticky ancestors such as `/tmp` are allowed). The
review root must be private (`0700` or stricter). Provision resources outside
untrusted checkout/writable mounts; ownership/modes do not authenticate issuance.
For each frozen request, provide `<request SHA-256>.json` containing its separately
issued native review. Descriptor-relative no-follow/nonblocking opens require a
private regular single-link file, bounded to 2 MiB. Missing/unsafe review is an
unresolved observation, never permission to infer or generate approval.

Optional CA files must be regular, single-link, owner/root-controlled and not
writable by group/others, at most 64 KiB, without symlink ancestors. Valid PEM
extends system trust in a private verified transport; system stores are not
changed. Invalid resources fail startup before database access.

Each tick attempts at most `MAX_PER_TICK` requests; selection plus consumption
shares per-item `TIMEOUT`. Advance past individual failures and reset only on
end/selection error for the next tick. Publication-provider 429/503 ends the batch and respects the
larger of polling interval and Retry-After. Other unresolved work is revisited
on later rotations; restart resets the process-local cursor/backoff. No in-call
resubmission or implicit permanent completion occurs, including terminal Runs.

Shutdown cancels and joins analysis work before closing its directory, idle TLS
connections and database pool. Diagnostics contain fixed status messages, not
errors, paths, identities or credentials. Disable/drain before changing mounts,
credentials or source; retain evidence. Existing consumer exact-read recovery and
idempotent retention remain authoritative. Real-provider/k6 acceptance remains
the joint fixture's responsibility, not proof from resource/scheduling tests.
