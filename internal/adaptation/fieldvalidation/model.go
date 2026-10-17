// Package fieldvalidation proves compatible JSON field repairs with repeatable, discriminating runs.
package fieldvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// Version constants identify the separate field validation wire and policy boundaries.
const (
	RequestVersion   = "argus.dev/functional-api-field-validation-request/v1"
	ResultVersion    = "argus.dev/functional-api-field-validation-result/v1"
	EvidenceVersion  = "argus.dev/field-validation-evidence/v1"
	RejectionVersion = "argus.dev/field-validation-rejection/v1"
	PolicyVersion    = "argus.dev/validation-policy/functional-api-compatible-field-rename/v1"
)

// Phase identifies which exact source variant is executed.
type Phase string

// Required phases execute in order, each with repeat ordinals one and two.
const (
	Original        Phase = "original"
	Candidate       Phase = "candidate"
	NegativeControl Phase = "negative-control"
)

// Request asks reviewed code to execute one named test and one repeat ordinal.
type Request struct {
	APIVersion   string
	ValidationID string
	ProposalID   string
	Phase        Phase
	Repeat       int
	Test         adaptation.TestReference
	SourceSHA256 string
}

// Result is untrusted framework output for one requested run.
type Result struct {
	APIVersion     string
	ValidationID   string
	ProposalID     string
	Phase          Phase
	Repeat         int
	SuiteKey       string
	TestKey        string
	AdapterID      string
	AdapterVersion string
	SourceSHA256   string
	StartedAt      time.Time
	CompletedAt    time.Time
	Outcome        adaptation.ValidationOutcome
	Failure        *adaptation.ValidationFailure
}

// Run is a normalized, correlated observation retained in evidence.
type Run struct {
	Phase        Phase
	Repeat       int
	SourceSHA256 string
	StartedAt    time.Time
	CompletedAt  time.Time
	Outcome      adaptation.ValidationOutcome
	Failure      *adaptation.ValidationFailure
}

// Source binds the three materialized variants and verified restoration.
type Source struct {
	Path                 string
	OriginalSHA256       string
	CandidateSHA256      string
	NegativeSHA256       string
	RestoredSHA256       string
	NegativeControlField string
}

// Evidence proves all six required field validation outcomes.
type Evidence struct {
	APIVersion            string
	PolicyVersion         string
	ValidationID          string
	ProposalID            string
	ProposalPolicyVersion string
	Test                  adaptation.TestReference
	AdapterID             string
	AdapterVersion        string
	Edit                  adaptation.TextEdit
	Source                Source
	Runs                  []Run
}

// Rejection retains a complete trustworthy run prefix for one policy mismatch.
type Rejection struct {
	Evidence
	RejectedPhase   Phase
	Reason          string
	ExpectedOutcome adaptation.ValidationOutcome
	ActualOutcome   adaptation.ValidationOutcome
}

// RejectedError returns portable policy evidence while preserving error classification.
type RejectedError struct{ Evidence Rejection }

func (err *RejectedError) Error() string {
	return fmt.Sprintf("%s: %s", adaptation.ErrValidationRejected, err.Evidence.Reason)
}
func (err *RejectedError) Unwrap() error { return adaptation.ErrValidationRejected }

var shaPattern = regexp.MustCompile("^[0-9a-f]{64}$")
var controlPattern = regexp.MustCompile("^__argus_negative_field_[0-9a-f]{12}$")

func validPhase(phase Phase) bool {
	return phase == Original || phase == Candidate || phase == NegativeControl
}

// ValidateRequest checks the adapter request before process execution.
func ValidateRequest(request Request) error {
	if request.APIVersion != RequestVersion || !shaPattern.MatchString(request.ValidationID) || !shaPattern.MatchString(request.ProposalID) ||
		!validPhase(request.Phase) || (request.Repeat != 1 && request.Repeat != 2) || !shaPattern.MatchString(request.SourceSHA256) {
		return fmt.Errorf("%w: field validation request", adaptation.ErrInvalid)
	}

	return adaptation.ValidateTestReference(request.Test)
}

// ValidateResult checks an untrusted adapter result before correlation.
func ValidateResult(result Result) error {
	if result.APIVersion != ResultVersion || !shaPattern.MatchString(result.ValidationID) || !shaPattern.MatchString(result.ProposalID) ||
		!validPhase(result.Phase) || (result.Repeat != 1 && result.Repeat != 2) || !shaPattern.MatchString(result.SourceSHA256) ||
		!catalog.IsLocalKey(result.SuiteKey) || !catalog.IsLocalKey(result.TestKey) || !catalog.IsLocalKey(result.AdapterID) || strings.TrimSpace(result.AdapterVersion) == "" ||
		len(result.AdapterVersion) > 127 || result.StartedAt.IsZero() || result.CompletedAt.Before(result.StartedAt) {
		return fmt.Errorf("%w: field validation result", adaptation.ErrInvalid)
	}
	switch result.Outcome {
	case adaptation.ValidationPassed:
		if result.Failure != nil {
			return fmt.Errorf("%w: passing field result has failure", adaptation.ErrInvalid)
		}
	case adaptation.ValidationFailed, adaptation.ValidationError:
		if result.Failure == nil || result.Failure.Code == "" || strings.TrimSpace(result.Failure.Message) == "" || len(result.Failure.Message) > 2000 {
			return fmt.Errorf("%w: field failure", adaptation.ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: field outcome", adaptation.ErrInvalid)
	}

	return nil
}

// DeriveID binds the immutable proposal and materialized source variants to this policy.
func DeriveID(proposalID, candidateSHA, negativeSHA, control string) string {
	data := strings.Join([]string{PolicyVersion, proposalID, candidateSHA, negativeSHA, control}, "\x00")
	sum := sha256.Sum256([]byte(data))

	return hex.EncodeToString(sum[:])
}

// ValidateEvidence requires a complete ordered six-run success proof.
func ValidateEvidence(evidence Evidence) error { return validate(evidence, 6, "") }

// ValidateRejection requires two agreeing mismatches at the first failed gate.
func ValidateRejection(rejection Rejection) error {
	count := 0
	switch rejection.Reason {
	case "original-passed":
		count = 2
		if rejection.RejectedPhase != Original || rejection.ExpectedOutcome != adaptation.ValidationFailed || rejection.ActualOutcome != adaptation.ValidationPassed {
			return adaptation.ErrInvalid
		}
	case "candidate-failed":
		count = 4
		if rejection.RejectedPhase != Candidate || rejection.ExpectedOutcome != adaptation.ValidationPassed || rejection.ActualOutcome != adaptation.ValidationFailed {
			return adaptation.ErrInvalid
		}
	case "negative-control-passed":
		count = 6
		if rejection.RejectedPhase != NegativeControl || rejection.ExpectedOutcome != adaptation.ValidationFailed || rejection.ActualOutcome != adaptation.ValidationPassed {
			return adaptation.ErrInvalid
		}
	default:
		return fmt.Errorf("%w: field rejection reason", adaptation.ErrInvalid)
	}

	return validate(rejection.Evidence, count, rejection.Reason)
}
func validate(evidence Evidence, count int, reason string) error {
	if err := validateEnvelope(evidence, reason); err != nil {
		return err
	}
	return validateRuns(evidence, count, reason)
}
func validateEnvelope(evidence Evidence, reason string) error {
	wantVersion := EvidenceVersion
	if reason != "" {
		wantVersion = RejectionVersion
	}
	if evidence.APIVersion != wantVersion || evidence.PolicyVersion != PolicyVersion || evidence.ProposalPolicyVersion != fieldrepair.PolicyVersion ||
		!shaPattern.MatchString(evidence.ProposalID) || evidence.AdapterID != evidence.Test.Adapter || evidence.AdapterVersion == "" ||
		len(evidence.AdapterVersion) > 127 || evidence.Edit.Path != evidence.Source.Path ||
		evidence.Edit.BeforeSHA256 != evidence.Source.OriginalSHA256 || !shaPattern.MatchString(evidence.Source.OriginalSHA256) ||
		!shaPattern.MatchString(evidence.Source.CandidateSHA256) || !shaPattern.MatchString(evidence.Source.NegativeSHA256) ||
		evidence.Source.RestoredSHA256 != evidence.Source.OriginalSHA256 || !controlPattern.MatchString(evidence.Source.NegativeControlField) ||
		evidence.Source.NegativeControlField != "__argus_negative_field_"+evidence.ProposalID[:12] ||
		evidence.ValidationID != DeriveID(evidence.ProposalID, evidence.Source.CandidateSHA256, evidence.Source.NegativeSHA256, evidence.Source.NegativeControlField) {
		return fmt.Errorf("%w: field validation evidence", adaptation.ErrInvalid)
	}
	if err := adaptation.ValidateTestReference(evidence.Test); err != nil {
		return err
	}
	if err := adaptation.ValidateTextEdit(evidence.Edit); err != nil {
		return err
	}
	if evidence.Edit.SemanticRole != "request-body-key" && evidence.Edit.SemanticRole != "response-field-access" {
		return fmt.Errorf("%w: field edit role", adaptation.ErrInvalid)
	}

	return nil
}
func validateRuns(evidence Evidence, count int, reason string) error {
	if len(evidence.Runs) != count {
		return fmt.Errorf("%w: field run count", adaptation.ErrInvalid)
	}
	phases := []Phase{Original, Candidate, NegativeControl}
	hashes := []string{evidence.Source.OriginalSHA256, evidence.Source.CandidateSHA256, evidence.Source.NegativeSHA256}
	wants := []adaptation.ValidationOutcome{adaptation.ValidationFailed, adaptation.ValidationPassed, adaptation.ValidationFailed}
	for i, run := range evidence.Runs {
		group := i / 2
		want := wants[group]
		if reason != "" && i >= count-2 {
			if want == adaptation.ValidationFailed {
				want = adaptation.ValidationPassed
			} else {
				want = adaptation.ValidationFailed
			}
		}
		if run.Phase != phases[group] || run.Repeat != i%2+1 || run.SourceSHA256 != hashes[group] || run.Outcome != want ||
			run.StartedAt.IsZero() || run.CompletedAt.Before(run.StartedAt) ||
			(i > 0 && run.StartedAt.Before(evidence.Runs[i-1].CompletedAt)) {
			return fmt.Errorf("%w: field run sequence", adaptation.ErrInvalid)
		}
		if run.Outcome == adaptation.ValidationPassed && run.Failure != nil {
			return fmt.Errorf("%w: passing field run failure", adaptation.ErrInvalid)
		}
		if run.Outcome == adaptation.ValidationFailed && (run.Failure == nil || run.Failure.Code == "" || strings.TrimSpace(run.Failure.Message) == "") {
			return fmt.Errorf("%w: failed field run diagnostics", adaptation.ErrInvalid)
		}
	}

	return nil
}
