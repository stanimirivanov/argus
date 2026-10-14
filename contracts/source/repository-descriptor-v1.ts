import { Schema } from "effect";

export const LocalKey = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(63),
  Schema.isPattern(/^[a-z][a-z0-9._-]*$/u),
).annotate({
  identifier: "LocalKey",
  description:
    "A stable, repository-declared key. It is scoped by its containing repository or parent declaration.",
});

export const DisplayName = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255));

export const RepositoryIdentity = Schema.Struct({
  provider: Schema.Literals(["github", "gitlab", "azure-devops", "other"]),
  host: Schema.String.check(
    Schema.isMinLength(1),
    Schema.isMaxLength(255),
    Schema.isPattern(/^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/u),
  ),
  providerRepositoryId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
}).annotate({
  identifier: "RepositoryIdentity",
  description: "A repository identity that remains stable across ordinary renames and transfers.",
});

export const RepositoryReference = Schema.Struct({
  ...RepositoryIdentity.fields,
  owner: DisplayName,
  name: DisplayName,
}).annotate({
  identifier: "RepositoryReference",
  description:
    "A repository reference anchored by the provider-assigned identity; owner and name are mutable display coordinates.",
});

const CapabilityDeclaration = Schema.Struct({
  key: LocalKey,
  name: DisplayName,
}).annotate({ identifier: "CapabilityDeclaration" });

const ComponentDeclaration = Schema.Struct({
  key: LocalKey,
  root: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1)),
}).annotate({ identifier: "ComponentDeclaration" });

const TestDeclaration = Schema.Struct({
  key: LocalKey,
  name: DisplayName,
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1)),
}).annotate({ identifier: "TestDeclaration" });

export const TestFamily = Schema.Literals([
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
]).annotate({ identifier: "TestFamily" });

const TestSuiteDeclaration = Schema.Struct({
  key: LocalKey,
  repository: RepositoryReference,
  family: TestFamily,
  adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  tests: Schema.Array(TestDeclaration).check(Schema.isMinLength(1)),
}).annotate({ identifier: "TestSuiteDeclaration" });

/**
 * RepositoryDescriptorV1 is the repository-authored catalog input. The
 * immutable source revision is supplied by the verified ingestion context and
 * deliberately does not live in the repository-controlled document.
 */
export const RepositoryDescriptorV1APIVersion = Schema.Literals([
  "argus.dev/repository-descriptor/v1",
]);

export const RepositoryDescriptorV1 = Schema.Struct({
  apiVersion: RepositoryDescriptorV1APIVersion,
  repository: RepositoryReference,
  capabilities: Schema.Array(CapabilityDeclaration).check(Schema.isMinLength(1)),
  components: Schema.Array(ComponentDeclaration).check(Schema.isMinLength(1)),
  testSuites: Schema.Array(TestSuiteDeclaration),
}).annotate({
  identifier: "RepositoryDescriptorV1",
  title: "Argus repository descriptor v1",
  description:
    "Declares repository-scoped capabilities, components, test suites, stable tests, and explicit mappings.",
});

export type RepositoryDescriptorV1 = typeof RepositoryDescriptorV1.Type;
