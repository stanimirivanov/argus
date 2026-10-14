# CI policy check

This offline detector compares `.github/workflows/validate.yml` with the
reviewed job mapping in [`policy.json`](policy.json). It checks the unfiltered
`pull_request` trigger, exact job IDs and display names, runner and OS matrix
values, and each job's required `make` command. Required jobs and their command
steps cannot use `if` or `continue-on-error`; required jobs also cannot use
`needs`, which could skip them with an upstream job. The check rejects Make
control variables at workflow, job, and required-step scope, workflow or job
`defaults.run`, and shell or directory overrides on required Make steps. Matrix
`include`, `exclude`, and other axes are rejected.

From the repository root:

~~~sh
go run ./tools/ci-policy-check
go test -count=1 ./tools/ci-policy-check
~~~

Change `policy.json` and the workflow together when CI is intentionally
restructured. This check detects accidental drift in checked-in files; it does
not inspect live GitHub settings or prove that a run is a merge gate. Review
its policy and implementation during manual pre-merge verification.
