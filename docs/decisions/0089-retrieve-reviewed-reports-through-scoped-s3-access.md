# ADR-0089: Retrieve reviewed reports through scoped S3 access

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Use a dedicated read-only AWS identity and explicit bucket/directory prefix.
- Fetch one admitted report with the official SDK, then verify reviewed conformance.
- Keep runtime activation and durable retention in PR9d3.

## Decision and consequences

The maintainer selected direct S3 retrieval instead of an operator downloader.
PR9d2c2 adds a callable adapter with protected host region, credential provider,
bucket and slash-terminated prefix. IAM independently restricts `s3:GetObject` to
that prefix. Argus does not discover ambient credentials or provision IAM. Only
AWS-derived HTTPS endpoints are supported; no custom endpoints or presigned URLs.

Validate admission/review correlation and bounded integer size before network
access. Decode the admitted S3 key once, compare exact bucket/prefix, and reject
empty/dot path segments, controls and backslashes without normalizing object keys.
Fetch only the report; never candidate/baseline/raw artifacts. Keep integration
Bearer credentials separate from AWS signing credentials. The SDK handles SigV4,
endpoint selection and protocol errors; disable retries, cookies and redirects,
clone host HTTP configuration and cap the full request/body exchange at ten seconds.
Bound the read to 1 MiB plus one overflow byte and always close the successful GET
body before accepting evidence. Trusted credential providers/transports must honor
context cancellation. Return safe errors or owned exact-byte reviewed conformance,
including original admission/review pins and independent outcomes.

Adopt the provider-aligned pinned SDK rather than implementing signing. Dependency
admission is recorded in the dependency policy. This adds no migration, persistence,
background worker or default behavior. Deployment must grant a dedicated identity
and compose this adapter; live IAM/S3 validation remains a deployment follow-up.
Reverting removes the callable adapter and its SDK dependencies without data work.
