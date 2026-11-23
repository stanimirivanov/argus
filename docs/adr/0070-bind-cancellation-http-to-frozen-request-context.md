# ADR-0070: Bind cancellation HTTP to frozen request context

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Bind one authenticated cancellation POST to the entire original frozen context.
- Require HTTP status/state agreement without inferring termination or rerun authority.
- Keep durable intent, cutoff, operator activation and reviewed reruns in later slices.

## Decision and consequences

PR10a adds `IntegrationHTTP.Cancel(ctx, frozen, cancellation)` over the pinned
endpoint. Configuration/principal and complete frozen context must match before
one authenticated POST of the exact cancellation bytes. The escaped original
identity determines the URL. Calls retain ten-second and response bounds,
cookie/redirect isolation, resource cleanup, cancellation and redacted errors.

Only lifecycle evidence from the configured provider/run issuer is accepted.
HTTP 202 requires CANCELLING; HTTP 200 requires CANCELLED, COMPLETED or FAILED.
Late cancellation is valid and completion may win the race. Neither an intent,
acknowledgement, elapsed deadline nor a transport error proves termination or grants
rerun authority. The caller must enforce retained run identity and lifecycle order.

`IntegrationHTTPStatusError.RetryAfter` adds an optional positive seconds-based
delay for cancellation 429/503 responses, bounded by duration representability.
Absent/invalid hints and other exchanges retain zero; existing status/error text
is unchanged. The transport never sleeps, retries or generates identities.

Activation requires independent authorization, durable intent, first-send cutoff
and reconciliation in later slices; reruns require fresh approval and identities.
No runtime, storage, dependency or wire changes occur. Revert preserves data.
See the [PR10 plan](../development/plans/m09-pr10-cancellation-reruns.md).
