// Package browservalidation proves a browser locator repair with repeated,
// discriminating execution in a disposable test checkout.
package browservalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

const (
	// EvidenceVersion identifies the public browser validation contract.
	EvidenceVersion = "argus.dev/browser-locator-validation/v1"
	// PolicyVersion identifies the discriminating six-run policy.
	PolicyVersion = "argus.dev/validation-policy/browser-stable-locator/v1"
)

// Phase names the exact source variant used for a browser execution attempt.
type Phase string

const (
	// Original runs the unchanged source.
	Original Phase = "original"
	// Candidate runs the proposed locator edit.
	Candidate Phase = "candidate"
	// NegativeControl runs a deliberately impossible locator.
	NegativeControl Phase = "negative-control"
)

// Status distinguishes a complete proof from a trustworthy policy mismatch.
type Status string

const (
	// Validated means every required run matched the policy.
	Validated Status = "validated"
	// Rejected means one repeated phase consistently contradicted the policy.
	Rejected Status = "rejected"
)

// Source binds all materialized variants and verified restoration.
type Source struct {
	Path            string `json:"path"`
	OriginalSHA256  string `json:"originalSha256"`
	CandidateSHA256 string `json:"candidateSha256"`
	NegativeSHA256  string `json:"negativeSha256"`
	RestoredSHA256  string `json:"restoredSha256"`
	NegativeLocator string `json:"negativeLocator"`
}

// Run retains one artifact-verified browser attempt and its source preimage.
type Run struct {
	Phase        Phase           `json:"phase"`
	Repeat       int             `json:"repeat"`
	SourceSHA256 string          `json:"sourceSha256"`
	Attempt      browser.Attempt `json:"attempt"`
}

// Evidence is one immutable successful proof or trustworthy rejection prefix.
// External artifact bytes must be retained by the caller; references alone do
// not guarantee that the bytes remain available after validation.
type Evidence struct {
	APIVersion    string                  `json:"apiVersion"`
	PolicyVersion string                  `json:"policyVersion"`
	ValidationID  string                  `json:"validationId"`
	Status        Status                  `json:"status"`
	Reason        string                  `json:"reason,omitempty"`
	Proposal      browserlocator.Proposal `json:"proposal"`
	Baseline      browser.Attempt         `json:"baseline"`
	Failed        browser.Attempt         `json:"failed"`
	Source        Source                  `json:"source"`
	Runs          []Run                   `json:"runs"`
}

// RejectedError carries durable policy evidence while preserving error identity.
type RejectedError struct{ Evidence Evidence }

func (err *RejectedError) Error() string {
	return fmt.Sprintf("%s: %s", adaptation.ErrValidationRejected, err.Evidence.Reason)
}
func (err *RejectedError) Unwrap() error { return adaptation.ErrValidationRejected }

// ErrConflict means an immutable validation ID was reused with different evidence.
var ErrConflict = errors.New("conflicting browser validation evidence")

// ErrNotFound means no validation has the requested ID.
var ErrNotFound = errors.New("browser validation evidence not found")
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// DeriveID binds the proposal and all source variants to this policy revision.
func DeriveID(proposalID, failedID, candidateSHA, negativeSHA string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{PolicyVersion, proposalID, failedID, candidateSHA, negativeSHA}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// ValidateEvidence checks the complete ordered proof or the first consistent
// policy mismatch. Infrastructure errors and disagreement have no valid form.
func ValidateEvidence(value Evidence) error {
	if err := validateEnvelope(value); err != nil {
		return err
	}
	count := 6
	if value.Status == Rejected {
		switch value.Reason {
		case "original-passed":
			count = 2
		case "candidate-locator-failed":
			count = 4
		case "negative-control-passed":
			count = 6
		default:
			return fmt.Errorf("%w: browser rejection reason", adaptation.ErrInvalid)
		}
	} else if value.Status != Validated || value.Reason != "" {
		return fmt.Errorf("%w: browser validation status", adaptation.ErrInvalid)
	}

	return validateRuns(value, count)
}

func validateEnvelope(value Evidence) error {
	if value.APIVersion != EvidenceVersion || value.PolicyVersion != PolicyVersion ||
		browserlocator.ValidateProposal(value.Proposal) != nil ||
		browser.ValidateAttempt(value.Baseline) != nil || browser.ValidateAttempt(value.Failed) != nil ||
		value.Baseline.Request.AttemptID != value.Proposal.BaselineID ||
		value.Failed.Request.AttemptID != value.Proposal.FailedID ||
		!sameContext(value.Baseline.Request, value.Failed.Request, value.Proposal) ||
		value.Baseline.Result.Results[0].Outcome != execution.TestPassed ||
		value.Failed.Result.Results[0].Outcome != execution.TestFailed ||
		value.Failed.Result.Results[0].Failure == nil ||
		value.Failed.Result.Results[0].Failure.Code != "locator-not-found" ||
		!hasSnapshot(value.Baseline, value.Proposal.BaselineSHA256) ||
		!hasSnapshot(value.Failed, value.Proposal.FailedSHA256) ||
		value.Source.Path != value.Proposal.Edit.Path ||
		value.Source.OriginalSHA256 != value.Proposal.Edit.BeforeSHA256 ||
		!digestPattern.MatchString(value.Source.CandidateSHA256) ||
		!digestPattern.MatchString(value.Source.NegativeSHA256) ||
		value.Source.RestoredSHA256 != value.Source.OriginalSHA256 ||
		value.Source.CandidateSHA256 == value.Source.OriginalSHA256 ||
		value.Source.NegativeSHA256 == value.Source.CandidateSHA256 ||
		value.ValidationID != DeriveID(value.Proposal.ProposalID, value.Proposal.FailedID, value.Source.CandidateSHA256, value.Source.NegativeSHA256) {
		return fmt.Errorf("%w: browser validation envelope", adaptation.ErrInvalid)
	}
	control, err := browserlocator.NegativeControlLocator(value.Proposal)
	if err != nil || control != value.Source.NegativeLocator {
		return fmt.Errorf("%w: browser negative control", adaptation.ErrInvalid)
	}

	return nil
}

func validateRuns(value Evidence, count int) error {
	if len(value.Runs) != count {
		return fmt.Errorf("%w: browser run count", adaptation.ErrInvalid)
	}
	phases := []Phase{Original, Candidate, NegativeControl}
	hashes := []string{value.Source.OriginalSHA256, value.Source.CandidateSHA256, value.Source.NegativeSHA256}
	expected := []execution.TestOutcome{execution.TestFailed, execution.TestPassed, execution.TestFailed}
	for i, run := range value.Runs {
		group := i / 2
		want := expected[group]
		if value.Status == Rejected && group == count/2-1 {
			if want == execution.TestFailed {
				want = execution.TestPassed
			} else {
				want = execution.TestFailed
			}
		}
		if run.Phase != phases[group] || run.Repeat != i%2+1 || run.SourceSHA256 != hashes[group] ||
			browser.ValidateAttempt(run.Attempt) != nil ||
			!sameContext(value.Failed.Request, run.Attempt.Request, value.Proposal) ||
			run.Attempt.Request.AttemptID != attemptID(value.ValidationID, run.Phase, run.Repeat) ||
			run.Attempt.Result.AdapterVersion != value.Failed.Result.AdapterVersion ||
			run.Attempt.Result.Results[0].Outcome != want ||
			(i == 0 && run.Attempt.Result.StartedAt.Before(value.Failed.Result.CompletedAt)) ||
			(i > 0 && run.Attempt.Result.StartedAt.Before(value.Runs[i-1].Attempt.Result.CompletedAt)) {
			return fmt.Errorf("%w: browser run correlation", adaptation.ErrInvalid)
		}
		if want == execution.TestFailed && run.Attempt.Result.Results[0].Failure.Code != "locator-not-found" {
			return fmt.Errorf("%w: browser failure class", adaptation.ErrInvalid)
		}
	}

	return nil
}

func sameContext(a, b browser.Request, proposal browserlocator.Proposal) bool {
	return a.TestRepository == b.TestRepository && a.TestRevision == b.TestRevision &&
		a.Project == b.Project && a.Environment == b.Environment && a.Adapter == b.Adapter &&
		len(a.Tests) == 1 && len(b.Tests) == 1 && a.Tests[0] == b.Tests[0] &&
		a.Tests[0].SuiteKey == proposal.SuiteKey && a.Tests[0].TestKey == proposal.TestKey &&
		a.Manifest == b.Manifest && a.Stage == b.Stage
}

func attemptID(id string, phase Phase, repeat int) string {
	return fmt.Sprintf("browser-validation-%s-%s-%d", id[:16], phase, repeat)
}
