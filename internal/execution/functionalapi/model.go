package functionalapi

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

const (
	// AdapterRequestAPIVersion identifies functional API adapter input v1.
	AdapterRequestAPIVersion = "argus.dev/functional-api-adapter-request/v1"
	// AdapterResultAPIVersion identifies untrusted functional API adapter output v1.
	AdapterResultAPIVersion = "argus.dev/functional-api-adapter-result/v1"
)

// Test identifies one requested functional API test.
type Test struct {
	SuiteKey string
	TestKey  string
	Name     string
}

// Identity returns the stable suite-scoped test identity.
func (test Test) Identity(repository catalog.RepositoryIdentity) catalog.TestIdentity {
	return catalog.TestIdentity{TestRepository: repository, SuiteKey: test.SuiteKey, TestKey: test.TestKey}
}

// Request is the exact input passed to one functional API adapter process.
type Request struct {
	APIVersion     string
	AttemptID      string
	Manifest       execution.ManifestReference
	Stage          execution.Stage
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	Adapter        string
	Tests          []Test
}

// AdapterResult is untrusted output returned by a functional API adapter.
type AdapterResult struct {
	APIVersion     string
	AttemptID      string
	AdapterID      string
	AdapterVersion string
	StartedAt      time.Time
	CompletedAt    time.Time
	Results        []execution.TestResult
	Artifacts      []execution.ArtifactReference
}

// CanonicalRequest deep-copies and orders requested tests.
func CanonicalRequest(request Request) Request {
	canonical := request
	canonical.Tests = append([]Test{}, request.Tests...)
	slices.SortFunc(canonical.Tests, func(left, right Test) int {
		if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
			return compared
		}

		return cmp.Compare(left.TestKey, right.TestKey)
	})

	return canonical
}

// CanonicalAdapterResult deep-copies and orders result and artifact sets.
func CanonicalAdapterResult(result AdapterResult) AdapterResult {
	canonical := result
	canonical.StartedAt = result.StartedAt.UTC()
	canonical.CompletedAt = result.CompletedAt.UTC()
	canonical.Results, canonical.Artifacts = execution.CanonicalEvidence(result.Results, result.Artifacts)

	return canonical
}

// ValidateRequest rejects ambiguous, unbounded, or mutable adapter input.
func ValidateRequest(request Request) error {
	if request.APIVersion != AdapterRequestAPIVersion ||
		execution.ValidateAttemptID(request.AttemptID) != nil ||
		(request.Stage != execution.StageSelected && request.Stage != execution.StageFullSuite) ||
		!catalog.IsLocalKey(request.Adapter) || len(request.Tests) == 0 ||
		len(request.Tests) > execution.MaxAttemptTests {
		return fmt.Errorf("%w: request envelope", execution.ErrInvalid)
	}
	if err := execution.ValidateManifestReference(request.Manifest); err != nil {
		return err
	}
	if err := execution.ValidateTestRepository(request.TestRepository); err != nil {
		return err
	}
	if err := execution.ValidateTestRevision(request.TestRevision); err != nil {
		return err
	}
	identities := make(map[catalog.TestIdentity]struct{}, len(request.Tests))
	for _, test := range request.Tests {
		identity := test.Identity(request.TestRepository.Identity)
		if !identity.Valid() || strings.TrimSpace(test.Name) == "" || len(test.Name) > 255 {
			return fmt.Errorf("%w: requested test", execution.ErrInvalid)
		}
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate requested test", execution.ErrInvalid)
		}
		identities[identity] = struct{}{}
	}

	return nil
}

// ValidateAdapterResult checks structural evidence before request correlation.
func ValidateAdapterResult(result AdapterResult) error {
	if result.APIVersion != AdapterResultAPIVersion ||
		execution.ValidateAttemptID(result.AttemptID) != nil || !catalog.IsLocalKey(result.AdapterID) ||
		strings.TrimSpace(result.AdapterVersion) == "" || len(result.AdapterVersion) > 127 {
		return fmt.Errorf("%w: adapter result envelope", execution.ErrInvalid)
	}

	return execution.ValidateResultEvidence(result.StartedAt, result.CompletedAt, result.Results, result.Artifacts)
}
