// Package execution owns framework-neutral test-attempt evidence.
package execution

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/selection"
)

const (
	// FunctionalAPIAdapterRequestAPIVersion identifies adapter input v1.
	FunctionalAPIAdapterRequestAPIVersion = "argus.dev/functional-api-adapter-request/v1"
	// FunctionalAPIAdapterResultAPIVersion identifies untrusted adapter output v1.
	FunctionalAPIAdapterResultAPIVersion = "argus.dev/functional-api-adapter-result/v1"
	// AttemptAPIVersion identifies normalized execution-attempt evidence v1.
	AttemptAPIVersion = "argus.dev/execution-attempt/v1"
	// MaxAttemptTests bounds one adapter process and its normalized evidence.
	MaxAttemptTests = selection.MaxManifestDecisions
	// MaxAttemptArtifacts bounds external evidence references from one attempt.
	MaxAttemptArtifacts = 100
	// MaxTestDuration rejects implausible or overflowing adapter durations.
	MaxTestDuration = 24 * time.Hour
)

var (
	// ErrInvalid means execution input or adapter output violates an invariant.
	ErrInvalid = errors.New("invalid execution evidence")
	// ErrUnavailable means an adapter could not provide trustworthy evidence.
	ErrUnavailable = errors.New("execution adapter unavailable")
	// ErrNotFound means requested execution evidence does not exist.
	ErrNotFound = errors.New("execution evidence not found")
	// ErrConflict means an immutable execution identity was reused with different content.
	ErrConflict = errors.New("execution evidence conflict")
	// ErrAttemptNotPassed means valid evidence contains a non-passing outcome.
	ErrAttemptNotPassed = errors.New("execution attempt did not pass")

	attemptIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,126}$`)
	hexSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Stage identifies the selected or full-suite execution pass.
type Stage string

const (
	// StageSelected runs only tests required by the early selection policy.
	StageSelected Stage = "selected"
	// StageFullSuite runs every candidate so selection misses remain observable.
	StageFullSuite Stage = "full-suite"
)

// TestOutcome is the normalized result of one requested test.
type TestOutcome string

const (
	// TestPassed means the requested test completed successfully.
	TestPassed TestOutcome = "passed"
	// TestFailed means assertions or expected behavior failed.
	TestFailed TestOutcome = "failed"
	// TestSkipped means the adapter did not execute an otherwise runnable test.
	TestSkipped TestOutcome = "skipped"
	// TestError means the test could not produce a valid assertion result.
	TestError TestOutcome = "error"
)

// AttemptOutcome summarizes all normalized test results.
type AttemptOutcome string

const (
	// AttemptPassed means every requested test passed.
	AttemptPassed AttemptOutcome = "passed"
	// AttemptFailed means at least one test failed and none errored.
	AttemptFailed AttemptOutcome = "failed"
	// AttemptIncomplete means no test failed or errored but at least one was skipped.
	AttemptIncomplete AttemptOutcome = "incomplete"
	// AttemptError means at least one test ended in an execution error.
	AttemptError AttemptOutcome = "error"
)

// ManifestReference binds an attempt to canonical manifest bytes.
type ManifestReference struct {
	APIVersion string
	SHA256     string
}

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
	Manifest       ManifestReference
	Stage          Stage
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	Adapter        string
	Tests          []Test
}

// Failure is bounded, normalized diagnostic metadata rather than raw output.
type Failure struct {
	Code    string
	Message string
}

// TestResult records one result for one requested test.
type TestResult struct {
	SuiteKey string
	TestKey  string
	Outcome  TestOutcome
	Duration time.Duration
	Failure  *Failure
}

// Identity returns the stable suite-scoped result identity.
func (result TestResult) Identity(repository catalog.RepositoryIdentity) catalog.TestIdentity {
	return catalog.TestIdentity{TestRepository: repository, SuiteKey: result.SuiteKey, TestKey: result.TestKey}
}

// ArtifactReference identifies immutable evidence without loading its bytes.
type ArtifactReference struct {
	Key    string
	Kind   string
	URI    string
	SHA256 string
}

// AdapterResult is untrusted output returned by a functional API adapter.
type AdapterResult struct {
	APIVersion     string
	AttemptID      string
	AdapterID      string
	AdapterVersion string
	StartedAt      time.Time
	CompletedAt    time.Time
	Results        []TestResult
	Artifacts      []ArtifactReference
}

// Attempt is normalized evidence correlated with one exact request.
type Attempt struct {
	APIVersion     string
	AttemptID      string
	Manifest       ManifestReference
	Stage          Stage
	TestRepository catalog.Repository
	TestRevision   catalog.Revision
	AdapterID      string
	AdapterVersion string
	StartedAt      time.Time
	CompletedAt    time.Time
	Outcome        AttemptOutcome
	Results        []TestResult
	Artifacts      []ArtifactReference
}

// CanonicalRequest deep-copies and orders requested tests.
func CanonicalRequest(request Request) Request {
	canonical := request
	canonical.Tests = append([]Test{}, request.Tests...)
	slices.SortFunc(canonical.Tests, compareTests)

	return canonical
}

// CanonicalAdapterResult deep-copies and orders result and artifact sets.
func CanonicalAdapterResult(result AdapterResult) AdapterResult {
	canonical := result
	canonical.StartedAt = result.StartedAt.UTC()
	canonical.CompletedAt = result.CompletedAt.UTC()
	canonical.Results = append([]TestResult{}, result.Results...)
	for index, item := range canonical.Results {
		if item.Failure != nil {
			failure := *item.Failure
			canonical.Results[index].Failure = &failure
		}
	}
	slices.SortFunc(canonical.Results, compareResults)
	canonical.Artifacts = append([]ArtifactReference{}, result.Artifacts...)
	slices.SortFunc(canonical.Artifacts, func(left, right ArtifactReference) int {
		return cmp.Compare(left.Key, right.Key)
	})

	return canonical
}

// CanonicalAttempt deep-copies and orders normalized attempt evidence.
func CanonicalAttempt(attempt Attempt) Attempt {
	canonical := attempt
	result := CanonicalAdapterResult(AdapterResult{
		APIVersion: FunctionalAPIAdapterResultAPIVersion, AttemptID: attempt.AttemptID,
		AdapterID: attempt.AdapterID, AdapterVersion: attempt.AdapterVersion,
		StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt,
		Results: attempt.Results, Artifacts: attempt.Artifacts,
	})
	canonical.StartedAt = result.StartedAt
	canonical.CompletedAt = result.CompletedAt
	canonical.Results = result.Results
	canonical.Artifacts = result.Artifacts

	return canonical
}

// ValidateRequest rejects ambiguous, unbounded, or mutable execution input.
func ValidateRequest(request Request) error {
	if request.APIVersion != FunctionalAPIAdapterRequestAPIVersion ||
		!attemptIDPattern.MatchString(request.AttemptID) ||
		request.Manifest.APIVersion != selection.ManifestAPIVersion ||
		!hexSHA256Pattern.MatchString(request.Manifest.SHA256) ||
		(request.Stage != StageSelected && request.Stage != StageFullSuite) ||
		!catalog.IsLocalKey(request.Adapter) || len(request.Tests) == 0 ||
		len(request.Tests) > MaxAttemptTests {
		return fmt.Errorf("%w: request envelope", ErrInvalid)
	}
	if err := validateRepository(request.TestRepository); err != nil {
		return err
	}
	if err := validateRevision(request.TestRevision); err != nil {
		return err
	}
	identities := make(map[catalog.TestIdentity]struct{}, len(request.Tests))
	for _, test := range request.Tests {
		identity := test.Identity(request.TestRepository.Identity)
		if !identity.Valid() || strings.TrimSpace(test.Name) == "" || len(test.Name) > 255 {
			return fmt.Errorf("%w: requested test", ErrInvalid)
		}
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate requested test", ErrInvalid)
		}
		identities[identity] = struct{}{}
	}

	return nil
}

// ValidateAdapterResult verifies structural evidence before request correlation.
func ValidateAdapterResult(result AdapterResult) error {
	if result.APIVersion != FunctionalAPIAdapterResultAPIVersion ||
		!attemptIDPattern.MatchString(result.AttemptID) || !catalog.IsLocalKey(result.AdapterID) ||
		strings.TrimSpace(result.AdapterVersion) == "" || len(result.AdapterVersion) > 127 ||
		len(result.Results) == 0 || len(result.Results) > MaxAttemptTests ||
		len(result.Artifacts) > MaxAttemptArtifacts {
		return fmt.Errorf("%w: adapter result envelope", ErrInvalid)
	}
	if !isUTC(result.StartedAt) || !isUTC(result.CompletedAt) || result.StartedAt.IsZero() ||
		result.CompletedAt.Before(result.StartedAt) || hasSubMicrosecondPrecision(result.StartedAt) ||
		hasSubMicrosecondPrecision(result.CompletedAt) {
		return fmt.Errorf("%w: adapter result timestamps", ErrInvalid)
	}
	if err := validateResults(result.Results); err != nil {
		return err
	}

	return validateArtifacts(result.Artifacts)
}

// ValidateAttempt verifies normalized evidence independently of an adapter.
func ValidateAttempt(attempt Attempt) error {
	if attempt.APIVersion != AttemptAPIVersion {
		return fmt.Errorf("%w: attempt version", ErrInvalid)
	}
	request := Request{
		APIVersion: FunctionalAPIAdapterRequestAPIVersion, AttemptID: attempt.AttemptID,
		Manifest: attempt.Manifest, Stage: attempt.Stage,
		TestRepository: attempt.TestRepository, TestRevision: attempt.TestRevision,
		Adapter: attempt.AdapterID, Tests: testsFromResults(attempt.Results),
	}
	if err := ValidateRequest(request); err != nil {
		return err
	}
	result := AdapterResult{
		APIVersion: FunctionalAPIAdapterResultAPIVersion, AttemptID: attempt.AttemptID,
		AdapterID: attempt.AdapterID, AdapterVersion: attempt.AdapterVersion,
		StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt,
		Results: attempt.Results, Artifacts: attempt.Artifacts,
	}
	if err := ValidateAdapterResult(result); err != nil {
		return err
	}
	if attempt.Outcome != DeriveAttemptOutcome(attempt.Results) {
		return fmt.Errorf("%w: attempt outcome", ErrInvalid)
	}

	return nil
}

// DeriveAttemptOutcome applies stable precedence to per-test outcomes.
func DeriveAttemptOutcome(results []TestResult) AttemptOutcome {
	outcome := AttemptPassed
	for _, result := range results {
		switch result.Outcome {
		case TestError:
			return AttemptError
		case TestFailed:
			outcome = AttemptFailed
		case TestSkipped:
			if outcome == AttemptPassed {
				outcome = AttemptIncomplete
			}
		}
	}

	return outcome
}

func validateResults(results []TestResult) error {
	identities := make(map[string]struct{}, len(results))
	for _, result := range results {
		identity := result.SuiteKey + "\x00" + result.TestKey
		if !catalog.IsLocalKey(result.SuiteKey) || !catalog.IsLocalKey(result.TestKey) ||
			result.Duration < 0 || result.Duration > MaxTestDuration ||
			result.Duration%time.Millisecond != 0 {
			return fmt.Errorf("%w: test result", ErrInvalid)
		}
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("%w: duplicate test result", ErrInvalid)
		}
		identities[identity] = struct{}{}
		if err := validateResultFailure(result); err != nil {
			return err
		}
	}

	return nil
}

func testsFromResults(results []TestResult) []Test {
	tests := make([]Test, 0, len(results))
	for _, result := range results {
		tests = append(tests, Test{SuiteKey: result.SuiteKey, TestKey: result.TestKey, Name: result.TestKey})
	}

	return tests
}

func validateResultFailure(result TestResult) error {
	switch result.Outcome {
	case TestPassed, TestSkipped:
		if result.Failure != nil {
			return fmt.Errorf("%w: unexpected test failure", ErrInvalid)
		}
	case TestFailed, TestError:
		if result.Failure == nil || !catalog.IsLocalKey(result.Failure.Code) ||
			strings.TrimSpace(result.Failure.Message) == "" || len(result.Failure.Message) > 2000 {
			return fmt.Errorf("%w: missing or invalid test failure", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: test outcome", ErrInvalid)
	}

	return nil
}

func validateArtifacts(artifacts []ArtifactReference) error {
	keys := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		parsed, err := url.Parse(artifact.URI)
		if !catalog.IsLocalKey(artifact.Key) || !catalog.IsLocalKey(artifact.Kind) ||
			err != nil || parsed.Scheme == "" || parsed.User != nil ||
			len(artifact.URI) > 2048 || !hexSHA256Pattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("%w: artifact reference", ErrInvalid)
		}
		if _, exists := keys[artifact.Key]; exists {
			return fmt.Errorf("%w: duplicate artifact key", ErrInvalid)
		}
		keys[artifact.Key] = struct{}{}
	}

	return nil
}

func validateRepository(repository catalog.Repository) error {
	identity := catalog.TestIdentity{
		TestRepository: repository.Identity, SuiteKey: "valid", TestKey: "valid",
	}
	if !identity.Valid() || strings.TrimSpace(repository.Owner) == "" || len(repository.Owner) > 255 ||
		strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return fmt.Errorf("%w: test repository", ErrInvalid)
	}

	return nil
}

func validateRevision(revision catalog.Revision) error {
	normalized, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil || normalized != revision {
		return fmt.Errorf("%w: test revision", ErrInvalid)
	}

	return nil
}

func compareTests(left, right Test) int {
	if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
		return compared
	}

	return cmp.Compare(left.TestKey, right.TestKey)
}

func compareResults(left, right TestResult) int {
	if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
		return compared
	}

	return cmp.Compare(left.TestKey, right.TestKey)
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()

	return offset == 0
}

// Normalized timestamps stop at microseconds so portable storage boundaries
// can round-trip immutable evidence without silent precision loss.
func hasSubMicrosecondPrecision(value time.Time) bool {
	return value.Nanosecond()%1_000 != 0
}
