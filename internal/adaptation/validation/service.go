// Package validation proves constrained repair candidates in a disposable workspace.
package validation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

// Workspace owns source access inside an explicitly disposable checkout.
type Workspace interface {
	Read(context.Context, string) ([]byte, error)
	WriteIfUnchanged(context.Context, string, string, []byte) error
	Restore(context.Context, string, []byte) error
}

// Runner executes exactly one validation phase in the current workspace state.
type Runner interface {
	Execute(context.Context, adaptation.ValidationRequest) (adaptation.ValidationAdapterResult, error)
}

// Service applies only the proposed request-target span and a deterministic
// negative control, restoring the original bytes after every execution.
type Service struct {
	workspace Workspace
	runner    Runner
}

// NewService creates the three-phase validation use case.
func NewService(workspace Workspace, runner Runner) *Service {
	return &Service{workspace: workspace, runner: runner}
}

// Validate requires original failure, candidate success, and negative-control
// failure before returning evidence. It restores original source on all
// mutation paths; a restoration failure prevents evidence publication.
func (service *Service) Validate(
	ctx context.Context,
	proposal adaptation.Proposal,
) (adaptation.ValidationEvidence, error) {
	if service == nil || service.workspace == nil || service.runner == nil {
		return adaptation.ValidationEvidence{}, adaptation.ErrUnavailable
	}
	if err := adaptation.ValidateProposal(proposal); err != nil {
		return adaptation.ValidationEvidence{}, err
	}
	material, err := service.prepare(ctx, proposal)
	if err != nil {
		return adaptation.ValidationEvidence{}, err
	}
	runs, err := service.executePhases(ctx, proposal, material)
	if err != nil {
		return adaptation.ValidationEvidence{}, err
	}
	restoredSHA, err := service.verifyRestored(ctx, proposal.Edit.Path, material.originalSHA)
	if err != nil {
		return adaptation.ValidationEvidence{}, err
	}

	return buildEvidence(proposal, material, runs, restoredSHA)
}

type validationMaterial struct {
	original     []byte
	candidate    []byte
	negative     []byte
	originalSHA  string
	candidateSHA string
	negativeSHA  string
	negativePath string
	validationID string
}

type phaseRuns struct {
	original  adaptation.ValidationAdapterResult
	candidate adaptation.ValidationAdapterResult
	negative  adaptation.ValidationAdapterResult
}

func (service *Service) prepare(
	ctx context.Context,
	proposal adaptation.Proposal,
) (validationMaterial, error) {
	original, err := service.workspace.Read(ctx, proposal.Edit.Path)
	if err != nil {
		return validationMaterial{}, fmt.Errorf("read proposed source: %w", err)
	}
	if len(original) > adaptation.MaxSourceBytes {
		return validationMaterial{}, fmt.Errorf("%w: source exceeds validation bound", adaptation.ErrInvalid)
	}
	originalSHA := digest(original)
	if originalSHA != proposal.Edit.BeforeSHA256 {
		return validationMaterial{}, fmt.Errorf("%w: source preimage digest", adaptation.ErrInvalid)
	}
	candidate, err := replaceSpan(original, proposal.Edit, proposal.Edit.Replacement)
	if err != nil {
		return validationMaterial{}, err
	}
	negativePath := "/__argus_negative_control__/" + proposal.ProposalID[:16]
	negative, err := replaceSpan(original, proposal.Edit, negativePath)
	if err != nil {
		return validationMaterial{}, err
	}
	if len(candidate) > adaptation.MaxSourceBytes || len(negative) > adaptation.MaxSourceBytes {
		return validationMaterial{}, fmt.Errorf("%w: patched source exceeds validation bound", adaptation.ErrInvalid)
	}
	candidateSHA, negativeSHA := digest(candidate), digest(negative)
	validationID, err := validationID(proposal, candidateSHA, negativeSHA, negativePath)
	if err != nil {
		return validationMaterial{}, err
	}

	return validationMaterial{
		original: original, candidate: candidate, negative: negative,
		originalSHA: originalSHA, candidateSHA: candidateSHA, negativeSHA: negativeSHA,
		negativePath: negativePath, validationID: validationID,
	}, nil
}

func (service *Service) executePhases(
	ctx context.Context,
	proposal adaptation.Proposal,
	material validationMaterial,
) (phaseRuns, error) {
	originalRun, err := service.runOriginal(
		ctx, proposal, material.validationID, material.original, material.originalSHA,
	)
	if err != nil {
		return phaseRuns{}, err
	}
	if err := requireOutcome("original", originalRun.Outcome, adaptation.ValidationFailed); err != nil {
		return phaseRuns{}, err
	}
	candidateRun, err := service.runVariant(
		ctx, proposal, material.validationID, adaptation.ValidationCandidate,
		material.original, material.originalSHA, material.candidate, material.candidateSHA,
	)
	if err != nil {
		return phaseRuns{}, err
	}
	if err := requireOutcome("candidate", candidateRun.Outcome, adaptation.ValidationPassed); err != nil {
		return phaseRuns{}, err
	}
	negativeRun, err := service.runVariant(
		ctx, proposal, material.validationID, adaptation.ValidationNegativeControl,
		material.original, material.originalSHA, material.negative, material.negativeSHA,
	)
	if err != nil {
		return phaseRuns{}, err
	}
	if err := requireOutcome("negative-control", negativeRun.Outcome, adaptation.ValidationFailed); err != nil {
		return phaseRuns{}, err
	}
	if candidateRun.AdapterVersion != originalRun.AdapterVersion ||
		negativeRun.AdapterVersion != originalRun.AdapterVersion {
		return phaseRuns{}, fmt.Errorf("%w: adapter version changed between phases", adaptation.ErrInvalid)
	}

	return phaseRuns{original: originalRun, candidate: candidateRun, negative: negativeRun}, nil
}

func (service *Service) verifyRestored(ctx context.Context, path string, originalSHA string) (string, error) {
	restored, err := service.workspace.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("verify restored source: %w", err)
	}
	restoredSHA := digest(restored)
	if restoredSHA != originalSHA {
		return "", fmt.Errorf("%w: source was not restored", adaptation.ErrInvalid)
	}

	return restoredSHA, nil
}

func buildEvidence(
	proposal adaptation.Proposal,
	material validationMaterial,
	runs phaseRuns,
	restoredSHA string,
) (adaptation.ValidationEvidence, error) {
	evidence := adaptation.CanonicalValidationEvidence(adaptation.ValidationEvidence{
		APIVersion:    adaptation.ValidationEvidenceAPIVersion,
		PolicyVersion: adaptation.ValidationPolicyVersion, ValidationID: material.validationID,
		ProposalID: proposal.ProposalID, ProposalPolicyVersion: proposal.PolicyVersion,
		Test: proposal.Test, AdapterID: proposal.AdapterID, AdapterVersion: runs.original.AdapterVersion,
		Edit: proposal.Edit,
		Source: adaptation.ValidationSourceEvidence{
			Path: proposal.Edit.Path, OriginalSHA256: material.originalSHA,
			CandidateSHA256: material.candidateSHA, NegativeSHA256: material.negativeSHA,
			RestoredSHA256: restoredSHA, NegativeControlPath: material.negativePath,
		},
		Runs: []adaptation.ValidationRun{
			normalizeRun(runs.original), normalizeRun(runs.candidate), normalizeRun(runs.negative),
		},
	})
	if err := adaptation.ValidateValidationEvidence(evidence); err != nil {
		return adaptation.ValidationEvidence{}, err
	}

	return evidence, nil
}

func requireOutcome(label string, actual adaptation.ValidationOutcome, expected adaptation.ValidationOutcome) error {
	if actual != expected {
		return fmt.Errorf("%w: %s test outcome is %s", adaptation.ErrValidationRejected, label, actual)
	}

	return nil
}

func (service *Service) runOriginal(
	ctx context.Context,
	proposal adaptation.Proposal,
	validationID string,
	original []byte,
	originalSHA string,
) (adaptation.ValidationAdapterResult, error) {
	result, runErr := service.run(ctx, proposal, validationID, adaptation.ValidationOriginal, originalSHA)
	restoreContext := context.WithoutCancel(ctx)
	current, readErr := service.workspace.Read(restoreContext, proposal.Edit.Path)
	if readErr == nil && digest(current) == originalSHA {
		if runErr != nil {
			return adaptation.ValidationAdapterResult{}, runErr
		}
		return result, nil
	}
	restoreErr := service.workspace.Restore(restoreContext, proposal.Edit.Path, original)
	if restoreErr != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("restore original source after original run: %w", restoreErr)
	}
	if readErr != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("verify original source after original run: %w", readErr)
	}

	return adaptation.ValidationAdapterResult{}, fmt.Errorf("%w: adapter mutated source during original run", adaptation.ErrInvalid)
}

func (service *Service) runVariant(
	ctx context.Context,
	proposal adaptation.Proposal,
	validationID string,
	phase adaptation.ValidationPhase,
	original []byte,
	originalSHA string,
	variant []byte,
	variantSHA string,
) (adaptation.ValidationAdapterResult, error) {
	if err := service.workspace.WriteIfUnchanged(ctx, proposal.Edit.Path, originalSHA, variant); err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("materialize %s source: %w", phase, err)
	}
	result, runErr := service.run(ctx, proposal, validationID, phase, variantSHA)
	restoreContext := context.WithoutCancel(ctx)
	current, readErr := service.workspace.Read(restoreContext, proposal.Edit.Path)
	variantUnchanged := readErr == nil && digest(current) == variantSHA
	restoreErr := service.workspace.Restore(restoreContext, proposal.Edit.Path, original)
	if restoreErr != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("restore original source after %s: %w", phase, restoreErr)
	}
	restored, verifyErr := service.workspace.Read(restoreContext, proposal.Edit.Path)
	if verifyErr != nil || digest(restored) != originalSHA {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("%w: source restoration after %s", adaptation.ErrInvalid, phase)
	}
	if readErr != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("verify %s source after execution: %w", phase, readErr)
	}
	if !variantUnchanged {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("%w: adapter mutated source during %s", adaptation.ErrInvalid, phase)
	}
	if runErr != nil {
		return adaptation.ValidationAdapterResult{}, runErr
	}

	return result, nil
}

func (service *Service) run(
	ctx context.Context,
	proposal adaptation.Proposal,
	validationID string,
	phase adaptation.ValidationPhase,
	sourceSHA string,
) (adaptation.ValidationAdapterResult, error) {
	request := adaptation.ValidationRequest{
		APIVersion: adaptation.ValidationRequestAPIVersion, ValidationID: validationID,
		ProposalID: proposal.ProposalID, Phase: phase, Test: proposal.Test, SourceSHA256: sourceSHA,
	}
	if err := adaptation.ValidateValidationRequest(request); err != nil {
		return adaptation.ValidationAdapterResult{}, err
	}
	result, err := service.runner.Execute(ctx, request)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, err
	}
	if err := adaptation.ValidateValidationAdapterResult(result); err != nil {
		return adaptation.ValidationAdapterResult{}, err
	}
	if result.ValidationID != request.ValidationID || result.ProposalID != request.ProposalID ||
		result.Phase != request.Phase || result.SuiteKey != request.Test.SuiteKey ||
		result.TestKey != request.Test.TestKey || result.AdapterID != request.Test.Adapter ||
		result.SourceSHA256 != request.SourceSHA256 {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("%w: validation result correlation", adaptation.ErrInvalid)
	}

	return result, nil
}

func replaceSpan(original []byte, edit adaptation.TextEdit, replacement string) ([]byte, error) {
	if edit.StartByte < 0 || edit.EndByte > len(original) || edit.StartByte >= edit.EndByte ||
		!bytes.Equal(original[edit.StartByte:edit.EndByte], []byte(edit.Original)) {
		return nil, fmt.Errorf("%w: proposal source span", adaptation.ErrInvalid)
	}
	result := make([]byte, 0, len(original)-len(edit.Original)+len(replacement))
	result = append(result, original[:edit.StartByte]...)
	result = append(result, replacement...)
	result = append(result, original[edit.EndByte:]...)

	return result, nil
}

func validationID(
	proposal adaptation.Proposal,
	candidateSHA string,
	negativeSHA string,
	negativePath string,
) (string, error) {
	material := struct {
		Policy       string
		ProposalID   string
		CandidateSHA string
		NegativeSHA  string
		NegativePath string
	}{
		Policy: adaptation.ValidationPolicyVersion, ProposalID: proposal.ProposalID,
		CandidateSHA: candidateSHA, NegativeSHA: negativeSHA, NegativePath: negativePath,
	}
	data, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("encode validation identity: %w", err)
	}

	return digest(data), nil
}

func normalizeRun(result adaptation.ValidationAdapterResult) adaptation.ValidationRun {
	return adaptation.ValidationRun{
		Phase: result.Phase, SourceSHA256: result.SourceSHA256,
		StartedAt: result.StartedAt, CompletedAt: result.CompletedAt,
		Outcome: result.Outcome, Failure: cloneFailure(result.Failure),
	}
}

func cloneFailure(failure *adaptation.ValidationFailure) *adaptation.ValidationFailure {
	if failure == nil {
		return nil
	}
	cloned := *failure

	return &cloned
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}
