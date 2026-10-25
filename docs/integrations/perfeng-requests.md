# Perfeng request contract boundary

## TL;DR

- M09-PR7a pins integration messages; it creates no request or dispatch work.
- Decode/encode checks consistency, never authentication or execution authority.
- PR7 completes through reviewed compilation, durable intent/outbox and operator CLI.

## Pinned interchange

The [performance adapter](../../internal/performance/adapters/perfeng/integration.go)
adopts performance-integration/v1, API 1.0.0, from
[contracts commit 041273947703542f7fe22eab838c176df5517974](https://github.com/stanimirivanov/perfeng-contracts/tree/041273947703542f7fe22eab838c176df5517974),
bundle 0.24.0, matching the merged provider. The schema, OpenAPI and three published
exchanges live under its `snapshot/integration/` directory with byte checksum tests.
The parent snapshot Apache-2.0 license covers the copied artifacts. These are
externally owned pins, not Argus-generated Effect schemas.

`DecodeIntegration` and `EncodeIntegration` handle request, accepted, rejected,
lifecycle and cancellation messages. They enforce the 64 KiB bound, unique keys,
valid Unicode, closed shapes, formats and per-message identity/time semantics.
Only embedded schema resources resolve. Decode returns owned values; neither
operation performs I/O, consults a clock or mutates caller data. Errors expose no
provider diagnostics; unsupported generations also classify as `ErrIntegrationVersion`.

Request and attempt identities differ. Request/cancellation event source equals
the request issuer. Deadline follows creation, events cannot predate creation,
and acceptance precedes deadline. Late cancellation and lifecycle evidence remain
valid. JSON number revisions preserve integer-valued exponent syntax and the
published safe-integer bound; timestamps preserve offsets and microsecond precision.

## Authority and remaining work

Schema validity does not prove producer identity, principal scope, review authority,
freshness, reference association, or approval. Typed values remain transport claims.
Runtime consumers must verify expected provider/principal independently, retain
immutable context, reject conflicting deliveries and prevent terminal regression.
Successful completion does not establish a passing performance verdict.

Selection is a recommendation. Request compilation must require explicit trusted
owner approval and current source/catalog/policy evidence. Missing service,
capability or deployment evidence must never be inferred from display/local IDs.
See [the PR7 delivery plan](../development/plans/m09-pr7-performance-requests.md).

Run focused adapter/architecture tests and `make validate`. This foundation needs
no SQL gate, migration, credential, network endpoint or deployment enablement.
