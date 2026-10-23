# ADR-0053: Fetch authorized catalogs for inspection

- Status: Proposed
- Date: 2026-10-23
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

The pinned complete-snapshot validator exists. Operators need to retrieve and
inspect authorized metadata without treating partial pages, body claims or
catalog availability as permission to select or launch performance workloads.

## Decision

Add an HTTP adapter and a read-only inspection command. The composition root
wires the transport into the CLI's narrow fetch capability; vendor wire validation
remains in the contract adapter. Require independent authority/version configuration,
HTTPS except literal loopback HTTP, and an environment-provided bearer token.
Reject redirects and cookies. Keep cursors on the configured endpoint and bound
requests, bytes and elapsed time. Restart exactly once on 410, discarding prior
pages, and validate the complete view at fetch completion before exposing output.
No automatic retry occurs for other statuses. Errors exclude untrusted bodies,
credentials and cursor URLs; cancellation propagates to requests and reads.

## Consequences and compatibility

The additive command emits a page array for inspection, never a durable current
view or execution eligibility. Existing test-catalog, database and execution
contracts remain unchanged. No new dependency or shared transport framework is
needed. Caller-supplied clocks/HTTP clients are trusted embedding/test inputs;
the production CLI uses the actual clock and default verified TLS transport.
M09-PR4c owns transactional publication and freshness rechecks at later use.

## Verification and rollback

Use deterministic loopback HTTP tests with frozen fixture time, credential/query
assertions, bounded 410 recovery, redirect rejection, cancellation and failure
cases. Check CLI output/usage plus executable smoke coverage and `make validate`.
Rollback removes the command/configuration; there is no database migration or
published state to undo. See the [catalog guide](../integrations/perfeng-catalog.md).
