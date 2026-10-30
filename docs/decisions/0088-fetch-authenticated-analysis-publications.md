# ADR-0088: Fetch authenticated analysis publications

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Read one scoped publication using protected integration credentials.
- Bind original response bytes to authoritative frozen request/admission evidence.
- Keep artifact access and runtime activation in later slices.

## Decision and consequences

PR9d2c1 pins the separate API 1.1 read contract from perfeng-contracts bundle
0.26.0, commit `5bbeceec61f0ddd8922f27ceb6d4968cbdfef3ac`. Existing payload,
schema and native analysis pins remain unchanged. The callable transport checks
frozen request/principal and retained admission before network access, escapes the
opaque request ID once and includes the original issuer in the query.

Reuse the trusted IntegrationHTTP configuration: clone its client, disable cookies
and redirects, restrict endpoint schemes/origins, send Bearer credentials only to
the configured endpoint, and bound the exchange to ten seconds and 64 KiB. Require
JSON and full admitted publication correlation, retaining exact response bytes.
Close response bodies on success and failure; expose safe errors without raw body
or transport diagnostics. Preserve cancellation and status/positive delta-second
Retry-After hints for 429/503. Perform no automatic retries or writes. A scoped
404 cannot establish terminal state or authorize resubmission.

PR9d2c2 must supply separately authorized artifact access for the published S3
addresses, exact-byte verification and reviewed report conformance. Do not invent
an artifact HTTP route or send integration credentials to report/candidate/baseline
URIs. PR9d3 owns durable consumption and opt-in runtime activation. This slice adds
no dependency, migration, provider change or default runtime behavior.
