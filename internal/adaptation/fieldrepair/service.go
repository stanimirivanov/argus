package fieldrepair

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/change"
)

// Adapter locates one language-specific field reference without editing source.
type Adapter interface {
	ProposeField(context.Context, Request) (Result, error)
}

// Service correlates semantic impact, immutable field proof, and one reviewed
// framework edit before producing proposal-only evidence.
type Service struct{ adapter Adapter }

// NewService constructs the field proposal use case.
func NewService(adapter Adapter) *Service { return &Service{adapter: adapter} }

// Propose returns one immutable review candidate or abstains conservatively.
func (service *Service) Propose(
	ctx context.Context, impact change.CapabilityImpact, test adaptation.TestReference,
	rename change.FieldRename,
) (Proposal, error) {
	if service == nil || service.adapter == nil {
		return Proposal{}, adaptation.ErrUnavailable
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return Proposal{}, err
	}
	test = adaptation.CanonicalTestReference(test)
	if err := adaptation.ValidateTestReference(test); err != nil {
		return Proposal{}, err
	}
	if err := change.ValidateFieldRename(rename); err != nil {
		return Proposal{}, err
	}
	if !matchesImpact(impact, test, rename) {
		return Proposal{}, ErrNoAuthoritativeRename
	}
	identity, err := proposalID(impact, test, rename)
	if err != nil {
		return Proposal{}, err
	}
	request := Request{
		APIVersion: RequestAPIVersion, ProposalID: identity,
		Change: impact.Change, Test: test, Rename: rename,
	}
	if err := ValidateRequest(request); err != nil {
		return Proposal{}, err
	}
	result, err := service.adapter.ProposeField(ctx, request)
	if err != nil {
		return Proposal{}, err
	}
	if err := ValidateResult(result); err != nil {
		return Proposal{}, err
	}
	if result.ProposalID != identity || result.AdapterID != test.Adapter {
		return Proposal{}, fmt.Errorf("%w: field adapter correlation", adaptation.ErrInvalid)
	}
	if result.Outcome == "abstained" {
		return Proposal{}, fmt.Errorf("%w (%s): %s", ErrAdapterAbstained, result.ReasonCode, result.Reason)
	}
	if result.Edit == nil {
		return Proposal{}, fmt.Errorf("%w: missing field edit", adaptation.ErrInvalid)
	}
	proposal := Proposal{
		APIVersion: ProposalAPIVersion, PolicyVersion: PolicyVersion,
		ProposalID: identity, ImpactAPIVersion: impact.APIVersion,
		ImpactAnalyzerVersion: impact.AnalyzerVersion, Change: impact.Change,
		Test: test, Classification: "INVALIDATED", Decision: "PATCH_AND_VALIDATE",
		Rename: rename, AdapterID: result.AdapterID, AdapterVersion: result.AdapterVersion,
		Edit: *result.Edit,
	}
	if err := ValidateProposal(proposal); err != nil {
		return Proposal{}, err
	}

	return proposal, nil
}

func matchesImpact(impact change.CapabilityImpact, test adaptation.TestReference, rename change.FieldRename) bool {
	if impact.APIVersion != change.ImpactAPIVersion || impact.AnalyzerVersion != change.OpenAPIAnalyzerVersion ||
		impact.Status != change.ImpactComplete {
		return false
	}
	for _, document := range impact.Documents {
		if document.Path != rename.DocumentPath || document.Kind != change.SemanticModified {
			continue
		}
		for _, operation := range document.Operations {
			if operation.Kind != change.SemanticModified || operation.Method != rename.Method ||
				operation.Path != rename.Path || operation.OperationID == nil ||
				*operation.OperationID != rename.OperationID ||
				!slices.Equal(operation.Capabilities, rename.Capabilities) {
				continue
			}
			for _, capability := range rename.Capabilities {
				if slices.Contains(test.Capabilities, capability) {
					return true
				}
			}
		}
	}

	return false
}

func proposalID(impact change.CapabilityImpact, test adaptation.TestReference, rename change.FieldRename) (string, error) {
	data, err := json.Marshal(struct {
		Policy string
		Impact change.CapabilityImpact
		Test   adaptation.TestReference
		Rename change.FieldRename
	}{PolicyVersion, impact, test, rename})
	if err != nil {
		return "", fmt.Errorf("encode field proposal identity: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
