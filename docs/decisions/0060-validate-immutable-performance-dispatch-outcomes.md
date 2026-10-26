# ADR-0060: Validate immutable performance dispatch outcomes

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Convert accepted, rejected and lifecycle readback messages into owned inward evidence.
- Validate the entire frozen request context and independently expected provider.
- Settle dispatch separately from ordered run lifecycle; enable no worker here.

## Decision and consequences

PR8b2a supplies the outcome boundary shared by subsequent storage and worker slices.
The Perfeng adapter validates both messages against the pinned schema, requires a
requested original, matches every context field and the provider/run issuer, and
rejects other message kinds. Callers must authenticate provider provenance and
configuration independently; neither wire claims nor these values grant authority.

Retain canonical EncodeIntegration bytes, not original HTTP response bytes. Payloads
are bounded to 64 KiB. NewOutcome validates inward identity, kind and payload bounds,
copies bytes and binds all metadata plus bytes with the versioned SHA-256 projection
`argus.performance-dispatch-outcome/v1` (JSON Version/Data). Data returns owned copies.
Changing this projection needs compatibility review. Validation errors expose no
provider content. No wall-clock lookup, I/O or external dependency is introduced.

Accepted/rejected admission responses and a correlated lifecycle lookup can settle
dispatch; a lifecycle snapshot does not establish revision ordering or run completion.
PR8b2b owns fenced atomic storage, exact replay, scoped reads and acquisition exclusion.
PR8c owns worker eligibility, authentication, dispatch and uncertain-admission recovery;
PR9 owns lifecycle ordering. Existing wire formats and transport return types remain
unchanged. Revert this unused boundary before dependent callers, or disable callers
first. Run focused outcome/adapter tests and `make validate`.
