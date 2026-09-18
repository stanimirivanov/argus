package catalog

import (
	"cmp"
	"fmt"
	"regexp"
	"strings"
)

var localKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

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
	Algorithm RevisionAlgorithm
	Digest    string
}

// RepositoryIdentity is stable across repository renames and ownership
// transfers. ProviderRepositoryID is deliberately opaque.
type RepositoryIdentity struct {
	Provider             Provider
	Host                 string
	ProviderRepositoryID string
}

// Repository combines stable identity with mutable display coordinates.
type Repository struct {
	Identity RepositoryIdentity
	Owner    string
	Name     string
}

// Capability is a repository-scoped product behavior.
type Capability struct {
	Key  string
	Name string
}

// Component is a repository-scoped source tree root mapped to capabilities.
type Component struct {
	Key          string
	Root         string
	Capabilities []string
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
	Key          string
	Name         string
	Capabilities []string
}

// TestSuite groups tests that share a repository, family, and adapter.
type TestSuite struct {
	Key        string
	Repository Repository
	Family     TestFamily
	Adapter    string
	Tests      []Test
}

// Snapshot is the normalized catalog state observed at one immutable source
// revision. Values crossing a storage boundary must first pass a validated
// ingress adapter; the exported fields are not an alternative validation API.
type Snapshot struct {
	APIVersion   string
	Repository   Repository
	Revision     Revision
	Capabilities []Capability
	Components   []Component
	TestSuites   []TestSuite
}

// SnapshotKey identifies one immutable descriptor observation.
type SnapshotKey struct {
	Repository RepositoryIdentity
	Revision   Revision
	APIVersion string
}

// Valid reports whether the key names a complete, normalized immutable
// snapshot. Use-case packages translate a false result into their own stable
// query or evidence error.
func (key SnapshotKey) Valid() bool {
	return ValidateSnapshotKey(key) == nil
}

// ValidateSnapshotKey verifies every normalized component of an immutable
// snapshot identity and returns the stable query error used by consuming
// capabilities. Valid is the boolean form for invariant checks.
func ValidateSnapshotKey(key SnapshotKey) error {
	switch key.Repository.Provider {
	case ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, ProviderOther:
	default:
		return fmt.Errorf("%w: repository provider", ErrInvalidQuery)
	}
	if strings.TrimSpace(key.Repository.Host) == "" ||
		strings.TrimSpace(key.Repository.ProviderRepositoryID) == "" ||
		strings.TrimSpace(key.APIVersion) == "" {
		return fmt.Errorf("%w: incomplete snapshot identity", ErrInvalidQuery)
	}
	validatedRevision, err := NewRevision(key.Revision.Algorithm, key.Revision.Digest)

	if err != nil || validatedRevision != key.Revision {
		return fmt.Errorf("%w: revision", ErrInvalidQuery)
	}

	return nil
}

// SnapshotReference describes the immutable catalog observation associated
// with query results and evidence.
type SnapshotReference struct {
	SourceRepository     Repository
	Revision             Revision
	DescriptorAPIVersion string
}

// Key returns the immutable identity represented by the reference.
func (reference SnapshotReference) Key() SnapshotKey {
	return SnapshotKey{
		Repository: reference.SourceRepository.Identity,
		Revision:   reference.Revision,
		APIVersion: reference.DescriptorAPIVersion,
	}
}

// TestIdentity is stable across repository-coordinate and test display-name
// changes.
type TestIdentity struct {
	TestRepository RepositoryIdentity
	SuiteKey       string
	TestKey        string
}

// Valid reports whether every part of the stable test identity is normalized.
func (identity TestIdentity) Valid() bool {
	switch identity.TestRepository.Provider {
	case ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, ProviderOther:
	default:
		return false
	}

	return strings.TrimSpace(identity.TestRepository.Host) != "" &&
		strings.TrimSpace(identity.TestRepository.ProviderRepositoryID) != "" &&
		IsLocalKey(identity.SuiteKey) &&
		IsLocalKey(identity.TestKey)
}

// CompareTestIdentities returns the lexical ordering used by catalog query
// ports and deterministic cursors.
func CompareTestIdentities(left, right TestIdentity) int {
	if compared := compareRepositoryIdentities(left.TestRepository, right.TestRepository); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
		return compared
	}

	return cmp.Compare(left.TestKey, right.TestKey)
}

// IsLocalKey reports whether value is a normalized repository-local catalog
// key.
func IsLocalKey(value string) bool {
	return localKeyPattern.MatchString(value)
}

// Key returns the immutable identity used to persist and retrieve the snapshot.
func (snapshot Snapshot) Key() SnapshotKey {
	return SnapshotKey{
		Repository: snapshot.Repository.Identity,
		Revision:   snapshot.Revision,
		APIVersion: snapshot.APIVersion,
	}
}
