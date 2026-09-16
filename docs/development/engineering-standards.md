# Engineering standards

## TL;DR

- Apply architecture and SOLID principles to preserve boundaries, not to
  maximize interfaces or patterns.
- Model domain meaning explicitly and keep infrastructure at adapters.
- Prefer small coherent APIs, composition, immutability, typed failures, and
  bounded resource use.
- Use each language idiomatically: Go is not class-oriented Python, Python is
  not dynamically typed TypeScript, and TypeScript types do not validate input
  at runtime.
- Test behavior and important failures deterministically.
- Document contracts, invariants, ownership, units, side effects, and failure
  semantics; avoid comments that restate code.
- Pin tools and dependencies in repository configuration and run the same checks
  locally and in CI.

## Policy strength

Normative terms use the meanings defined in
[CONTRIBUTING.md](../../CONTRIBUTING.md#policy-language-and-sources-of-truth).
MUST and MUST NOT are reviewable requirements; SHOULD and SHOULD NOT are strong
defaults that require a recorded reason to deviate; MAY is optional. Other
imperative or advisory wording explains recommended design practice and becomes
mandatory only when an issue, ADR, or language-specific checked-in tool makes it
an explicit requirement.

## Purpose and applicability

These standards apply to production code, tests, scripts, generated bindings,
and operational tooling. They intentionally avoid project-specific domain
rules and exact tool versions. A repository MAY add stricter local rules but
MUST NOT silently weaken an explicit requirement.

The goal is maintainable correctness, not ceremonial compliance. A deviation
from a strong default MAY be accepted when its trade-off is explicit and tested;
it MUST be recorded in an ADR when it affects more than a local implementation.

## Architecture

### Capability-oriented boundaries

Organize code around capabilities and ownership rather than technical folders
alone. Introduce domain, application, and adapter boundaries where they clarify
different responsibilities:

- Domain code owns business vocabulary, invariants, state transitions, and
  policies. It does not import HTTP, SQL, message brokers, cloud SDKs, UI
  frameworks, or model-provider types.
- Application code coordinates use cases, transactions, authorization, and
  ports. It does not parse vendor wire formats or issue vendor-specific calls.
- Inbound adapters validate protocol shape and translate requests or events
  into application commands.
- Outbound adapters implement application-owned ports and translate domain
  values into persistence, transport, or provider representations.

Transport DTOs, event envelopes, database records, model output schemas, and
domain objects are distinct when they have different lifecycles or invariants.
Do not make one annotated type serve every boundary for convenience.

A module owns its writes. Cross-module access uses a public application
capability or versioned event, not another module's tables or private types.

### SOLID without ceremony

- Single responsibility means one cohesive reason to change at the relevant
  scale. It does not require one type per file or tiny functions without
  meaning.
- Open/closed applies to stable variation points. Do not invent extension
  mechanisms before a real second behavior demonstrates the variation.
- Liskov substitution requires implementations to honor the same success,
  absence, failure, ordering, concurrency, and ownership contract.
- Interface segregation favors small consumer-owned interfaces. A caller should
  not depend on capabilities it cannot use.
- Dependency inversion places policy above details. Use dependency injection
  and ports where substitution, testing, or ownership requires them; do not
  wrap stable standard-library functions reflexively.

Prefer composition over inheritance. Select patterns by the problem:

- value objects for validated domain meaning;
- explicit state machines for constrained transitions;
- strategy for a demonstrated family of interchangeable policies;
- repository/port for a domain-oriented persistence capability;
- adapter for an external protocol or provider;
- decorator/middleware for orthogonal behavior with preserved semantics; and
- factory only when construction has meaningful policy or invariants.

Avoid service locator, global mutable state, boolean-driven state machines,
generic repository frameworks, and classes named Manager, Helper, Util,
Processor, Base, or Impl when a precise capability name exists.

### Data and time

- Use explicit types for identifiers, revisions, hashes, risk, confidence,
  durations, timestamps, byte sizes, and other meaningful primitives.
- Make units visible in types, field names, or API contracts.
- Store instants in UTC and preserve their source. Distinguish event time,
  ingestion time, decision time, and persistence time.
- Inject clocks, ID generators, randomness, filesystem roots, and external
  clients when determinism or policy depends on them.
- Prefer immutable values. Confine mutation to clearly owned aggregates,
  transactions, caches, and adapter internals.
- Model closed outcomes with enums, tagged/discriminated unions, or sealed
  alternatives rather than combinations of booleans.

### Errors and resource ownership

- Distinguish expected domain outcomes, invalid input, conflicts, unavailable
  dependencies, cancellation, timeouts, and internal defects.
- Do not expose driver, framework, vendor, or secret-bearing error text across
  a public boundary.
- Preserve causal information when wrapping errors.
- Acquire resources with explicit ownership and release them on every path.
- Bound input size, output size, collection size, concurrency, retries,
  execution time, and memory-intensive operations.
- Keep database transactions short and free of remote calls.
- Define idempotency using stable identities plus immutable-content comparison;
  key equality alone may not prove an exact retry.

### Concurrency and asynchronous work

- Prefer synchronous code until concurrency provides a measured or required
  benefit.
- Use structured concurrency: child work belongs to a request, job, or service
  lifecycle and is cancelled with it.
- Never start unbounded goroutines, tasks, promises, workers, or retries.
- Document lock ordering, ownership transfer, cancellation priority, retry
  safety, and delivery guarantees.
- Do not use sleeps to coordinate tests. Use barriers, channels, events,
  controllable clocks, or observable state.
- Background work defines deduplication, retry classification, terminal
  failure, dead-letter or recovery behavior, and operator visibility.

## Go

- Use the Go version declared by go.mod and pin non-standard quality tools.
- gofmt is authoritative. Use goimports when adopted by repository tooling.
- Follow standard Go naming and package guidance. Package names are short,
  lower-case, meaningful, and avoid api, common, misc, types, and util.
- Keep packages cohesive with a small public surface. Avoid packages that
  merely mirror architecture layer names across unrelated capabilities.
- Define interfaces near consumers. Accept interfaces and return concrete types
  unless a public abstraction genuinely requires otherwise.
- Prefer concrete values and ordinary functions. Use generics when one
  type-safe algorithm or data structure truly applies across types, not to
  recreate inheritance or generic repositories.
- Make zero values useful when doing so preserves invariants. Otherwise provide
  a validating constructor and keep invalid representations private.
- Pass context.Context as the first parameter for request-scoped cancellation
  and deadlines. Do not store it in structs or use it as an optional bag of
  arguments.
- Return errors; reserve panic for unrecoverable programmer invariants and
  process startup failures where continuation is impossible.
- Classify public errors so callers can use errors.Is or errors.As. Wrap with
  operational context without leaking secrets or changing classification.
- Close and check resources such as response bodies, files, SQL rows, and
  transaction commits. A deferred rollback must not replace the primary error.
- Keep goroutine ownership visible. Use channels to communicate and mutexes to
  protect owned state; neither is universally preferable.
- Document every package and exported declaration with complete sentences that
  explain purpose and contract. Avoid exporting a name only to simplify tests.
- Use table-driven tests where cases share behavior, helpers marked with
  t.Helper, race tests for concurrent code, and native fuzzing for parsers and
  untrusted boundaries.
- Run formatting, go vet/static analysis, module verification, tests, the race
  detector where supported, and govulncheck in CI.

Primary references:
[Go code review comments](https://go.dev/wiki/CodeReviewComments),
[Go documentation comments](https://go.dev/doc/comment), and
[Go security best practices](https://go.dev/doc/security/best-practices).

## Python

- Declare supported Python versions and project/tool configuration in
  pyproject.toml. Use a lock file for applications and reproducible tools.
- Format and lint with repository-pinned tools. Do not hand-format against the
  formatter.
- Type public and application-layer functions. Keep the configured type checker
  strict enough that untyped values do not silently spread through the core.
- Treat Any as an explicit escape hatch. Normalize untyped libraries and parsed
  data at adapters.
- Use dataclasses or small classes for values with behavior; use frozen values
  where mutation is not part of the model.
- Use Protocol for structural consumer contracts when multiple implementations
  or a meaningful test seam exist. Do not add abstract base classes by default.
- Raise precise exceptions for exceptional failures and return explicit domain
  outcomes when callers are expected to branch. Never catch Exception merely to
  discard or relabel every failure.
- Keep import-time behavior side-effect-free. Configuration is loaded and
  validated at composition/startup boundaries.
- Prefer pathlib, context managers, iterators, comprehensions that remain
  readable, and standard-library facilities before dependencies.
- Use async only for genuinely asynchronous I/O. Keep blocking work out of the
  event loop and retain task ownership through task groups or equivalent
  structured lifecycles.
- Docstrings describe public modules, classes, functions, contracts, errors,
  units, and non-obvious side effects. They do not repeat the signature.
- Tests use temporary resources, controlled clocks, and explicit fakes.
  Monkey-patching is limited to true external boundaries.
- CI runs formatting checks, linting, type checking, tests with coverage used as
  diagnostic evidence rather than a target, dependency auditing, and packaging
  validation when the component is distributable.

Primary references:
[Python typing](https://docs.python.org/3/library/typing.html) and
[pyproject.toml guidance](https://packaging.python.org/en/latest/guides/writing-pyproject-toml/).

## TypeScript

- Enable strict in tsconfig.json. Consider noUncheckedIndexedAccess,
  exactOptionalPropertyTypes, useUnknownInCatchVariables, and
  noImplicitOverride according to runtime/library compatibility.
- Pin Node.js, the package manager, TypeScript, formatter, linter, and lock file.
  CI uses frozen/immutable installation.
- Treat external input as unknown. Static types disappear at runtime; validate
  HTTP, event, storage, environment, and model data with an explicit schema at
  the adapter.
- Avoid any. A necessary use is isolated, explained, and converted to a safe
  type immediately.
- Prefer readonly data, discriminated unions, exhaustive switches checked with
  never, and domain-specific types over broad object/index-signature shapes.
- Distinguish absent, null, and undefined deliberately. Do not make fields
  optional merely because construction is inconvenient.
- Prefer functions and composition. Use classes for identity, lifecycle, or
  encapsulated state, not as the default module structure.
- Keep promises owned and awaited. Handle rejection, cancellation/AbortSignal,
  timeouts, and concurrency bounds explicitly; do not leave floating promises.
- Use Error subclasses or tagged results with stable classifications. Preserve
  causes and do not throw strings.
- Select module and moduleResolution settings for the actual runtime. Do not
  mix ESM and CommonJS assumptions accidentally.
- Export the smallest supported API. Avoid barrel files that create cycles or
  make ownership unclear.
- TSDoc/JSDoc explains public contracts and non-obvious behavior. Examples are
  compiled or tested when practical.
- CI runs formatting, linting, tsc without emission, tests, package/build
  checks, and dependency/security review.

Primary references:
[TypeScript strict mode](https://www.typescriptlang.org/tsconfig/strict) and
[module compiler options](https://www.typescriptlang.org/docs/handbook/modules/guides/choosing-compiler-options).

## Contracts and compatibility

- Treat HTTP schemas, events, CLI behavior, configuration, database migrations,
  generated bindings, persisted payloads, test manifests, and model/tool
  schemas as compatibility boundaries.
- Version semantics, not filenames alone. Never change a published field's
  meaning in place.
- Additive changes define defaults and old-reader behavior. Breaking changes
  require a new version or an expand/migrate/contract plan, contract tests, and
  an ADR.
- Commands express requested intent. Events describe completed facts in past
  tense.
- Every execution or decision that must be explainable pins exact source,
  contract, policy, model, adapter, environment, and evidence revisions.
- Generated code is reproducible. Check it in only when consumers cannot
  reasonably run the generator, and verify that regeneration produces no diff.

## Testing

- Test a behavior at the lowest boundary that provides convincing evidence:
  pure domain tests, application tests with fakes, adapter integration tests
  against real protocols, contract tests, and a small number of end-to-end
  tests.
- Test names state the condition and observable outcome.
- Every defect fix includes a test that fails before the fix.
- Tests cover important success, rejection, duplicate, stale revision,
  authorization, timeout, retry, cancellation, partial failure, and recovery
  paths as applicable.
- Use a real disposable PostgreSQL or protocol service when a mock cannot
  reproduce constraints, transactions, locking, encoding, or driver behavior.
- Avoid paid or externally mutable services in default CI. Gate explicit live
  tests separately.
- Never weaken assertions, add blind retries, or lengthen timeouts without
  diagnosing the nondeterminism.
- Coverage identifies unexamined code but does not prove correctness. Mutation
  or fault-seeding tests are valuable where preservation of an oracle matters.

## Documentation

- Public APIs document purpose, invariants, inputs, outputs, units, ownership,
  side effects, concurrency, security expectations, and failures that callers
  can handle.
- Write comments for decisions and constraints, including protocol rules,
  transaction/isolation choices, compatibility workarounds, and deliberate
  performance trade-offs.
- Prefer clear names and types over explanatory comments. Delete commented-out
  code and stale documentation.
- Keep tutorials and operational procedures in documentation rather than large
  source comments.
- Long documentation contains a TL;DR under the policy in CONTRIBUTING.md.

## Security, privacy, and operations

- Deny by default and use least privilege. Authentication establishes identity;
  authorization is enforced at the use case and protected data boundary.
- Secrets come from an approved secret mechanism, never committed defaults.
- Never log credentials, authorization headers, sensitive payloads, personal
  data without need, or raw model prompts containing secrets.
- Model output, retrieved content, repository content, webhooks, and tool
  descriptions are untrusted. Parse them into typed proposals and validate them
  independently.
- A model cannot grant capability, approve its own action, provide credentials,
  or declare its own result verified.
- Expose structured logs, traces, and bounded-cardinality metrics sufficient to
  diagnose behavior. Correlation IDs are not authorization grants.
- Dependencies require a current need, compatible license, maintained release,
  proportionate security review, pinned resolution, and a replacement path at
  a boundary.
- New operational behavior documents healthy signals, failure classes,
  recovery, rollout, and rollback.
