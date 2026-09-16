# Argus

Argus is an adaptive test intelligence and evolution platform. It determines
which tests should run for a software change, identifies stale or missing test
coverage, and produces evidence-backed maintenance proposals.

The repository is at engineering-foundation stage; executable components have
not been scaffolded yet.

## Start here

- [Product definition](docs/product/product-definition.md)
- [Architecture overview](docs/architecture/overview.md)
- [Proposal decomposition and provenance](docs/proposal.md)
- [Contributor workflow](CONTRIBUTING.md)
- [Engineering standards](docs/development/engineering-standards.md)
- [SQL migration criteria](docs/development/sql-migrations.md)
- [Architecture decisions](docs/decisions/README.md)
- [Implementation milestones](docs/roadmap/milestones.md)
- [Research and market landscape](docs/research/landscape.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)

Exact build, test, and local environment commands will be added with the first
control-plane scaffold. Until then, `make fmt`, `make check`, and `make test`
are fail-closed placeholders: they explain that executable verification is not
configured and exit non-zero. Do not report that result as a passing check.
