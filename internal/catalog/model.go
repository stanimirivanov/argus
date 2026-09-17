// Package catalog defines the normalized repository and test catalog model.
package catalog

// Provider identifies a supported source-code hosting provider.
type Provider string

const (
	// ProviderGitHub identifies GitHub and GitHub Enterprise hosts.
	ProviderGitHub Provider = "github"
	// ProviderGitLab identifies GitLab and self-managed GitLab hosts.
	ProviderGitLab Provider = "gitlab"
	// ProviderAzureDevOps identifies Azure DevOps hosts.
	ProviderAzureDevOps Provider = "azure-devops"
	// ProviderOther identifies a provider without first-class semantics yet.
	ProviderOther Provider = "other"
)

// RevisionAlgorithm describes how an immutable source revision digest is
// encoded.
type RevisionAlgorithm string

const (
	// RevisionGitSHA1 is the 160-bit Git object format.
	RevisionGitSHA1 RevisionAlgorithm = "git-sha1"
	// RevisionGitSHA256 is the 256-bit Git object format.
	RevisionGitSHA256 RevisionAlgorithm = "git-sha256"
)

// Revision is an immutable, ingestion-verified source revision.
type Revision struct {
	Algorithm RevisionAlgorithm `json:"algorithm"`
	Digest    string            `json:"digest"`
}

// RepositoryIdentity is stable across repository renames and ownership
// transfers. ProviderRepositoryID is deliberately opaque.
type RepositoryIdentity struct {
	Provider             Provider `json:"provider"`
	Host                 string   `json:"host"`
	ProviderRepositoryID string   `json:"providerRepositoryId"`
}

// Repository combines stable identity with mutable display coordinates.
type Repository struct {
	Identity RepositoryIdentity `json:"identity"`
	Owner    string             `json:"owner"`
	Name     string             `json:"name"`
}

// Capability is a repository-scoped product behavior.
type Capability struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Component is a repository-scoped source tree root mapped to capabilities.
type Component struct {
	Key          string   `json:"key"`
	Root         string   `json:"root"`
	Capabilities []string `json:"capabilities"`
}

// TestFamily classifies the intent and execution boundary of a test suite.
type TestFamily string

const (
	// TestFamilyUnit contains isolated unit tests.
	TestFamilyUnit TestFamily = "unit"
	// TestFamilyComponent contains tests scoped to one deployable component.
	TestFamilyComponent TestFamily = "component"
	// TestFamilyContract contains consumer or provider contract tests.
	TestFamilyContract TestFamily = "contract"
	// TestFamilyIntegration contains tests across multiple technical units.
	TestFamilyIntegration TestFamily = "integration"
	// TestFamilyFunctionalAPI contains functional tests through an API.
	TestFamilyFunctionalAPI TestFamily = "functional-api"
	// TestFamilyFunctionalUI contains functional tests through a user interface.
	TestFamilyFunctionalUI TestFamily = "functional-ui"
	// TestFamilyEndToEnd contains complete user or system journeys.
	TestFamilyEndToEnd TestFamily = "end-to-end"
	// TestFamilyPerformance contains load, stress, and responsiveness tests.
	TestFamilyPerformance TestFamily = "performance"
	// TestFamilySecurity contains security verification tests.
	TestFamilySecurity TestFamily = "security"
	// TestFamilyResilience contains failure and recovery experiments.
	TestFamilyResilience TestFamily = "resilience"
	// TestFamilyOther is an extension point for uncategorized suites.
	TestFamilyOther TestFamily = "other"
)

// Test is a stable suite-scoped test mapped to source capabilities.
type Test struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

// TestSuite groups tests that share a repository, family, and adapter.
type TestSuite struct {
	Key        string     `json:"key"`
	Repository Repository `json:"repository"`
	Family     TestFamily `json:"family"`
	Adapter    string     `json:"adapter"`
	Tests      []Test     `json:"tests"`
}

// Snapshot is the normalized catalog state observed at one immutable source
// revision.
type Snapshot struct {
	APIVersion   string       `json:"apiVersion"`
	Repository   Repository   `json:"repository"`
	Revision     Revision     `json:"revision"`
	Capabilities []Capability `json:"capabilities"`
	Components   []Component  `json:"components"`
	TestSuites   []TestSuite  `json:"testSuites"`
}

// SnapshotKey identifies one immutable descriptor observation.
type SnapshotKey struct {
	Repository RepositoryIdentity `json:"repository"`
	Revision   Revision           `json:"revision"`
	APIVersion string             `json:"apiVersion"`
}

// Key returns the immutable identity used to persist and retrieve the snapshot.
func (snapshot Snapshot) Key() SnapshotKey {
	return SnapshotKey{
		Repository: snapshot.Repository.Identity,
		Revision:   snapshot.Revision,
		APIVersion: snapshot.APIVersion,
	}
}
