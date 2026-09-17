import { Schema } from "effect";

const LocalKey = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(63),
  Schema.pattern(/^[a-z][a-z0-9._-]*$/),
).annotations({
  identifier: "LocalKey",
  description:
    "A stable, repository-declared key. It is scoped by its containing repository or parent declaration.",
});

const DisplayName = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255));

const RepositoryReference = Schema.Struct({
  provider: Schema.Literal("github", "gitlab", "azure-devops", "other"),
  host: Schema.String.pipe(
    Schema.minLength(1),
    Schema.maxLength(255),
    Schema.pattern(/^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/),
  ),
  providerRepositoryId: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255)),
  owner: DisplayName,
  name: DisplayName,
}).annotations({
  identifier: "RepositoryReference",
  description:
    "A repository reference anchored by the provider-assigned identity; owner and name are mutable display coordinates.",
});

const CapabilityDeclaration = Schema.Struct({
  key: LocalKey,
  name: DisplayName,
}).annotations({ identifier: "CapabilityDeclaration" });

const ComponentDeclaration = Schema.Struct({
  key: LocalKey,
  root: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(1024)),
  capabilities: Schema.Array(LocalKey).pipe(Schema.minItems(1)),
}).annotations({ identifier: "ComponentDeclaration" });

const TestDeclaration = Schema.Struct({
  key: LocalKey,
  name: DisplayName,
  capabilities: Schema.Array(LocalKey).pipe(Schema.minItems(1)),
}).annotations({ identifier: "TestDeclaration" });

const TestSuiteDeclaration = Schema.Struct({
  key: LocalKey,
  repository: RepositoryReference,
  family: Schema.Literal(
    "unit",
    "component",
    "contract",
    "integration",
    "functional-api",
    "functional-ui",
    "end-to-end",
    "performance",
    "security",
    "resilience",
    "other",
  ),
  adapter: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
  tests: Schema.Array(TestDeclaration).pipe(Schema.minItems(1)),
}).annotations({ identifier: "TestSuiteDeclaration" });

/**
 * RepositoryDescriptorV1 is the repository-authored catalog input. The
 * immutable source revision is supplied by the verified ingestion context and
 * deliberately does not live in the repository-controlled document.
 */
export const RepositoryDescriptorV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/repository-descriptor/v1"),
  repository: RepositoryReference,
  capabilities: Schema.Array(CapabilityDeclaration).pipe(Schema.minItems(1)),
  components: Schema.Array(ComponentDeclaration).pipe(Schema.minItems(1)),
  testSuites: Schema.Array(TestSuiteDeclaration),
}).annotations({
  identifier: "RepositoryDescriptorV1",
  title: "Argus repository descriptor v1",
  description:
    "Declares repository-scoped capabilities, components, test suites, stable tests, and explicit mappings.",
});

export type RepositoryDescriptorV1 = typeof RepositoryDescriptorV1.Type;
