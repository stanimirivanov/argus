// Package fieldrepair owns proposal-only policy for compatible functional API
// JSON field renames. Validation and publication require a later class-specific
// evidence contract.
package fieldrepair

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

const (
	// RequestAPIVersion identifies the field adapter request contract.
	RequestAPIVersion = "argus.dev/functional-api-field-adaptation-request/v1"
	// ResultAPIVersion identifies the field adapter result contract.
	ResultAPIVersion = "argus.dev/functional-api-field-adaptation-result/v1"
	// ProposalAPIVersion identifies proposal-only field evidence.
	ProposalAPIVersion = "argus.dev/field-adaptation-proposal/v1"
	// PolicyVersion identifies the compatible field-rename policy.
	PolicyVersion = "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1"
)

var (
	// ErrNoAuthoritativeRename means source evidence cannot prove one safe field identity.
	ErrNoAuthoritativeRename = errors.New("no authoritative compatible field rename")
	// ErrAdapterAbstained means the reviewed adapter could not locate one safe field reference.
	ErrAdapterAbstained = errors.New("field adaptation adapter abstained")
	fieldNamePattern    = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,254}$`)
	sha256Pattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Request asks reviewed test-repository code to locate one field reference.
type Request struct {
	APIVersion string
	ProposalID string
	Change     change.Reference
	Test       adaptation.TestReference
	Rename     change.FieldRename
}

// Result is untrusted adapter output and can explicitly abstain.
type Result struct {
	APIVersion     string
	ProposalID     string
	AdapterID      string
	AdapterVersion string
	Outcome        string
	ReasonCode     string
	Reason         string
	Edit           *adaptation.TextEdit
}

// Proposal is one immutable, reviewable candidate, not a validation result.
type Proposal struct {
	APIVersion            string
	PolicyVersion         string
	ProposalID            string
	ImpactAPIVersion      string
	ImpactAnalyzerVersion string
	Change                change.Reference
	Test                  adaptation.TestReference
	Classification        string
	Decision              string
	Rename                change.FieldRename
	AdapterID             string
	AdapterVersion        string
	Edit                  adaptation.TextEdit
}

// ValidateRequest checks provenance and field proof before process execution.
func ValidateRequest(request Request) error {
	if request.APIVersion != RequestAPIVersion || !sha256Pattern.MatchString(request.ProposalID) {
		return fmt.Errorf("%w: field request envelope", adaptation.ErrInvalid)
	}
	if err := validateChange(request.Change); err != nil {
		return err
	}
	if err := adaptation.ValidateTestReference(request.Test); err != nil {
		return err
	}

	return change.ValidateFieldRename(request.Rename)
}

// ValidateResult applies shape checks before the service correlates policy.
func ValidateResult(result Result) error {
	if result.APIVersion != ResultAPIVersion || !sha256Pattern.MatchString(result.ProposalID) ||
		!catalog.IsLocalKey(result.AdapterID) || strings.TrimSpace(result.AdapterVersion) == "" ||
		len(result.AdapterVersion) > 127 {
		return fmt.Errorf("%w: field result envelope", adaptation.ErrInvalid)
	}
	switch result.Outcome {
	case "candidate":
		if result.Edit == nil || result.ReasonCode != "" || result.Reason != "" {
			return fmt.Errorf("%w: field candidate", adaptation.ErrInvalid)
		}

		return validateEdit(*result.Edit)
	case "abstained":
		if result.Edit != nil || !catalog.IsLocalKey(result.ReasonCode) ||
			strings.TrimSpace(result.Reason) == "" || len(result.Reason) > 1000 {
			return fmt.Errorf("%w: field abstention", adaptation.ErrInvalid)
		}

		return nil
	default:
		return fmt.Errorf("%w: field result outcome", adaptation.ErrInvalid)
	}
}

// ValidateProposal protects future field validation and publication consumers.
func ValidateProposal(proposal Proposal) error {
	if proposal.APIVersion != ProposalAPIVersion || proposal.PolicyVersion != PolicyVersion ||
		!sha256Pattern.MatchString(proposal.ProposalID) ||
		proposal.ImpactAPIVersion != change.ImpactAPIVersion ||
		proposal.ImpactAnalyzerVersion != change.OpenAPIAnalyzerVersion ||
		proposal.Classification != "INVALIDATED" || proposal.Decision != "PATCH_AND_VALIDATE" ||
		!catalog.IsLocalKey(proposal.AdapterID) || proposal.AdapterID != proposal.Test.Adapter ||
		strings.TrimSpace(proposal.AdapterVersion) == "" || len(proposal.AdapterVersion) > 127 {
		return fmt.Errorf("%w: field proposal envelope", adaptation.ErrInvalid)
	}
	if err := ValidateRequest(Request{
		APIVersion: RequestAPIVersion, ProposalID: proposal.ProposalID,
		Change: proposal.Change, Test: proposal.Test, Rename: proposal.Rename,
	}); err != nil {
		return err
	}

	return ValidatePolicyEdit(proposal.Rename, proposal.Edit)
}

// ValidatePolicyEdit rejects edits to values, assertion operators, or a field
// direction other than the one proved by the authoritative contract.
func ValidatePolicyEdit(rename change.FieldRename, edit adaptation.TextEdit) error {
	if err := validateEdit(edit); err != nil {
		return err
	}
	if edit.Original != rename.PreviousName || edit.Replacement != rename.Name ||
		(rename.Direction == "request" && edit.SemanticRole != "request-body-key") ||
		(rename.Direction == "response" && edit.SemanticRole != "response-field-access") {
		return fmt.Errorf("%w: field edit policy", adaptation.ErrInvalid)
	}

	return nil
}

func validateEdit(edit adaptation.TextEdit) error {
	if err := adaptation.ValidateTextEdit(edit); err != nil {
		return err
	}
	if !fieldNamePattern.MatchString(edit.Original) || !fieldNamePattern.MatchString(edit.Replacement) ||
		(edit.SemanticRole != "request-body-key" && edit.SemanticRole != "response-field-access") {
		return fmt.Errorf("%w: unsupported field edit", adaptation.ErrInvalid)
	}

	return nil
}

func validateChange(reference change.Reference) error {
	return change.ValidateSet(change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: reference.SourceRepository,
		PullRequestNumber: reference.PullRequestNumber, BaseRevision: reference.BaseRevision,
		HeadRevision: reference.HeadRevision, ObservedAt: reference.ObservedAt, Trigger: reference.Trigger,
	})
}
