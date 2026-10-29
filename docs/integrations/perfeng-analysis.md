# Perfeng analysis artifact boundary

## TL;DR

- PR9d1 validates published analysis/v1 claims offline, without fetching artifacts.
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
