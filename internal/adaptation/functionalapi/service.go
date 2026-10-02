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
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/change"
)

// Adapter locates a framework-specific request target without mutating source.
type Adapter interface {
	Propose(context.Context, endpointrepair.AdapterRequest) (endpointrepair.AdapterResult, error)
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
) (endpointrepair.Proposal, error) {
	if service == nil || service.adapter == nil {
		return endpointrepair.Proposal{}, adaptation.ErrUnavailable
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return endpointrepair.Proposal{}, fmt.Errorf("validate capability impact: %w", err)
	}
	test = adaptation.CanonicalTestReference(test)
	if err := adaptation.ValidateTestReference(test); err != nil {
		return endpointrepair.Proposal{}, err
	}
	rename, err := DetectEndpointRename(impact, test.Capabilities)
	if err != nil {
		return endpointrepair.Proposal{}, err
	}
	proposalID, err := proposalID(impact, test, rename)
	if err != nil {
		return endpointrepair.Proposal{}, err
	}
	request := endpointrepair.AdapterRequest{
		APIVersion: endpointrepair.RequestAPIVersion, ProposalID: proposalID,
		Change: impact.Change, Test: test, Rename: rename,
	}
	if err := endpointrepair.ValidateAdapterRequest(request); err != nil {
		return endpointrepair.Proposal{}, err
	}
	result, err := service.adapter.Propose(ctx, request)
	if err != nil {
		return endpointrepair.Proposal{}, err
	}
	if err := endpointrepair.ValidateAdapterResult(result); err != nil {
		return endpointrepair.Proposal{}, fmt.Errorf("validate adaptation adapter result: %w", err)
	}
	if result.ProposalID != proposalID || result.AdapterID != test.Adapter {
		return endpointrepair.Proposal{}, fmt.Errorf("%w: adapter result correlation", adaptation.ErrInvalid)
	}
	if result.Outcome == "abstained" {
		return endpointrepair.Proposal{}, fmt.Errorf(
			"%w (%s): %s", endpointrepair.ErrAdapterAbstained, result.ReasonCode, result.Reason,
		)
	}
	if result.Edit == nil || result.Edit.Original != rename.PreviousPath ||
		result.Edit.Replacement != rename.Path || result.Edit.SemanticRole != "request-target" {
		return endpointrepair.Proposal{}, fmt.Errorf("%w: adapter proposed an unconstrained edit", adaptation.ErrInvalid)
	}
	proposal := endpointrepair.Proposal{
		APIVersion: endpointrepair.ProposalAPIVersion, PolicyVersion: endpointrepair.PolicyVersion,
		ProposalID: proposalID, ImpactAPIVersion: impact.APIVersion,
		ImpactAnalyzerVersion: impact.AnalyzerVersion, Change: impact.Change, Test: test,
		Classification: endpointrepair.ClassificationInvalidated,
		Decision:       endpointrepair.DecisionPatchAndValidate, Rename: rename,
		AdapterID: result.AdapterID, AdapterVersion: result.AdapterVersion, Edit: *result.Edit,
	}
	if err := endpointrepair.ValidateProposal(proposal); err != nil {
		return endpointrepair.Proposal{}, err
	}

	return proposal, nil
}

// DetectEndpointRename requires complete impact and a unique removed/added pair
// with the same method, operationId, and explicit capabilities. The unchanged
// operationId is authoritative contract identity; ambiguity always abstains.
func DetectEndpointRename(
	impact change.CapabilityImpact,
	testCapabilities []string,
) (endpointrepair.EndpointRename, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return endpointrepair.EndpointRename{}, err
	}
	if impact.Status != change.ImpactComplete {
		return endpointrepair.EndpointRename{}, endpointrepair.ErrNoAuthoritativeRename
	}
	testCapabilities = canonicalStrings(testCapabilities)
	candidates := make([]endpointrepair.EndpointRename, 0)
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
				candidates = append(candidates, endpointrepair.CanonicalEndpointRename(endpointrepair.EndpointRename{
					Method: removed.Method, OperationID: *removed.OperationID,
					PreviousPath: removed.Path, Path: added.Path,
					Capabilities: removed.Capabilities,
				}))
			}
		}
	}
	slices.SortFunc(candidates, endpointrepair.CompareEndpointRenames)
	candidates = slices.CompactFunc(candidates, func(left, right endpointrepair.EndpointRename) bool {
		return endpointrepair.CompareEndpointRenames(left, right) == 0 &&
			slices.Equal(left.Capabilities, right.Capabilities)
	})
	if len(candidates) != 1 {
		return endpointrepair.EndpointRename{}, endpointrepair.ErrNoAuthoritativeRename
	}
	if err := endpointrepair.ValidateEndpointRename(candidates[0]); err != nil {
		return endpointrepair.EndpointRename{}, err
	}

	return candidates[0], nil
}

func proposalID(
	impact change.CapabilityImpact,
	test adaptation.TestReference,
	rename endpointrepair.EndpointRename,
) (string, error) {
	material := struct {
		Policy string
		Impact change.CapabilityImpact
		Test   adaptation.TestReference
		Rename endpointrepair.EndpointRename
	}{Policy: endpointrepair.PolicyVersion, Impact: impact, Test: test, Rename: rename}
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
