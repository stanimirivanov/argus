# Authorized performance evidence export

## TL;DR

- Use `performance-evidence` for one independently authorized admitted child.
- Provision protected host approval and a dedicated restricted database login first.
- Arguments identify scope/position; they grant no permission.
- Accept output only after exit zero; retain it as a protected CI artifact.

## Provision and run

An authenticated operator/CI authority provisions
`ARGUS_PERFORMANCE_EVIDENCE_APPROVAL_FILE` outside submitted checkout/input control.
Run a reviewed binary in a trusted operator/CI job; never execute subject-controlled
code with this policy or database credential. It is canonical `json.Marshal(evidencecli.Approval)` JSON (outer whitespace allowed),
with exactly these fields in order:

- `scope`: existing inward keys `Issuer`, `PrincipalIssuer`, `PrincipalID`.
- `position`: exact retained child position, a positive integer.
- `intentFingerprint`: independently approved retained intent fingerprint.
- `requestSha256`: independently approved exact frozen request checksum.
- `expectedReviewer`: independently authenticated native-analysis reviewer.

Do not derive authority by hashing a subject's submitted evidence and trusting its
own reviewer claims. File ownership/modes supplement trusted issuance. Files must
be private regular single-link files, at most 64 KiB, owned by the current user or
root. Descriptor-relative Linux opens reject symlinks, FIFOs/devices and untrusted
writers in any ancestor. Trusted sticky ancestors such as `/tmp` are allowed.
Permissions are checked before reading; this command neither creates nor repairs policy.

`ARGUS_PERFORMANCE_EVIDENCE_DATABASE_URL` is separately provisioned by the host,
with authenticated server transport appropriate to that environment. There is no
fallback to the application's write credential. The login must have no administrative
attributes, role memberships, database/table ownership or schema CREATE privilege.
Effective table/column writes anywhere in `argus_catalog` are refused. Grant schema
USAGE and SELECT only on these five tables to the dedicated login:

~~~sql
GRANT USAGE ON SCHEMA argus_catalog TO evidence_reader;
GRANT SELECT ON argus_catalog.performance_request_intents,
  argus_catalog.performance_pending_dispatches,
  argus_catalog.performance_dispatch_outcomes,
  argus_catalog.performance_lifecycle_events,
  argus_catalog.performance_analysis_events TO evidence_reader;
~~~

The trusted operator must create/protect the login and deliver its connection
secret separately; do not put passwords in arguments, logs or checked-in files.
Memberships such as `pg_read_all_data` are intentionally rejected. Startup refuses
session-role impersonation and configures read-only sessions with bounded connects,
statements and one connection. This verifies the stated catalog privilege boundary;
it does not authenticate approval issuance or replace server authentication.

With both protected environment settings supplied:

~~~sh
go run ./cmd/performance-evidence \
  -issuer https://perfeng.example/identity \
  -principal-issuer https://auth.example \
  -principal-id argus -position 42
~~~

Authorization matches exact scope/position before opening the database. The command
has a 30-second overall deadline; the read-only repeatable-read snapshot has the
existing 15-second operation bound. It revalidates original admission, lifecycle
and analysis/review bytes and emits the [public projection](performance-decision-projection.md)
only after complete validation and snapshot commit. It starts no worker, provider
lookup, S3 read, migration or write. Existing expiry does not renew or invalidate
historical immutable evidence. No current catalog rebuild supplies missing proof.

## Failure, retention and recovery

Missing/unsafe policy and cross-scope/position requests are denied before database
access. Unknown requests, missing admission, corruption, failed revalidation, oversize output,
timeout and unavailable storage emit no document; known admitted empty analysis
history emits an empty list. Initial lifecycle evidence retained in admission is
preserved even before a separate lifecycle append. Conflicts and lateness retain
the public contract's meanings without an aggregate verdict or selected winner.

Stdout is not transactional: a failed/short destination write can leave partial
bytes. Use a private temporary destination and publish only after successful exit;
discard it on any failure. Do not publish the protected policy, retained raw bytes,
connection settings or driver diagnostics. Error messages expose fixed categories.
A retry performs another read-only snapshot and may see later retained events.
Correct host provisioning explicitly; the command never broadens grants or repairs
state. Rollback removes the command before its encoding/snapshot foundations,
without changing retained data. No public HTTP or automatic GitHub posting exists.
