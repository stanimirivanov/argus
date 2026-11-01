# ADR-0097: Validate protected analysis configuration

- Status: Proposed
- Date: 2026-11-01
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Validate disabled-by-default host-issued analysis policy without side effects.
- Bind explicit provider/principal, reviewer, review path and report credentials.
- Keep file protection, CA loading, IAM provisioning and activation separate.

## Decision and consequences

PR9d3e's configuration slice adds a pure command-root parser. Only an explicit
true enable flag validates further inputs; disabled mode reads nothing else.
Provider scope, absolute request issuer and independently expected reviewer come
from trusted host input, not provider publications or protected bundle claims.
Report access uses separate explicit key/secret and optional static session token;
never discover ambient credentials or substitute the provider Bearer token.
Capture credential bytes unchanged; failures return no partial configuration and
constant errors that do not echo secrets, paths or endpoints.

Require clean absolute non-root review-directory and optional CA-file paths.
Parsing proves syntax only: no file opens, ownership checks, certificate loading,
network probes or database access. Later activation must independently verify
protected issuance/access, safe bounded regular-file reads, reviewed admission
pins, CA validity and scoped read-only object-store grants. An absolute path or
file mode cannot establish reviewer authority. Never log parsed configuration.

Reuse existing integration and S3 adapter validation, including protected HTTPS
endpoint/addressing policy. Defaults are five-second polling, thirty-second
consumption timeout and one item per tick. Timing accepts one second through
one minute in whole microseconds; work accepts one through one hundred items.
These are resource ceilings, not performance or fairness guarantees. Activation
must respect caller cancellation and the adapters' shorter deadlines.

Analysis reads are independent of new-request dispatch enablement. The parser itself starts no worker. PR9d3e runtime activation now invokes it
at startup and enforces descriptor-relative Linux resource checks before opening
the database; the integration guide records polling and shutdown behavior. This adds no wire contract,
migration, dependency, roles or deployment mutation. Revert removes only this
inactive boundary; existing dispatch and retained evidence remain unchanged.

## Validation

Cover disabled getter isolation, incomplete scope/credentials, unsafe resource
paths/endpoints, exact secret bytes, independent frozen credential values,
provider cancellation, defaults/boundaries and safe zero-value failure. Accept
syntactically valid absent paths without touching them; do not claim readiness
or production authorization from configuration-only tests.
