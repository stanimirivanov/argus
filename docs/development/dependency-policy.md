# Dependency, license, and vulnerability policy

## TL;DR

- Application, test, build-tool, and GitHub Actions dependencies MUST be pinned,
  necessary, maintained, license-compatible, and reviewed before admission.
- Go tools use native `tool` directives and checksum files; incompatible build
  graphs use a checked-in isolated tool module. GitHub Actions use major
  semantic release tags. Ambient global binaries are not authoritative.
- Dependabot proposes grouped non-major and security updates for Go modules,
  npm/pnpm dependencies, and GitHub Actions. Updates are reviewed and validated;
  they are never implicitly trusted or auto-merged by repository policy.
- `make license` permits a narrow runtime license allowlist and separately checks
  development tools with named, non-distributed exceptions.
- `make vuln` checks reachable Go vulnerabilities and audits production pnpm
  dependencies without requiring an optional repository-hosted dependency
  graph.
- Novel or exploitable findings follow [SECURITY.md](../../SECURITY.md), not a
  public issue. Exceptions require an owner, rationale, scope, expiry, and
  compensating controls.

## Scope and sources of truth

This policy governs dependencies used by application code, tests, generators,
quality tools, GitHub Actions, and future packaging. It applies to direct and
transitive dependencies. It does not make third-party source part of Argus's
supported public API.

| Concern | Authority |
|:--|:--|
| Go version and application module | Root [`go.mod`](../../go.mod) and [`go.sum`](../../go.sum) |
| Node.js version and dependency graph | [`.node-version`](../../.node-version), [`package.json`](../../package.json), and [`pnpm-lock.yaml`](../../pnpm-lock.yaml) |
| Isolated tool graphs | [`tools`](../../tools) module and checksum files |
| Executable local checks and license exceptions | Root [`Makefile`](../../Makefile) |
| GitHub Actions versions | Major semantic `uses` tags in [workflow files](../../.github/workflows) |
| Automated update grouping and cadence | [`.github/dependabot.yml`](../../.github/dependabot.yml) |
| Private vulnerability reporting | [`SECURITY.md`](../../SECURITY.md) |
| Review ownership | [`.github/CODEOWNERS`](../../.github/CODEOWNERS) |

When metadata, a scanner, and the upstream license disagree, the dependency is
not silently admitted. A maintainer MUST inspect authoritative upstream files,
record the conclusion, and either correct the classifier input, add a narrow
exception, or reject the dependency.

## Admission criteria

A new dependency MUST have a current product or engineering need that is not
reasonably met by the standard library or existing dependencies. Its pull
request MUST document:

- the behavior it enables and why a smaller alternative is insufficient;
- upstream repository, selected release, maintenance activity, and replacement
  boundary;
- runtime, test-only, build-time, or CI scope;
- direct and material transitive license obligations;
- known advisories, reachability, and security posture;
- data, network, process-execution, credential, and supply-chain access; and
- removal, rollback, or substitution consequences.

Application and domain packages MUST NOT import a development tool. Tool
dependencies MUST be declared with Go's `tool` directive rather than installed
implicitly or resolved with `@latest`. GitHub Actions MUST reference only the
major semantic tag of a reviewed action, such as `actions/checkout@v7`.

The catalog PostgreSQL adapter admits `github.com/jackc/pgx/v5` v5.11.0 as a
runtime dependency. It provides the native protocol, bounded connection pool,
transactions, bulk copy, and PostgreSQL error typing needed by the M03 vertical
slice; reproducing these correctly with the standard library would require a
driver and protocol implementation. The module is actively maintained,
requires no process execution, receives a database URL supplied by the
operator, and is isolated behind `internal/catalog/adapters/postgres`. Its upstream MIT
license and the compatible licenses of reachable runtime dependencies fit the
runtime allowlist. Removal means replacing this adapter and its connection,
transaction, copy, and error-classification behavior behind the catalog domain
boundary.

The OpenAPI impact adapter admits `github.com/pb33f/libopenapi` v0.38.7 as a
runtime dependency. It supplies OpenAPI 3.0/3.1/3.2 parsing and semantic
document comparison; reproducing that standards surface locally would create a
larger and less reviewable parser. The selected release is active and MIT
licensed, and its reachable runtime dependencies use allowed permissive
licenses. Argus disables remote and filesystem reference lookup, passes only
bounded repository bytes, and does not expose credentials or process execution
to the library. The import is isolated under
`internal/change/adapters/openapi`; removal means replacing that adapter while
preserving analyzer conformance and persisted analyzer-version semantics.

Tools SHOULD share the root tool graph when their selected versions compile
together. A tool MUST move to a dedicated module when minimal-version selection
would otherwise make one pinned tool fail to build. The isolated module remains
owned, checksummed, updated, vulnerability-reviewed, and license-checked; it is
not an escape from dependency policy. actionlint currently uses this boundary
because its pinned prerelease YAML parser API is incompatible with the version
selected by the main quality-tool graph.

Local replacements, forks, pseudo-versions, pre-releases, abandoned projects,
and dependencies that execute untrusted repository content require explicit
rationale and focused review. A `replace` directive pointing outside the
repository MUST NOT be committed.

## Update automation and review

Dependabot checks Go modules, npm/pnpm dependencies, and GitHub Actions weekly.
Minor and patch version updates are grouped per ecosystem to reduce review
overhead. Major updates stay separate because they can change compatibility or
policy. Security updates are grouped separately and take priority over routine
version updates.

Every update pull request MUST:

1. retain exact Go and Node dependency versions, checksum/lock files, and major
   semantic action tags;
2. review release notes and relevant upstream security or compatibility notes;
3. explain material transitive, license, configuration, or generated changes;
4. run `make fmt` and `make validate` on the resulting graph;
5. avoid unrelated source or broad dependency churn.

Automated pull requests MUST NOT be auto-merged solely because checks are green.
A green update can still introduce changed semantics, excessive privilege,
abandoned ownership, unacceptable licensing, or a compromised release. A
failed grouped update SHOULD be narrowed only enough to identify and review the
incompatible dependency; the remaining safe updates MAY proceed together.

Dependabot version updates are enabled by the checked-in configuration.
Dependabot alerts and security updates depend on repository settings and plan
support. Evaluation and ownership of repository-hosted dependency graph and
advisory integrations are deferred to M10 production readiness; the current CI
MUST remain usable without them.

## License policy

Argus itself is licensed under Apache-2.0. Application and test dependency
packages are accepted automatically only when `go-licenses` identifies one of:

- Apache-2.0;
- BSD-2-Clause;
- BSD-3-Clause;
- ISC; or
- MIT.

`make license` includes test-only packages. A license outside this list is not
automatically forbidden, but it requires legal/owner review and a package-
specific exception before admission. Unknown licensing, missing required
notices, non-commercial clauses, source-available terms, and unreviewed strong
copyleft are rejected by default for code linked into or distributed with
Argus.

Development tools are executed during repository validation and are not linked
into or distributed with Argus. Their graph is checked separately. The checker
accepts the runtime allowlist plus MPL-2.0 for this non-distributed scope. It
rejects every other or unknown classification except these reviewed package
prefixes:

| Package prefix | Classification | Reason for narrow exception |
|:--|:--|:--|
| `github.com/golangci/golangci-lint/v2` | GPL-3.0 | Standalone development executable; never linked into or shipped with Argus. |
| `github.com/OpenPeeDeeP/depguard/v2`, `github.com/denis-tingaikin/go-header`, `github.com/firefart/nonamedreturns`, `github.com/ldez/structtags`, `github.com/leonklingele/grouper`, `github.com/xen0n/gosmopolitan` | GPL-3.0 | Linter implementations reachable only inside the standalone golangci-lint tool. |
| `github.com/alecthomas/chroma/v2` | OFL-1.1 classified as unknown | Development-tool rendering dependency; no font or package is distributed by Argus. |
| `github.com/ashanbrown/forbidigo/v2`, `github.com/ashanbrown/makezero/v2` | Classifier reports unknown; module license is Apache-2.0 | Upstream module archives contain Apache-2.0 license files, but package-level discovery misses them. |
| `github.com/golangci/gofmt` | Unknown | Development-only Go formatter fork; the selected module archive lacks classifier-visible license metadata and is not distributed. |

An exception MUST be present in both this table and `TOOL_LICENSE_EXCEPTIONS` in
the Makefile. It MUST remain restricted to development tooling. Moving an
excepted package into application or test code requires fresh review under the
runtime allowlist; the tool exception does not follow it.

The Node production graph currently contains Effect and its MIT-licensed
transitive packages. `make license` inventories the production graph through
pnpm and fails when a license group is outside the runtime allowlist. Build and
test packages are development tools and are not distributed with the Go
commands; lock-changing pull requests MUST still review their direct and
material transitive licenses. Release SBOM and notice generation remain M10
packaging work.

Effect, TypeScript, Node.js, pnpm, Biome, and tsx versions are exact. Effect v4
release candidates are not admitted while the contract source targets stable
Effect v3. Updates MUST regenerate JSON Schema, run the shared fixture corpus,
and inspect generated diffs rather than assuming type-check success proves wire
compatibility.

Before publishing a distributable release, the release process MUST generate
and verify the notices, source offers, license bundle, or SBOM required by the
actual artifact graph. That packaging work belongs to production readiness; the
current policy prevents incompatible dependencies from entering unnoticed.

## Vulnerability policy

`make vuln` uses the pinned `govulncheck` tool for vulnerabilities reachable
from Argus packages and the isolated actionlint graph, and `pnpm audit --prod`
for high-severity or critical advisories in the production Node graph. It is
the current executable vulnerability gate and does not depend on optional
GitHub dependency-graph features. Repository-hosted advisory review may be
evaluated as part of M10 production readiness.

A maintainer reviewing an alert MUST establish the affected version, scope,
reachability, exploit preconditions, available fix, and operational exposure.
Reachable critical or high findings block release and receive immediate
remediation priority. Lower-severity or unreachable findings still require a
recorded disposition; scanner output alone is not proof of safety.

An advisory suppression MUST be specific to one advisory and dependency. It
MUST record the evidence, owner, creation date, expiry/review date, compensating
controls, and removal condition. Broad severity downgrades, wildcard package
exclusions, and permanent undocumented suppressions are prohibited.

If a finding is novel, not yet public, exploitable, or contains sensitive
reproduction details, use the private process in `SECURITY.md`. Routine updates
for already-public advisories MAY use normal dependency pull requests as long
as they do not disclose additional exploit information.

## Ownership and periodic review

CODEOWNERS identifies the current reviewer for manifests, workflows, security
policy, and development standards. Ownership MUST be updated before it becomes
stale; an absent reviewer is not permission to bypass dependency governance.

At least when beginning a milestone or preparing a release, maintainers SHOULD
review:

- stale Dependabot pull requests and whether automation is active;
- `go list -m -u all` output and unsupported transitive versions;
- vulnerability and secret-scanning alerts;
- license exceptions and their continued development-only scope;
- inactive, archived, transferred, or unexpectedly republished upstreams; and
- action permissions and selected major semantic tags.

This review is evidence for dependency health, not a mandate to upgrade every
package. Stability, compatibility, and security determine update priority.
