# Playwright functional API reference integration

## TL;DR

- `make api-validate` runs a disposable source repository, two independently
  installed Playwright test repositories, and a loopback fixture service.
- Trusted discovery checks every Playwright test against the repository-owned
  stable ID map before emitting a complete head inventory.
- A changed source file is mapped through the M04 analyzer and reviewed M05
  policy to a v4 manifest. The existing planner and runner now accept v4.
- Selected and full-suite stages produce immutable attempt JSON. A separate
  process reads those files and reports the deliberately missed failure and the
  group that ran only in the full-suite control.
- With `ARGUS_DATABASE_URL` set to a disposable PostgreSQL database, the gate
  also migrates, ingests every attempt, and compares the persisted report.

## Run the campaign

Install the root lockfile and run:

~~~sh
pnpm install --frozen-lockfile
make api-validate
~~~

The gate creates temporary Git repositories outside the checkout, installs each
test repository from its own lockfile in offline mode, starts a local HTTP
service on a random loopback port, builds the Go commands, and removes the
temporary repositories on success or failure. It never needs browser binaries:
Playwright's API request fixture supplies the test transport.

CI also runs this gate in the PostgreSQL integration job. For an equivalent
local persistence run, set `ARGUS_DATABASE_URL` to a **disposable** database
under your control before invoking the command. The gate runs migrations,
ingests the normalized attempt documents, and computes the complete report
again from the database. `make db-validate` remains the separate database
compatibility suite.

## Integrate a real test repository

The test checkout owns `argus-tests.json`, which maps each stable
`suiteKey`/`testKey` to one exact `[argus:suite:test]` Playwright tag and source
file. Keep this reviewed alongside the tests. The adapter runs from the test
checkout and resolves that checkout's own pinned Playwright installation.

Run discovery at an immutable checkout with trusted environment values:

~~~sh
ARGUS_SOURCE_REPOSITORY='{"provider":"github","host":"github.com","providerRepositoryId":"source-id"}' \
ARGUS_TEST_REPOSITORY='{"provider":"github","host":"github.com","providerRepositoryId":"tests-id","owner":"example","name":"api-tests"}' \
ARGUS_BASE_TEST_REVISION='<reviewed-base-sha>' \
node /path/to/argus/examples/functional-api/playwright-adapter.mjs discover \
  > head-test-inventory.json
~~~

The adapter compares every discovered test with the reviewed map, rejects
missing, duplicate, or extra tests and project variants, checks the current
Git SHA, and uses the base-to-head test-file diff to mark new or modified tests.
It emits `complete: true` only after all checks pass. Combine inventories from
all bound test repositories in trusted CI, then pass that combined document and
the reviewed owner policy to `select -policy-file ... -head-inventory-file ...`.
The source and test identities in this sample are placeholders; bind them to
your provider's stable repository IDs.

`plan-functional-api` and `run-functional-api` accept the resulting v4 manifest
without changing the command shape in the
[functional API adapter protocol](functional-api-adapters.md). Bind each group
to its immutable test revision and map the `playwright` adapter ID to the
literal, reviewed command `node /path/to/argus/examples/functional-api/playwright-adapter.mjs execute`.
The runner sends the exact requested test set on stdin. The adapter executes
each requested stable ID and returns one normalized result. It emits valid JSON
and exits zero for a test assertion failure; the Argus runner then writes the
attempt and returns a failed status. Malformed output or a revision mismatch
produces no valid attempt.

Discovery observes IDs and changed files. It never creates or changes the
source repository's authoritative capability mappings. A new test has no
capability mapping until an approved descriptor or evidence update provides it;
reviewed policy still makes it mandatory in the early stage. Full-suite jobs
remain the control for release decisions.

## Safety and current limits

- Use reviewed CI files and immutable revisions for policy, repository
  bindings, adapter commands, and the base test revision. Do not accept these
  controls from an untrusted pull-request head.
- The adapter supports one Playwright project variant per stable test. It fails
  discovery when variants duplicate a stable ID. A repository needing multiple
  projects needs an explicit identity extension before using this adapter.
- The reference executes one requested Playwright test per child process. This
  keeps result correlation exact, at the cost of startup overhead.
- Failures are normalized to bounded generic messages. Keep raw Playwright
  logs and artifacts under the CI repository's own access and retention policy.
- CI must isolate and clean descendant processes at the worker boundary after
  cancellation. The adapter forwards termination to its active child but is
  not an operating-system sandbox.
- The fixture campaign uses synthetic provider identities and local Git
  repositories. Replace these with actual reviewed repository coordinates and
  checkout steps for a production integration.
