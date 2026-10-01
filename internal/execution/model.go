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

// CanonicalEvidence deep-copies and orders normalized results and artifacts.
// It does not validate them; producers and persistence boundaries must validate
// the complete evidence envelope separately.
func CanonicalEvidence(results []TestResult, artifacts []ArtifactReference) ([]TestResult, []ArtifactReference) {
	canonicalResults := append([]TestResult{}, results...)
	for index, item := range canonicalResults {
		if item.Failure != nil {
			failure := *item.Failure
			canonicalResults[index].Failure = &failure
		}
	}
	slices.SortFunc(canonicalResults, compareResults)
	canonicalArtifacts := append([]ArtifactReference{}, artifacts...)
	slices.SortFunc(canonicalArtifacts, func(left, right ArtifactReference) int {
		return cmp.Compare(left.Key, right.Key)
	})

	return canonicalResults, canonicalArtifacts
}

// CanonicalAttempt deep-copies and orders normalized attempt evidence.
func CanonicalAttempt(attempt Attempt) Attempt {
	canonical := attempt
	canonical.StartedAt = attempt.StartedAt.UTC()
	canonical.CompletedAt = attempt.CompletedAt.UTC()
	canonical.Results, canonical.Artifacts = CanonicalEvidence(attempt.Results, attempt.Artifacts)

	return canonical
}

// ValidateAttemptID verifies a stable external attempt identity without
// requiring the rest of an execution document.
func ValidateAttemptID(attemptID string) error {
	if !attemptIDPattern.MatchString(attemptID) {
		return fmt.Errorf("%w: attempt ID", ErrInvalid)
	}

	return nil
}

// ValidateManifestReference verifies the version and digest of canonical
// execution-manifest bytes without loading the manifest itself.
func ValidateManifestReference(reference ManifestReference) error {
	if reference.APIVersion != selection.ManifestAPIVersion ||
		!hexSHA256Pattern.MatchString(reference.SHA256) {
		return fmt.Errorf("%w: manifest reference", ErrInvalid)
	}

	return nil
}

// ValidateResultEvidence checks the common bounded, immutable result and
// artifact rules shared by normalized attempts and framework adapters.
func ValidateResultEvidence(startedAt, completedAt time.Time, results []TestResult, artifacts []ArtifactReference) error {
	if len(results) == 0 || len(results) > MaxAttemptTests || len(artifacts) > MaxAttemptArtifacts {
		return fmt.Errorf("%w: result envelope", ErrInvalid)
	}
	if !isUTC(startedAt) || !isUTC(completedAt) || startedAt.IsZero() ||
		completedAt.Before(startedAt) || hasSubMicrosecondPrecision(startedAt) ||
		hasSubMicrosecondPrecision(completedAt) {
		return fmt.Errorf("%w: result timestamps", ErrInvalid)
	}
	if err := validateResults(results); err != nil {
		return err
	}

	return validateArtifacts(artifacts)
}

// ValidateAttempt verifies normalized evidence independently of an adapter.
func ValidateAttempt(attempt Attempt) error {
	if attempt.APIVersion != AttemptAPIVersion {
		return fmt.Errorf("%w: attempt version", ErrInvalid)
	}
	if ValidateAttemptID(attempt.AttemptID) != nil ||
		(attempt.Stage != StageSelected && attempt.Stage != StageFullSuite) ||
		!catalog.IsLocalKey(attempt.AdapterID) || strings.TrimSpace(attempt.AdapterVersion) == "" ||
		len(attempt.AdapterVersion) > 127 {
		return fmt.Errorf("%w: attempt envelope", ErrInvalid)
	}
	if err := ValidateManifestReference(attempt.Manifest); err != nil {
		return err
	}
	if err := ValidateTestRepository(attempt.TestRepository); err != nil {
		return err
	}
	if err := ValidateTestRevision(attempt.TestRevision); err != nil {
		return err
	}
	if err := ValidateResultEvidence(attempt.StartedAt, attempt.CompletedAt, attempt.Results, attempt.Artifacts); err != nil {
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

// ValidateTestRepository verifies the stable identity and display coordinates
// carried by execution evidence independently of its adapter family.
func ValidateTestRepository(repository catalog.Repository) error {
	identity := catalog.TestIdentity{
		TestRepository: repository.Identity, SuiteKey: "valid", TestKey: "valid",
	}
	if !identity.Valid() || strings.TrimSpace(repository.Owner) == "" || len(repository.Owner) > 255 ||
		strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return fmt.Errorf("%w: test repository", ErrInvalid)
	}

	return nil
}

// ValidateTestRevision verifies the immutable revision of the test checkout.
func ValidateTestRevision(revision catalog.Revision) error {
	normalized, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil || normalized != revision {
		return fmt.Errorf("%w: test revision", ErrInvalid)
	}

	return nil
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
