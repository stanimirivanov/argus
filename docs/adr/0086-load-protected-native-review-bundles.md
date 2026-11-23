# ADR-0086: Load protected native review bundles

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Use the selected protected operator/CI bundle for frozen native review inputs.
- Bind exact request/admission/native/policy pins and retain owned provenance.
- The trusted host protects issuance/origin/access; decoding cannot authenticate it.

## Decision and consequences

PR9d2b2b implements the user-selected operator trust model used by protected request
approval files. A trusted host authenticates the reviewer and supplies a protected
regular-file reader plus independently configured expected reviewer. It controls
trusted paths/parents and access permissions. Neither provider/agent input nor
self-declared bundle fields may select or authenticate this source.

The internal versioned bundle uses canonical json.Marshal(NativeReviewBundle)
bytes, with optional outer whitespace. Bound reads to 2 MiB, keep reader ownership
with the host and honor cancellation before/after reads and validation. Unknown,
duplicate, case-aliased or otherwise noncanonical fields fail closed. Errors return
zero evidence and safe classifications, without paths or policy contents.

Require the exact frozen request digest, authoritative settlement fingerprint,
provider/principal and full native binding from admitted publication. Decode exact
base64 policy bytes and verify their native policy ID/version/SHA-256. Validate
review ID/reviewer and producer/version/image/contracts provenance. Preserve the
exact input bundle digest and owned policy/baseline values; zero review has no digest.

This validates a host-authenticated historical review, not OS identities/ACLs,
current eligibility, new execution or baseline promotion. No policy body is
reconstructed from current catalog state. PR9d2b2c still verifies complete report
rule/section/native/baseline conformance; PR9d2c supplies authenticated artifact
retrieval. No new dependency, migration, shared wire schema, CLI/runtime activation,
network, clock or report outcome computation is added. Revert this callable loader
without changing stored data; retain protected historical review material for its
future consumers.
