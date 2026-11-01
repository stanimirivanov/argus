# Protected Linux analysis resources

## TL;DR

- Prepare owner/root-controlled resources before enabling analysis consumption.
- Descriptor-relative Linux opens reject symlinks and untrusted writers.
- File protection supplements independently authenticated issuance, not approval.
- Preparation starts no worker and installs no system trust.

`performanceAnalysisConfig.prepareAnalysis` pins the configured review directory
and constructs the existing consumer. Disabled mode opens nothing. Review roots
must be private (`0700` or stricter); ancestors must be owner/root-controlled and
not writable by group/others, except trusted sticky ancestors such as `/tmp`.
Keep these resources outside untrusted checkout/writable mounts.

Review opens are descriptor-relative, no-follow and nonblocking. Require private
regular single-link files, at most 2 MiB. This rejects links, FIFOs and devices
without following them or waiting for a writer. The caller owns closing returned
resources after any consumer has stopped. A pinned descriptor remains the root
if a pathname changes; loosened root permissions are rejected on subsequent opens.

Optional CA files must be regular, single-link, owner/root-controlled and not
writable by group/others, at most 64 KiB, with protected no-link ancestors. Valid
PEM extends system roots in a private verified transport for both provider and
report access; credentials remain separate per adapter. Invalid data fails.
No global trust store is modified. Linux is the supported runtime boundary.

Provision and authenticate review issuance and expected reviewer independently.
Ownership/modes cannot prove approval or scoped IAM grants. Existing consumer
checks still bind exact request/admission/policy/report evidence. Preparation
performs no provider or database I/O and activates no polling. Revert removes
this inactive preparation boundary without deleting resources or retained data.
