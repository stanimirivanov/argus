# Developer quickstart and local environment contract

## TL;DR

- Argus is verified on Ubuntu 24.04 x86-64 and Windows Server 2025 x86-64;
  Windows 11 x86-64 and Ubuntu 24.04 x86-64 are the supported local targets.
- Use the exact effective Go version declared by `go.mod`, GNU Make 4.3 or
  newer, a supported Git release, and either Bash or PowerShell 7.
- Run `make doctor` to report the effective toolchain, then run `make fmt` and
  `make validate` before review.
- The current scaffold needs no database, container runtime, cloud account,
  credentials, or external service. Initial tool and vulnerability checks need
  network access.
- Other operating systems, architectures, shells, and mixed Windows/WSL setups
  are best-effort until they have a CI lane; required checks are never weakened
  to accommodate an unverified environment.

## Environment support contract

Support describes where maintainers expect the complete checked-in workflow to
work and where failures block a change. It does not imply support for every
editor, terminal, package manager, or host customization.

| Tier | Environment | Contract |
|:--|:--|:--|
| Verified CI | Ubuntu 24.04 x86-64 with Bash; Windows Server 2025 x86-64 with PowerShell 7 | Every pull request MUST pass `make validate` on both. A failure blocks merge unless the check itself is deliberately changed and reviewed. |
| Supported local | Ubuntu 24.04 x86-64 with Bash; Windows 11 x86-64 with PowerShell 7 | Contributors SHOULD be able to run the complete workflow. Reproducible platform defects are project defects. |
| Best-effort | Other maintained Linux distributions, macOS, WSL2, ARM64, Git Bash, and other shells | The project accepts fixes that preserve the verified lanes, but does not promise platform-specific diagnosis or make these environments release gates. |
| Outside the contract | End-of-life operating systems or toolchains, 32-bit hosts, and environments that cannot execute the required checks | Contributors MAY use them for editing, but MUST verify through a supported environment or report checks as not run. |

The CI runner labels in
[the validation workflow](../../.github/workflows/validate.yml) are the
authority for verified operating systems. Adding a release-gating environment
requires a workflow change and corresponding update here. It does not require
an ADR unless it also changes a foundational technology or deployment boundary.

## Required tools and version authority

| Tool | Requirement | Authority and purpose |
|:--|:--|:--|
| Git | A vendor-supported release that honors `.gitattributes` | Clone, branch, diff, and preserve the repository's LF line-ending contract. |
| Go | The exact effective version in [`go.mod`](../../go.mod) | Build and test the control plane and run pinned Go-based quality tools. |
| GNU Make | 4.3 or newer; CI uses 4.4.1 on Windows | Provide the canonical command surface in the root [`Makefile`](../../Makefile). `nmake` is not a substitute. |
| Shell | Bash on Linux or PowerShell 7 on Windows | Run setup and Git commands. Make recipes intentionally avoid shell-specific syntax. |
| Network | Required for initial tool resolution and fresh vulnerability data | Download modules pinned by the Makefile and query the Go vulnerability database. |

Globally installed `golangci-lint`, `actionlint`, and `govulncheck` binaries are
neither required nor authoritative. The Makefile invokes explicit module
versions. IDE formatting, linting, and test integrations are optional feedback;
their success does not replace `make validate`.

`make doctor` does not alter repository files. It reports Git, Go, platform,
toolchain mode, CGO, and GNU Make information without printing repository
paths, proxy URLs, credentials, or the general process environment. It does not
replace acceptance checks. When `GOTOOLCHAIN=auto` and the required Go version
is absent, the Go command can perform its standard toolchain download.

## First-time setup

Clone with one Git implementation and keep that implementation for the working
tree:

~~~sh
git clone https://github.com/stanimirivanov/argus.git
cd argus
git status --short
~~~

The initial status MUST be empty. Report the effective toolchain:

~~~sh
make doctor
~~~

Confirm that:

- `go version` and `GOVERSION` match the exact version in `go.mod`;
- `GOOS` and `GOARCH` describe the intended supported environment;
- GNU Make is version 4.3 or newer; and
- Git is a maintained vendor release.

Then format and run the complete acceptance suite:

~~~sh
make fmt
git diff --check
make validate
~~~

The first run can be slower because Go resolves and compiles the pinned quality
tools. The build writes a platform-native executable under ignored `bin/`.
Start the current control-plane scaffold with:

~~~sh
go run ./cmd/control-plane
~~~

It emits structured lifecycle logs and waits for `Ctrl+C`. It intentionally
requires no configuration or external service at this stage.

## Platform setup notes

### Windows

Install the exact Go version from the official Go distribution and ensure it is
ahead of older Go installations on `PATH`. The CI-aligned GNU Make package is:

~~~powershell
choco install make --version=4.4.1 --yes
~~~

Package installation can require an elevated terminal and organizational
approval. Contributors MAY use another trusted installation source when
`make --version` reports GNU Make 4.3 or newer.

Use PowerShell 7 for the documented shell workflow. Repository text is checked
out with LF endings even when global Git configuration enables `autocrlf`.
Do not replace GNU Make with Visual Studio `nmake`.

### Ubuntu and other Linux distributions

Ubuntu provides Git and GNU Make through its package manager:

~~~sh
sudo apt-get update
sudo apt-get install --yes git make
~~~

Install the exact Go version declared by `go.mod` from the official Go
distribution or an equivalently trusted managed source. Other maintained Linux
distributions are best-effort until represented in CI.

### WSL2

Treat WSL2 as a Linux environment: install and invoke Git, Go, and GNU Make
inside the distribution and prefer a working tree on its Linux filesystem.
Avoid alternating Windows and WSL Git or Go tools against one working tree;
mixed path, permission, executable, and cache semantics are outside the support
contract.

## Daily development loop

Start from the issue and milestone required by
[CONTRIBUTING.md](../../CONTRIBUTING.md), preserve unrelated work, and keep one
pull-request-sized outcome. For an established environment, the normal loop is:

~~~sh
git status --short
make fmt
make check
make test
make validate
git diff --check
git status --short
~~~

`make check` and `make test` provide useful intermediate feedback.
`make validate` remains the required complete, non-mutating acceptance suite.
Run `make doctor` again after changing Go, Git, Make, the host OS, architecture,
shell, or CI image.

## Network, credentials, and services

Building and validating the current repository requires no credentials and no
local database, Kubernetes cluster, container runtime, object store, message
broker, or GitHub token. Do not add placeholder infrastructure merely to
anticipate roadmap work.

The first validation run needs access to Go module sources and checksum
services. Vulnerability checks need the Go vulnerability database unless it is
already cached. Corporate proxies and certificate authorities SHOULD be
configured through approved host or Go settings. Do not disable checksum,
certificate, lint, test, race, or vulnerability checks to bypass a network or
trust failure.

When a required check cannot run, follow the
[constrained-environment protocol](../../CONTRIBUTING.md#verification-and-constrained-environments):
report the exact command as not run, explain the blocker and residual risk, and
identify the supported environment where it should run.

## Troubleshooting

### Go reports the wrong version

Run `go version` and `go env GOTOOLCHAIN`. Install or select the exact version
in `go.mod`. Automatic Go toolchain resolution is acceptable when the effective
version is exact; when network policy blocks it, install that version explicitly.

### `make` is missing or is not GNU Make

Run `make --version`. Install GNU Make 4.3 or newer and ensure its executable is
first on `PATH`. Do not rename or silently substitute `nmake` or another build
program.

### Windows reports a formatting-only diff

Check the applicable Git attributes:

~~~sh
git check-attr text eol -- cmd/control-plane/main.go
~~~

The result MUST report `text: auto` and `eol: lf`. Preserve local changes before
repairing an older checkout. A fresh clone is the safest diagnostic comparison;
do not use a destructive restore merely to make formatting pass.

### Tool download or vulnerability lookup fails

Record the failing command and network error. Verify approved proxy,
certificate, DNS, and module-source configuration. Never report the check as
passed, switch to an unverified binary, or disable integrity verification.

### Race testing fails only on another architecture

Reproduce on a verified x86-64 environment. Other architectures remain
best-effort until a CI lane demonstrates the full suite. A platform expansion
must preserve race coverage rather than silently omit it.
