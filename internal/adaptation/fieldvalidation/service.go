package fieldvalidation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
)

const cleanupTimeout = 30 * time.Second

// Runner executes one exact field-validation request in the current workspace state.
type Runner interface {
	ExecuteField(context.Context, Request) (Result, error)
}

// Workspace owns guarded file access inside a disposable checkout.
type Workspace interface {
	Read(context.Context, string) ([]byte, error)
	WriteIfUnchanged(context.Context, string, string, []byte) error
	Restore(context.Context, string, []byte) error
}

// Service coordinates repeated runs and source restoration.
type Service struct {
	workspace Workspace
	runner    Runner
}

// NewService binds the field validation use case to its workspace and runner ports.
func NewService(workspace Workspace, runner Runner) *Service {
	return &Service{workspace: workspace, runner: runner}
}

// Validate executes each phase twice. Any disagreement is an execution error,
// never candidate-quality evidence. Every materialized variant is restored.
func (service *Service) Validate(ctx context.Context, proposal fieldrepair.Proposal) (Evidence, error) {
	if service == nil || service.workspace == nil || service.runner == nil {
		return Evidence{}, adaptation.ErrUnavailable
	}
	if err := fieldrepair.ValidateProposal(proposal); err != nil {
		return Evidence{}, err
	}
	material, err := service.prepare(ctx, proposal)
	if err != nil {
		return Evidence{}, err
	}
	evidence := Evidence{APIVersion: EvidenceVersion, PolicyVersion: PolicyVersion, ValidationID: material.id,
		ProposalID: proposal.ProposalID, ProposalPolicyVersion: proposal.PolicyVersion, Test: proposal.Test,
		AdapterID: proposal.AdapterID, AdapterVersion: proposal.AdapterVersion, Edit: proposal.Edit, Source: material.source}
	for _, phase := range material.phases() {
		observed, phaseErr := service.runPhase(ctx, proposal, material, phase, &evidence)
		if phaseErr != nil {
			return Evidence{}, phaseErr
		}
		if observed != phase.expected {
			return service.reject(proposal, phase, observed, evidence)
		}
	}
	evidence.Source.RestoredSHA256, err = service.restored(proposal.Edit.Path, material.source.OriginalSHA256)
	if err != nil {
		return Evidence{}, err
	}
	if err := ValidateEvidence(evidence); err != nil {
		return Evidence{}, err
	}

	return evidence, nil
}

type materialized struct {
	original, candidate, negative []byte
	source                        Source
	id                            string
}
type phaseSpec struct {
	phase    Phase
	bytes    []byte
	sha      string
	expected adaptation.ValidationOutcome
	reason   string
}

func (service *Service) prepare(ctx context.Context, proposal fieldrepair.Proposal) (materialized, error) {
	original, err := service.workspace.Read(ctx, proposal.Edit.Path)
	if err != nil {
		return materialized{}, err
	}
	if len(original) > adaptation.MaxSourceBytes || digest(original) != proposal.Edit.BeforeSHA256 {
		return materialized{}, fmt.Errorf("%w: field source preimage", adaptation.ErrInvalid)
	}
	candidate, err := replace(original, proposal.Edit, proposal.Edit.Replacement)
	if err != nil {
		return materialized{}, err
	}
	control := "__argus_negative_field_" + proposal.ProposalID[:12]
	negative, err := replace(original, proposal.Edit, control)
	if err != nil {
		return materialized{}, err
	}
	if len(candidate) > adaptation.MaxSourceBytes || len(negative) > adaptation.MaxSourceBytes {
		return materialized{}, fmt.Errorf("%w: field variant size", adaptation.ErrInvalid)
	}
	source := Source{Path: proposal.Edit.Path, OriginalSHA256: digest(original), CandidateSHA256: digest(candidate), NegativeSHA256: digest(negative), NegativeControlField: control}

	return materialized{original, candidate, negative, source, DeriveID(proposal.ProposalID, source.CandidateSHA256, source.NegativeSHA256, control)}, nil
}
func (material materialized) phases() []phaseSpec {
	return []phaseSpec{
		{Original, material.original, material.source.OriginalSHA256, adaptation.ValidationFailed, "original-passed"},
		{Candidate, material.candidate, material.source.CandidateSHA256, adaptation.ValidationPassed, "candidate-failed"},
		{NegativeControl, material.negative, material.source.NegativeSHA256, adaptation.ValidationFailed, "negative-control-passed"},
	}
}
func (service *Service) runPhase(ctx context.Context, proposal fieldrepair.Proposal, material materialized, phase phaseSpec, evidence *Evidence) (adaptation.ValidationOutcome, error) {
	var observed adaptation.ValidationOutcome
	for repeat := 1; repeat <= 2; repeat++ {
		result, err := service.execute(ctx, proposal, material.id, phase.phase, repeat, material.original, phase.bytes, phase.sha)
		if err != nil {
			return "", err
		}
		if result.AdapterVersion != proposal.AdapterVersion {
			return "", fmt.Errorf("%w: field adapter version drift", adaptation.ErrInvalid)
		}
		if len(evidence.Runs) > 0 && result.StartedAt.Before(evidence.Runs[len(evidence.Runs)-1].CompletedAt) {
			return "", fmt.Errorf("%w: overlapping field runs", adaptation.ErrInvalid)
		}
		if result.Outcome == adaptation.ValidationError {
			return "", fmt.Errorf("%w: field adapter execution error", adaptation.ErrUnavailable)
		}
		if repeat == 2 && result.Outcome != observed {
			return "", fmt.Errorf("%w: field repeat disagreement", adaptation.ErrUnavailable)
		}
		observed = result.Outcome
		evidence.Runs = append(evidence.Runs, Run{Phase: phase.phase, Repeat: repeat, SourceSHA256: result.SourceSHA256,
			StartedAt: result.StartedAt.UTC(), CompletedAt: result.CompletedAt.UTC(), Outcome: result.Outcome, Failure: result.Failure})
	}

	return observed, nil
}
func (service *Service) reject(proposal fieldrepair.Proposal, phase phaseSpec, observed adaptation.ValidationOutcome, evidence Evidence) (Evidence, error) {
	restored, err := service.restored(proposal.Edit.Path, evidence.Source.OriginalSHA256)
	if err != nil {
		return Evidence{}, err
	}
	evidence.Source.RestoredSHA256 = restored
	evidence.APIVersion = RejectionVersion
	rejection := Rejection{Evidence: evidence, RejectedPhase: phase.phase, Reason: phase.reason,
		ExpectedOutcome: phase.expected, ActualOutcome: observed}
	if err := ValidateRejection(rejection); err != nil {
		return Evidence{}, err
	}

	return Evidence{}, &RejectedError{Evidence: rejection}
}

func (service *Service) execute(ctx context.Context, proposal fieldrepair.Proposal, id string, phase Phase, repeat int, original, variant []byte, variantSHA string) (Result, error) {
	mutated := phase != Original
	if mutated {
		if err := service.workspace.WriteIfUnchanged(ctx, proposal.Edit.Path, digest(original), variant); err != nil {
			return Result{}, fmt.Errorf("materialize field %s: %w", phase, err)
		}
	}
	request := Request{APIVersion: RequestVersion, ValidationID: id, ProposalID: proposal.ProposalID, Phase: phase,
		Repeat: repeat, Test: proposal.Test, SourceSHA256: variantSHA}
	result, runErr := service.runner.ExecuteField(ctx, request)
	checkCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	current, checkErr := service.workspace.Read(checkCtx, proposal.Edit.Path)
	stop()
	unchanged := checkErr == nil && digest(current) == variantSHA
	if mutated || !unchanged {
		restoreCtx, stopRestore := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		restoreErr := service.workspace.Restore(restoreCtx, proposal.Edit.Path, original)
		stopRestore()
		if restoreErr != nil {
			return Result{}, fmt.Errorf("restore field source after %s run %d: %w", phase, repeat, restoreErr)
		}
	}
	if _, err := service.restored(proposal.Edit.Path, digest(original)); err != nil {
		return Result{}, err
	}
	if checkErr != nil {
		return Result{}, fmt.Errorf("verify field source: %w", checkErr)
	}
	if !unchanged {
		return Result{}, fmt.Errorf("%w: field adapter mutated source", adaptation.ErrInvalid)
	}
	if runErr != nil {
		return Result{}, runErr
	}
	if err := ValidateResult(result); err != nil {
		return Result{}, err
	}
	if result.ValidationID != request.ValidationID || result.ProposalID != request.ProposalID || result.Phase != request.Phase ||
		result.Repeat != request.Repeat || result.SuiteKey != request.Test.SuiteKey || result.TestKey != request.Test.TestKey ||
		result.AdapterID != request.Test.Adapter || result.SourceSHA256 != request.SourceSHA256 {
		return Result{}, fmt.Errorf("%w: field result correlation", adaptation.ErrInvalid)
	}

	return result, nil
}

func (service *Service) restored(path, sha string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), cleanupTimeout)
	defer stop()
	current, err := service.workspace.Read(ctx, path)
	if err != nil {
		return "", err
	}
	if digest(current) != sha {
		return "", fmt.Errorf("%w: field source restoration", adaptation.ErrInvalid)
	}

	return sha, nil
}
func replace(original []byte, edit adaptation.TextEdit, replacement string) ([]byte, error) {
	if edit.StartByte < 0 || edit.StartByte >= edit.EndByte || edit.EndByte > len(original) ||
		!bytes.Equal(original[edit.StartByte:edit.EndByte], []byte(edit.Original)) {
		return nil, fmt.Errorf("%w: field source span", adaptation.ErrInvalid)
	}
	result := make([]byte, 0, len(original)-len(edit.Original)+len(replacement))
	result = append(result, original[:edit.StartByte]...)
	result = append(result, replacement...)
	result = append(result, original[edit.EndByte:]...)

	return result, nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
