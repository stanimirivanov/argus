# ADR-0096: Support protected S3-compatible endpoints

- Status: Proposed
- Date: 2026-11-01
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Add optional protected root HTTPS endpoint and path-style configuration.
- Keep AWS endpoint/addressing defaults when those settings are omitted.
- Preserve report scope, signing, TLS, limits and reviewed conformance.
- Reuse the pinned AWS SDK; add no dependency, discovery or runtime activation.

## Decision and consequences

The approved joint-development plan requires reports from a local S3-compatible
store. Extend ADR-0089's endpoint restriction without changing its authority,
credentials, artifact access or verification boundaries. Host policy selects one
root HTTPS origin, bucket, directory prefix, signing region and addressing mode.
Publication content cannot select a network destination; evidence keeps S3 URIs.

Use SDK BaseEndpoint and UsePathStyle with its default resolver, following
[the official endpoint guidance](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-endpoints.html).
Do not replace signing/resolution or add an S3 library. Empty endpoint and false
path-style setting preserve AWS behavior. SDK virtual hosting can prefix the
bucket; the host must provision the corresponding DNS/certificate coverage.

Reject ambiguous origins, credentials in URLs, path prefixes, query/fragment
markers, malformed hosts/ports and insecure HTTP. Host-provided CA roots may
extend verified trust. Standard transports for custom endpoints cannot disable
verification or override the verified server name. Custom transports remain a
trusted host boundary and must enforce verification, cancellation and deadlines.
No public insecure-TLS switch, ambient credential lookup or CA installation exists.

Preserve independent GetObject-only credentials, exact bucket/prefix scope,
no redirects/cookies, no SDK retries, bounded report bytes/time and reviewed
conformance. This additive internal configuration change starts no runtime work.
Rollback requires clearing incompatible configuration before reverting source;
retained report references and evidence are unchanged. Real-store provisioning
and acceptance belong to later joint composition.

## Validation

Exercise existing AWS behavior, both SDK address modes, escaped object keys,
SigV4 headers and safe endpoint rejection. Use real local TLS to prove configured
CA trust, unknown-CA rejection and no redirected request, plus foreign bucket/
prefix refusal before network access. Preserve existing bounded failure and
cancellation tests; no live object-store acceptance is inferred from TLS fixtures.
