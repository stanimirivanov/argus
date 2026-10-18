// Package browserlocator owns proposal-only Playwright locator repair policy.
package browserlocator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// Version constants identify the snapshot, diagnosis, proposal, and policy contracts.
const (
	SnapshotVersion  = "argus.dev/browser-structure/v1"
	DiagnosisVersion = "argus.dev/browser-locator-diagnosis/v1"
	ProposalVersion  = "argus.dev/browser-locator-proposal/v1"
	PolicyVersion    = "argus.dev/adaptation-policy/browser-stable-locator/v1"
)

var (
	roleCall  = regexp.MustCompile(`getByRole\((["'])([A-Za-z][A-Za-z0-9-]*)["'],\s*\{\s*name:\s*(["'])([^"'\r\n]{1,200})["']\s*\}\s*\)`)
	idCall    = regexp.MustCompile(`getByTestId\((["'])([A-Za-z0-9_.:-]{1,200})["']\)`)
	hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Node is one potentially matching DOM element, including hidden elements.
type Node struct {
	Role    string `json:"role"`
	Name    string `json:"name"`
	TestID  string `json:"testId"`
	Visible bool   `json:"visible"`
}

// Snapshot is captured by the reviewed Playwright hook before page teardown.
type Snapshot struct {
	APIVersion string    `json:"apiVersion"`
	CapturedAt time.Time `json:"capturedAt"`
	URL        string    `json:"url"`
	Complete   bool      `json:"complete"`
	Nodes      []Node    `json:"nodes"`
	ARIA       string    `json:"ariaSnapshot"`
}

// Request binds prior passing and current failed attempts to exact snapshot bytes
// and the current test source preimage. It never authorizes a source mutation.
type Request struct {
	Baseline     browser.Attempt
	Failed       browser.Attempt
	SuiteKey     string
	TestKey      string
	SourcePath   string
	Source       []byte
	BaselineData []byte
	FailedData   []byte
}

// Diagnosis records a candidate or a deterministic abstention reason.
type Diagnosis struct {
	APIVersion string    `json:"apiVersion"`
	ReasonCode string    `json:"reasonCode"`
	BaselineID string    `json:"baselineAttemptId"`
	FailedID   string    `json:"failedAttemptId"`
	SuiteKey   string    `json:"suiteKey"`
	TestKey    string    `json:"testKey"`
	Proposal   *Proposal `json:"proposal,omitempty"`
}

// Proposal is one exact source-preimage edit requiring later validation.
type Proposal struct {
	APIVersion     string              `json:"apiVersion"`
	PolicyVersion  string              `json:"policyVersion"`
	ProposalID     string              `json:"proposalId"`
	BaselineID     string              `json:"baselineAttemptId"`
	FailedID       string              `json:"failedAttemptId"`
	BaselineSHA256 string              `json:"baselineSnapshotSha256"`
	FailedSHA256   string              `json:"failedSnapshotSha256"`
	SuiteKey       string              `json:"suiteKey"`
	TestKey        string              `json:"testKey"`
	OriginalTarget Node                `json:"originalTarget"`
	CurrentTarget  Node                `json:"currentTarget"`
	Edit           adaptation.TextEdit `json:"edit"`
}

func validRequest(request Request) error {
	if browser.ValidateAttempt(request.Baseline) != nil || browser.ValidateAttempt(request.Failed) != nil ||
		request.Baseline.Request.AttemptID == request.Failed.Request.AttemptID ||
		request.Baseline.Request.Project != request.Failed.Request.Project ||
		request.Baseline.Request.Environment != request.Failed.Request.Environment ||
		request.Baseline.Request.TestRepository.Identity != request.Failed.Request.TestRepository.Identity ||
		request.Baseline.Request.TestRevision != request.Failed.Request.TestRevision ||
		request.Baseline.Result.CompletedAt.After(request.Failed.Result.StartedAt) ||
		adaptation.ValidateSourcePath(request.SourcePath) != nil || len(request.Source) == 0 ||
		len(request.Source) > adaptation.MaxSourceBytes || len(request.BaselineData) == 0 ||
		len(request.FailedData) == 0 || len(request.BaselineData) > 1<<20 || len(request.FailedData) > 1<<20 {
		return adaptation.ErrInvalid
	}
	if testResult(request.Baseline, request.SuiteKey, request.TestKey).Outcome != execution.TestPassed ||
		testResult(request.Failed, request.SuiteKey, request.TestKey).Outcome != execution.TestFailed ||
		!hasSnapshot(request.Baseline, request.SuiteKey, request.TestKey, digest(request.BaselineData)) ||
		!hasSnapshot(request.Failed, request.SuiteKey, request.TestKey, digest(request.FailedData)) {
		return adaptation.ErrInvalid
	}

	return nil
}

func testResult(attempt browser.Attempt, suiteKey, testKey string) execution.TestResult {
	for _, result := range attempt.Result.Results {
		if result.SuiteKey == suiteKey && result.TestKey == testKey {
			return result
		}
	}

	return execution.TestResult{}
}

func hasSnapshot(attempt browser.Attempt, suiteKey, testKey, hash string) bool {
	for _, artifact := range attempt.Result.Artifacts {
		if artifact.SuiteKey == suiteKey && artifact.TestKey == testKey && artifact.Kind == "accessibility" && artifact.SHA256 == hash {
			return true
		}
	}

	return false
}

func parseSnapshot(data []byte, attempt browser.Attempt) (Snapshot, error) {
	var snapshot Snapshot
	if json.Unmarshal(data, &snapshot) != nil || snapshot.APIVersion != SnapshotVersion ||
		snapshot.CapturedAt.Before(attempt.Result.StartedAt) || snapshot.CapturedAt.After(attempt.Result.CompletedAt) ||
		len(snapshot.Nodes) > 10000 || len(snapshot.ARIA) > 1<<20 || snapshot.URL == "" || len(snapshot.URL) > 2048 {
		return Snapshot{}, fmt.Errorf("%w: stale or malformed browser structure", adaptation.ErrInvalid)
	}
	for _, node := range snapshot.Nodes {
		if len(node.Role) > 100 || len(node.Name) > 200 || len(node.TestID) > 200 ||
			strings.ContainsAny(node.Role+node.Name+node.TestID, "\r\n") {
			return Snapshot{}, fmt.Errorf("%w: malformed browser structure node", adaptation.ErrInvalid)
		}
	}

	return snapshot, nil
}

// ValidateProposal protects future validation and publication consumers.
func ValidateProposal(proposal Proposal) error {
	if proposal.APIVersion != ProposalVersion || proposal.PolicyVersion != PolicyVersion ||
		!hexDigest.MatchString(proposal.ProposalID) || !hexDigest.MatchString(proposal.BaselineSHA256) ||
		!hexDigest.MatchString(proposal.FailedSHA256) || proposal.BaselineID == proposal.FailedID ||
		!catalog.IsLocalKey(proposal.SuiteKey) || !catalog.IsLocalKey(proposal.TestKey) || adaptation.ValidateTextEdit(proposal.Edit) != nil ||
		proposal.Edit.SemanticRole != "browser-locator" ||
		len(proposal.Edit.Original) > 500 || len(proposal.Edit.Replacement) > 500 ||
		!proposal.OriginalTarget.Visible || !proposal.CurrentTarget.Visible {
		return adaptation.ErrInvalid
	}
	old, ok := exactLocator(proposal.Edit.Original)
	if !ok || len(matching([]Node{proposal.OriginalTarget}, old)) != 1 {
		return adaptation.ErrInvalid
	}
	newCall, ok := exactLocator(proposal.Edit.Replacement)
	if !ok || len(matching([]Node{proposal.CurrentTarget}, newCall)) != 1 {
		return adaptation.ErrInvalid
	}
	if old.kind != newCall.kind || old.quote != newCall.quote {
		return adaptation.ErrInvalid
	}
	if old.kind == "role" && (proposal.OriginalTarget.TestID == "" ||
		proposal.OriginalTarget.TestID != proposal.CurrentTarget.TestID ||
		proposal.OriginalTarget.Role != proposal.CurrentTarget.Role) {
		return adaptation.ErrInvalid
	}
	if old.kind == "test-id" && (proposal.OriginalTarget.Role == "" ||
		proposal.OriginalTarget.Name == "" || proposal.OriginalTarget.Role != proposal.CurrentTarget.Role ||
		proposal.OriginalTarget.Name != proposal.CurrentTarget.Name) {
		return adaptation.ErrInvalid
	}
	copy := proposal
	copy.ProposalID = ""
	data, err := json.Marshal(copy)
	if err != nil || digest(data) != proposal.ProposalID {
		return adaptation.ErrInvalid
	}

	return nil
}

// ValidateDiagnosis ensures a candidate belongs to the decision envelope and
// an abstention cannot smuggle an edit to a downstream consumer.
func ValidateDiagnosis(value Diagnosis) error {
	if value.APIVersion != DiagnosisVersion || value.BaselineID == value.FailedID ||
		!catalog.IsLocalKey(value.SuiteKey) || !catalog.IsLocalKey(value.TestKey) {
		return adaptation.ErrInvalid
	}
	if value.ReasonCode != "candidate" {
		if value.Proposal != nil || value.ReasonCode == "" {
			return adaptation.ErrInvalid
		}
		return nil
	}
	if value.Proposal == nil || ValidateProposal(*value.Proposal) != nil ||
		value.Proposal.BaselineID != value.BaselineID || value.Proposal.FailedID != value.FailedID ||
		value.Proposal.SuiteKey != value.SuiteKey || value.Proposal.TestKey != value.TestKey {
		return adaptation.ErrInvalid
	}

	return nil
}

func digest(data []byte) string {
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:])
}
