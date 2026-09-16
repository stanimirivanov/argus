# Security policy

## TL;DR

- Do not disclose suspected vulnerabilities in public issues, discussions, or
  pull requests.
- Use GitHub private vulnerability reporting when the repository exposes it.
- If no private channel is available, request one without including sensitive
  details.
- Argus is pre-release; the current default branch is the only supported line.
- Reports should contain enough reproducible evidence for safe triage.
- Public dependency advisories are handled through normal reviewed updates;
  novel or sensitive exploit information always uses the private channel.

## Supported versions

Argus is in engineering foundation and has no supported production release.
Security fixes target the current default branch. When releases begin, this
section MUST be replaced with an explicit supported-version table and disclosure
policy.

## Dependency vulnerabilities

The [dependency policy](docs/development/dependency-policy.md) defines admission,
update, license, automated review, suppression, and ownership requirements.
Routine remediation of an already-public advisory MAY use a normal dependency
pull request. Do not include new exploit techniques, sensitive reproduction
details, credentials, or affected private environments in that pull request.

A novel, embargoed, or plausibly exploitable finding MUST be reported through
the private process in [Reporting a vulnerability](#reporting-a-vulnerability)
even when it originates in a third-party dependency.
Maintainers MUST assess the selected and fixed versions, reachability, exposure,
available mitigations, and whether coordinated disclosure is required. Scanner
severity alone does not replace that assessment.

Repository-hosted dependency graph and advisory integrations are deferred to
M10 production readiness, where their plan support and operational ownership
can be assessed deliberately. Private vulnerability reporting and available
secret scanning remain recommended repository settings.

## Reporting a vulnerability

Use the repository Security page's **Report a vulnerability** action when it is
available. This is the preferred private channel.

If private vulnerability reporting is unavailable, do not publish the report.
Create a minimal issue asking the repository owner to establish a private
security contact, without naming the vulnerable component, exploit, affected
data, or reproduction details. You MAY also use a private contact method listed
on the repository owner's GitHub profile.

Include, where applicable:

- the affected revision, component, and configuration;
- impact and the conditions required to reproduce it;
- minimal reproduction steps or a proof of concept;
- relevant logs with credentials, tokens, personal data, and customer data
  removed;
- known mitigations; and
- whether and where the issue has already been disclosed.

Do not test against systems, repositories, accounts, or data you do not own or
have explicit permission to assess. Do not retain, alter, or disclose accessed
data beyond what is necessary to demonstrate the issue safely.

## Triage and disclosure

Maintainers SHOULD acknowledge a private report, validate severity and scope,
and agree on a communication path before public disclosure. Response times are
best-effort until a staffed security process is established.

Reporter and maintainers SHOULD coordinate disclosure after a fix or effective
mitigation is available. Maintainers MAY publish a GitHub security advisory
that credits the reporter, unless the reporter asks not to be named. Reports
made in good faith under this policy SHOULD be handled respectfully.

## Security expectations for contributions

Contributors MUST follow the trust-boundary, secret-handling, dependency, and
verification requirements in [AGENTS.md](AGENTS.md),
[CONTRIBUTING.md](CONTRIBUTING.md), and the
[engineering standards](docs/development/engineering-standards.md). A change
that affects authentication, authorization, sensitive data, execution of
untrusted content, or external credentials MUST document its threat and failure
model and MUST receive focused security review.
