# Task-scoped agent harness

This tool validates a task's declared execution scope and records verification results. It has two profiles:

- `audit`: declares no repository writes and no executable verification checks.
- `implementation`: declares explicitly listed repository-relative writable roots and named, non-mutating Make checks.

Both profiles *request* network denial and credential absence. The tool checks that the task policy matches a host-supplied scope declaration, rejects path traversal and symlink escapes, runs named checks without a shell, passes a limited environment to the child, and saves bounded stdout, stderr, exit status, timeout, and policy fingerprints in JSON under a scratch directory.

## Trust boundary

**The host must enforce the sandbox.** A Go process cannot provide cross-platform filesystem and socket isolation for arbitrary child code. The task and host-state JSON files should be supplied outside the checkout by the host, but their location and contents do not attest to enforcement: an agent able to write temporary files can forge them. The host must apply writable-root restrictions, network denial, credential isolation, scratch ownership, and child-process containment before it invokes this tool. Killing `make` on timeout does not guarantee its grandchildren are stopped. `make` executes repository code, so the limited environment alone is not a security boundary.

The task file must contain exactly one JSON object. For an audit task:

```json
{"version":1,"taskId":"catalog-audit","profile":"audit","repository":"/workspace/argus","writableRoots":[],"network":"deny","credentials":"none","checks":[]}
```

For an implementation task, choose the smallest applicable writable roots and checks:

```json
{"version":1,"taskId":"catalog-implementation","profile":"implementation","repository":"/workspace/argus","writableRoots":["internal/catalog","contracts"],"network":"deny","credentials":"none","checks":["verify","race"]}
```

The trusted host-state file uses absolute writable roots matching those paths and a scratch directory outside the repository:

```json
{"version":1,"repository":"/workspace/argus","writableRoots":["/workspace/argus/contracts","/workspace/argus/internal/catalog"],"scratchRoot":"/scratch/catalog-implementation","network":"deny","credentials":"none"}
```

Invoke from the repository root with paths supplied by the host:

Replace the illustrative absolute paths above with paths valid on the host.

```text
go run ./tools/agent-harness inspect --repo <repo> --task <task.json> --host-state <host.json>
go run ./tools/agent-harness verify --repo <repo> --task <task.json> --host-state <host.json> --check verify
```

`verify` returns 0 when the check passes, 1 when it fails or times out, and 2 when the harness refuses execution or cannot save its report. On failure it prints the last 32 KiB of each captured output stream so the agent can correct the error from the invocation; it also prints the report path. A failed or unavailable check is never reported as passed. Reports contain up to 4 MiB each of stdout and stderr, byte counts, truncation flags, the executable digest, and the exact task and host-state document digests. Treat reports and printed output as potentially sensitive. The allowlist contains only targets documented as offline after bootstrap. The runner resolves the existing Go module cache and passes it to the child; the host should make that cache read-only while keeping Go's build cache and temporary files in scratch. This runner is an inner-loop aid, not a substitute for required `make validate`, `make db-validate`, or other acceptance checks; run those in an appropriately authorized environment and report their outcomes separately.
