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
serves the upcoming review-input loader and report conformance verifier. Review
source selection, native/policy/baseline binding, complete rule checks, artifact
retrieval and durable decisions remain separate. There is no encoder, network,
clock, persistence or activation. See
[ADR-0085](../decisions/0085-validate-frozen-native-policy-claims.md).
