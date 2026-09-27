// Package functionalapi coordinates constrained functional API repair proposals.
package functionalapi

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

// Adapter locates a framework-specific request target without mutating source.
type Adapter interface {
	Propose(context.Context, adaptation.AdapterRequest) (adaptation.AdapterResult, error)
}

// Service proves one endpoint rename, delegates source discovery, and validates
// the returned edit before publishing a proposal.
type Service struct {
	adapter Adapter
}

// NewService creates the proposal use case.
func NewService(adapter Adapter) *Service {
	return &Service{adapter: adapter}
}

// Propose returns one reviewable edit for one immutable test revision.
func (service *Service) Propose(
	ctx context.Context,
	impact change.CapabilityImpact,
	test adaptation.TestReference,
) (adaptation.Proposal, error) {
	if service == nil || service.adapter == nil {
		return adaptation.Proposal{}, adaptation.ErrUnavailable
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return adaptation.Proposal{}, fmt.Errorf("validate capability impact: %w", err)
	}
	test = adaptation.CanonicalTestReference(test)
	if err := adaptation.ValidateTestReference(test); err != nil {
		return adaptation.Proposal{}, err
	}
	rename, err := DetectEndpointRename(impact, test.Capabilities)
	if err != nil {
		return adaptation.Proposal{}, err
	}
	proposalID, err := proposalID(impact, test, rename)
	if err != nil {
		return adaptation.Proposal{}, err
	}
	request := adaptation.AdapterRequest{
		APIVersion: adaptation.RequestAPIVersion, ProposalID: proposalID,
		Change: impact.Change, Test: test, Rename: rename,
	}
	if err := adaptation.ValidateAdapterRequest(request); err != nil {
		return adaptation.Proposal{}, err
	}
	result, err := service.adapter.Propose(ctx, request)
	if err != nil {
		return adaptation.Proposal{}, err
	}
	if err := adaptation.ValidateAdapterResult(result); err != nil {
		return adaptation.Proposal{}, fmt.Errorf("validate adaptation adapter result: %w", err)
	}
	if result.ProposalID != proposalID || result.AdapterID != test.Adapter {
		return adaptation.Proposal{}, fmt.Errorf("%w: adapter result correlation", adaptation.ErrInvalid)
	}
	if result.Outcome == "abstained" {
		return adaptation.Proposal{}, fmt.Errorf(
			"%w (%s): %s", adaptation.ErrAdapterAbstained, result.ReasonCode, result.Reason,
		)
	}
	if result.Edit == nil || result.Edit.Original != rename.PreviousPath ||
		result.Edit.Replacement != rename.Path || result.Edit.SemanticRole != "request-target" {
		return adaptation.Proposal{}, fmt.Errorf("%w: adapter proposed an unconstrained edit", adaptation.ErrInvalid)
	}
	proposal := adaptation.Proposal{
		APIVersion: adaptation.ProposalAPIVersion, PolicyVersion: adaptation.PolicyVersion,
		ProposalID: proposalID, ImpactAPIVersion: impact.APIVersion,
		ImpactAnalyzerVersion: impact.AnalyzerVersion, Change: impact.Change, Test: test,
		Classification: adaptation.ClassificationInvalidated,
		Decision:       adaptation.DecisionPatchAndValidate, Rename: rename,
		AdapterID: result.AdapterID, AdapterVersion: result.AdapterVersion, Edit: *result.Edit,
	}
	if err := adaptation.ValidateProposal(proposal); err != nil {
		return adaptation.Proposal{}, err
	}

	return proposal, nil
}

// DetectEndpointRename requires complete impact and a unique removed/added pair
// with the same method, operationId, and explicit capabilities. The unchanged
// operationId is authoritative contract identity; ambiguity always abstains.
func DetectEndpointRename(
	impact change.CapabilityImpact,
	testCapabilities []string,
) (adaptation.EndpointRename, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return adaptation.EndpointRename{}, err
	}
	if impact.Status != change.ImpactComplete {
		return adaptation.EndpointRename{}, adaptation.ErrNoAuthoritativeRename
	}
	testCapabilities = canonicalStrings(testCapabilities)
	candidates := make([]adaptation.EndpointRename, 0)
	for _, document := range impact.Documents {
		for _, removed := range document.Operations {
			if removed.Kind != change.SemanticRemoved || removed.OperationID == nil ||
				!intersects(removed.Capabilities, testCapabilities) {
				continue
			}
			for _, added := range document.Operations {
				if added.Kind != change.SemanticAdded || added.OperationID == nil ||
					removed.Method != added.Method || *removed.OperationID != *added.OperationID ||
					removed.Path == added.Path || !slices.Equal(removed.Capabilities, added.Capabilities) {
					continue
				}
				candidates = append(candidates, adaptation.CanonicalEndpointRename(adaptation.EndpointRename{
					Method: removed.Method, OperationID: *removed.OperationID,
					PreviousPath: removed.Path, Path: added.Path,
					Capabilities: removed.Capabilities,
				}))
			}
		}
	}
	slices.SortFunc(candidates, adaptation.CompareEndpointRenames)
	candidates = slices.CompactFunc(candidates, func(left, right adaptation.EndpointRename) bool {
		return adaptation.CompareEndpointRenames(left, right) == 0 &&
			slices.Equal(left.Capabilities, right.Capabilities)
	})
	if len(candidates) != 1 {
		return adaptation.EndpointRename{}, adaptation.ErrNoAuthoritativeRename
	}
	if err := adaptation.ValidateEndpointRename(candidates[0]); err != nil {
		return adaptation.EndpointRename{}, err
	}

	return candidates[0], nil
}

func proposalID(
	impact change.CapabilityImpact,
	test adaptation.TestReference,
	rename adaptation.EndpointRename,
) (string, error) {
	material := struct {
		Policy string
		Impact change.CapabilityImpact
		Test   adaptation.TestReference
		Rename adaptation.EndpointRename
	}{Policy: adaptation.PolicyVersion, Impact: impact, Test: test, Rename: rename}
	data, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("encode proposal identity: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func intersects(left, right []string) bool {
	for _, value := range left {
		if slices.Contains(right, value) {
			return true
		}
	}

	return false
}

func canonicalStrings(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)
	return slices.Compact(result)
}
