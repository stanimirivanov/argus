// Package browser owns execution evidence for an explicit browser project and environment.
package browser

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

// Browser protocol versions remain distinct from functional API execution v1.
const (
	ManifestVersion = "argus.dev/browser-selection-manifest/v1"
	RequestVersion  = "argus.dev/browser-adapter-request/v1"
	ResultVersion   = "argus.dev/browser-adapter-result/v1"
	AttemptVersion  = "argus.dev/browser-execution-attempt/v1"
	MaxArtifacts    = 3 * execution.MaxAttemptTests
)

// ErrInvalid identifies malformed or uncorrelated browser execution evidence.
var ErrInvalid = errors.New("invalid browser execution evidence")

// Test is one stable browser test requested for a project.
type Test struct {
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Name     string `json:"name"`
}

// Request is the exact CI-local browser adapter input.
type Request struct {
	Staging        *Staging                    `json:"staging,omitempty"`
	Candidates     []Test                      `json:"candidates,omitempty"`
	APIVersion     string                      `json:"apiVersion"`
	AttemptID      string                      `json:"attemptId"`
	Manifest       execution.ManifestReference `json:"manifest"`
	Stage          execution.Stage             `json:"stage"`
	TestRepository catalog.Repository          `json:"testRepository"`
	TestRevision   catalog.Revision            `json:"testRevision"`
	Adapter        string                      `json:"adapter"`
	Project        string                      `json:"project"`
	Environment    string                      `json:"environment"`
	Tests          []Test                      `json:"tests"`
}

// Artifact references one test's trace, screenshot, or accessibility evidence.
type Artifact struct {
	Key      string `json:"key"`
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	SHA256   string `json:"sha256"`
}

// Result is untrusted browser adapter output before correlation.
type Result struct {
	APIVersion     string                 `json:"apiVersion"`
	AttemptID      string                 `json:"attemptId"`
	AdapterID      string                 `json:"adapterId"`
	AdapterVersion string                 `json:"adapterVersion"`
	Project        string                 `json:"project"`
	Environment    string                 `json:"environment"`
	StartedAt      time.Time              `json:"startedAt"`
	CompletedAt    time.Time              `json:"completedAt"`
	Results        []execution.TestResult `json:"results"`
	Artifacts      []Artifact             `json:"artifacts"`
}

// Attempt is normalized browser evidence bound to the exact request.
type Attempt struct {
	APIVersion string                   `json:"apiVersion"`
	Request    Request                  `json:"request"`
	Result     Result                   `json:"result"`
	Outcome    execution.AttemptOutcome `json:"outcome"`
}

// CanonicalAttempt deep-copies and orders request and result evidence.
func CanonicalAttempt(attempt Attempt) Attempt {
	if attempt.Request.Staging != nil {
		value := *attempt.Request.Staging
		attempt.Request.Staging = &value
	}
	attempt.Request.Candidates = slices.Clone(attempt.Request.Candidates)
	slices.SortFunc(attempt.Request.Candidates, func(a, b Test) int {
		if n := cmp.Compare(a.SuiteKey, b.SuiteKey); n != 0 {
			return n
		}
		return cmp.Compare(a.TestKey, b.TestKey)
	})
	attempt.Request.Tests = append([]Test{}, attempt.Request.Tests...)
	if attempt.Request.APIVersion != StagedRequestVersion {
		slices.SortFunc(attempt.Request.Tests, func(a, b Test) int {
			if n := cmp.Compare(a.SuiteKey, b.SuiteKey); n != 0 {
				return n
			}

			return cmp.Compare(a.TestKey, b.TestKey)
		})
	}
	attempt.Result.Results = append([]execution.TestResult{}, attempt.Result.Results...)
	for i, item := range attempt.Result.Results {
		if item.Failure != nil {
			value := *item.Failure
			attempt.Result.Results[i].Failure = &value
		}
	}
	slices.SortFunc(attempt.Result.Results, func(a, b execution.TestResult) int {
		if n := cmp.Compare(a.SuiteKey, b.SuiteKey); n != 0 {
			return n
		}

		return cmp.Compare(a.TestKey, b.TestKey)
	})
	attempt.Result.Artifacts = append([]Artifact{}, attempt.Result.Artifacts...)
	slices.SortFunc(attempt.Result.Artifacts, func(a, b Artifact) int { return cmp.Compare(a.Key, b.Key) })
	attempt.Result.StartedAt = attempt.Result.StartedAt.UTC()
	attempt.Result.CompletedAt = attempt.Result.CompletedAt.UTC()

	return attempt
}

// ValidateRequest rejects ambiguous or mutable browser adapter input.
func ValidateRequest(request Request) error {
	if request.APIVersion != RequestVersion || request.Staging != nil || len(request.Candidates) != 0 || execution.ValidateAttemptID(request.AttemptID) != nil || request.Manifest.APIVersion != ManifestVersion || !digestValid(request.Manifest.SHA256) ||
		(request.Stage != execution.StageSelected && request.Stage != execution.StageFullSuite) || execution.ValidateTestRepository(request.TestRepository) != nil || execution.ValidateTestRevision(request.TestRevision) != nil ||
		!catalog.IsLocalKey(request.Adapter) || !catalog.IsLocalKey(request.Project) || !catalog.IsLocalKey(request.Environment) || len(request.Tests) == 0 || len(request.Tests) > execution.MaxAttemptTests {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, test := range request.Tests {
		key := test.SuiteKey + "\x00" + test.TestKey
		if !catalog.IsLocalKey(test.SuiteKey) || !catalog.IsLocalKey(test.TestKey) || strings.TrimSpace(test.Name) == "" || len(test.Name) > 255 || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
	}

	return nil
}

// ValidateResult checks exact result membership and required artifacts.
func ValidateResult(request Request, result Result) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if result.APIVersion != ResultVersion || result.AttemptID != request.AttemptID || result.AdapterID != request.Adapter || result.Project != request.Project || result.Environment != request.Environment ||
		strings.TrimSpace(result.AdapterVersion) == "" || len(result.AdapterVersion) > 127 || len(result.Results) != len(request.Tests) || len(result.Artifacts) > MaxArtifacts {
		return ErrInvalid
	}
	if err := execution.ValidateResultEvidence(result.StartedAt, result.CompletedAt, result.Results, nil); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := correlateResults(request.Tests, result.Results); err != nil {
		return err
	}

	return validateArtifacts(request.Tests, result)
}

func correlateResults(tests []Test, results []execution.TestResult) error {
	expected := map[string]bool{}
	for _, test := range tests {
		expected[test.SuiteKey+"\x00"+test.TestKey] = true
	}
	for _, test := range results {
		key := test.SuiteKey + "\x00" + test.TestKey
		if !expected[key] {
			return ErrInvalid
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		return ErrInvalid
	}

	return nil
}

func validateArtifacts(tests []Test, result Result) error {
	keys := map[string]bool{}
	kinds := map[string]map[string]bool{}
	for _, artifact := range result.Artifacts {
		key := artifact.SuiteKey + "\x00" + artifact.TestKey
		if !catalog.IsLocalKey(artifact.Key) || keys[artifact.Key] || !catalog.IsLocalKey(artifact.SuiteKey) || !catalog.IsLocalKey(artifact.TestKey) || !containsTest(tests, key) || !digestValid(artifact.SHA256) {
			return ErrInvalid
		}
		switch artifact.Kind {
		case "trace", "screenshot", "accessibility":
		default:
			return ErrInvalid
		}
		uri, err := url.Parse(artifact.URI)
		if err != nil || uri.Scheme != "file" || uri.User != nil || len(artifact.URI) > 2048 {
			return ErrInvalid
		}
		if kinds[key] == nil {
			kinds[key] = map[string]bool{}
		}
		if kinds[key][artifact.Kind] {
			return ErrInvalid
		}
		kinds[key][artifact.Kind] = true
		keys[artifact.Key] = true
	}

	return validateArtifactCoverage(result.Results, kinds)
}

func validateArtifactCoverage(results []execution.TestResult, kinds map[string]map[string]bool) error {
	for _, test := range results {
		observed := kinds[test.SuiteKey+"\x00"+test.TestKey]
		if test.Outcome == execution.TestSkipped {
			if len(observed) != 0 {
				return ErrInvalid
			}
			continue
		}
		if !observed["trace"] || !observed["screenshot"] || !observed["accessibility"] {
			return ErrInvalid
		}
	}

	return nil
}

// ValidateAttempt checks a normalized browser attempt before persistence.
func ValidateAttempt(attempt Attempt) error {
	if attempt.APIVersion != AttemptVersion || attempt.Outcome != execution.DeriveAttemptOutcome(attempt.Result.Results) {
		return ErrInvalid
	}

	return ValidateResult(attempt.Request, attempt.Result)
}

func containsTest(tests []Test, key string) bool {
	for _, test := range tests {
		if test.SuiteKey+"\x00"+test.TestKey == key {
			return true
		}
	}

	return false
}
func digestValid(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}

	return true
}
