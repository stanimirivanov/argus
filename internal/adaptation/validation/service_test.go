package validation

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestValidateProvesThreePhasesAndRestoresSource(t *testing.T) {
	t.Parallel()

	proposal, source := validProposal()
	workspace := &memoryWorkspace{data: append([]byte{}, source...)}
	runner := &fakeRunner{workspace: workspace, outcomes: map[adaptation.ValidationPhase]adaptation.ValidationOutcome{
		adaptation.ValidationOriginal:        adaptation.ValidationFailed,
		adaptation.ValidationCandidate:       adaptation.ValidationPassed,
		adaptation.ValidationNegativeControl: adaptation.ValidationFailed,
	}}
	evidence, err := NewService(workspace, runner).Validate(context.Background(), proposal)
	if err != nil {
		t.Fatalf("validate proposal: %v", err)
	}
	if len(evidence.Runs) != 3 || evidence.Runs[0].Outcome != adaptation.ValidationFailed ||
		evidence.Runs[1].Outcome != adaptation.ValidationPassed ||
		evidence.Runs[2].Outcome != adaptation.ValidationFailed {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if !bytes.Equal(workspace.data, source) || workspace.restoreCount != 2 {
		t.Fatalf("source was not restored: restores=%d source=%q", workspace.restoreCount, workspace.data)
	}
	if evidence.Source.RestoredSHA256 != proposal.Edit.BeforeSHA256 ||
		evidence.Source.NegativeControlPath != "/__argus_negative_control__/"+proposal.ProposalID[:16] {
		t.Fatalf("unexpected source proof: %+v", evidence.Source)
	}
}

func TestValidateRejectsWrongPhaseOutcomesWithoutLeavingPatch(t *testing.T) {
	t.Parallel()

	tests := map[string]map[adaptation.ValidationPhase]adaptation.ValidationOutcome{
		"original passes": {
			adaptation.ValidationOriginal: adaptation.ValidationPassed,
		},
		"candidate still fails": {
			adaptation.ValidationOriginal:  adaptation.ValidationFailed,
			adaptation.ValidationCandidate: adaptation.ValidationFailed,
		},
		"negative control passes": {
			adaptation.ValidationOriginal:        adaptation.ValidationFailed,
			adaptation.ValidationCandidate:       adaptation.ValidationPassed,
			adaptation.ValidationNegativeControl: adaptation.ValidationPassed,
		},
	}
	for name, outcomes := range tests {
		name, outcomes := name, outcomes
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			proposal, source := validProposal()
			workspace := &memoryWorkspace{data: append([]byte{}, source...)}
			runner := &fakeRunner{workspace: workspace, outcomes: outcomes}
			_, err := NewService(workspace, runner).Validate(context.Background(), proposal)
			if !errors.Is(err, adaptation.ErrValidationRejected) {
				t.Fatalf("expected validation rejection, got %v", err)
			}
			if !bytes.Equal(workspace.data, source) {
				t.Fatalf("source was not restored: %q", workspace.data)
			}
		})
	}
}

func TestValidateDetectsAdapterSourceMutationAndRestores(t *testing.T) {
	t.Parallel()

	proposal, source := validProposal()
	workspace := &memoryWorkspace{data: append([]byte{}, source...)}
	runner := &fakeRunner{
		workspace: workspace,
		outcomes: map[adaptation.ValidationPhase]adaptation.ValidationOutcome{
			adaptation.ValidationOriginal:  adaptation.ValidationFailed,
			adaptation.ValidationCandidate: adaptation.ValidationPassed,
		},
		mutatePhase: adaptation.ValidationCandidate,
	}
	_, err := NewService(workspace, runner).Validate(context.Background(), proposal)
	if !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected mutation rejection, got %v", err)
	}
	if !bytes.Equal(workspace.data, source) {
		t.Fatalf("source was not restored: %q", workspace.data)
	}
}

func TestValidateRestoresSourceAfterRunnerError(t *testing.T) {
	t.Parallel()

	proposal, source := validProposal()
	runnerErr := errors.New("runner unavailable")
	workspace := &memoryWorkspace{data: append([]byte{}, source...)}
	runner := &fakeRunner{
		workspace: workspace,
		outcomes: map[adaptation.ValidationPhase]adaptation.ValidationOutcome{
			adaptation.ValidationOriginal: adaptation.ValidationFailed,
		},
		errorPhase:   adaptation.ValidationCandidate,
		executionErr: runnerErr,
	}
	_, err := NewService(workspace, runner).Validate(context.Background(), proposal)
	if !errors.Is(err, runnerErr) {
		t.Fatalf("expected runner error, got %v", err)
	}
	if !bytes.Equal(workspace.data, source) {
		t.Fatalf("source was not restored: %q", workspace.data)
	}
}

type memoryWorkspace struct {
	data         []byte
	restoreCount int
}

func (workspace *memoryWorkspace) Read(context.Context, string) ([]byte, error) {
	return append([]byte{}, workspace.data...), nil
}

func (workspace *memoryWorkspace) WriteIfUnchanged(
	_ context.Context,
	_ string,
	expectedSHA string,
	data []byte,
) error {
	if digest(workspace.data) != expectedSHA {
		return adaptation.ErrInvalid
	}
	workspace.data = append([]byte{}, data...)

	return nil
}

func (workspace *memoryWorkspace) Restore(_ context.Context, _ string, data []byte) error {
	workspace.data = append([]byte{}, data...)
	workspace.restoreCount++

	return nil
}

type fakeRunner struct {
	workspace    *memoryWorkspace
	outcomes     map[adaptation.ValidationPhase]adaptation.ValidationOutcome
	mutatePhase  adaptation.ValidationPhase
	errorPhase   adaptation.ValidationPhase
	executionErr error
	calls        int
}

func (runner *fakeRunner) Execute(
	_ context.Context,
	request adaptation.ValidationRequest,
) (adaptation.ValidationAdapterResult, error) {
	if digest(runner.workspace.data) != request.SourceSHA256 {
		return adaptation.ValidationAdapterResult{}, adaptation.ErrInvalid
	}
	if request.Phase == runner.errorPhase {
		return adaptation.ValidationAdapterResult{}, runner.executionErr
	}
	outcome := runner.outcomes[request.Phase]
	startedAt := time.Date(2026, 9, 27, 10, 0, runner.calls*2, 0, time.UTC)
	runner.calls++
	result := adaptation.ValidationAdapterResult{
		APIVersion:   adaptation.ValidationResultAPIVersion,
		ValidationID: request.ValidationID, ProposalID: request.ProposalID, Phase: request.Phase,
		SuiteKey: request.Test.SuiteKey, TestKey: request.Test.TestKey,
		AdapterID: request.Test.Adapter, AdapterVersion: "test-v1", SourceSHA256: request.SourceSHA256,
		StartedAt: startedAt, CompletedAt: startedAt.Add(time.Second), Outcome: outcome,
	}
	if outcome != adaptation.ValidationPassed {
		result.Failure = &adaptation.ValidationFailure{Code: "request-failed", Message: "request failed"}
	}
	if request.Phase == runner.mutatePhase {
		runner.workspace.data = []byte("adapter mutation")
	}

	return result, nil
}

func validProposal() (adaptation.Proposal, []byte) {
	source := []byte("const endpoint = \"/v1/orders\";\n")
	start := bytes.Index(source, []byte("/v1/orders"))
	proposalID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	proposal := adaptation.Proposal{
		APIVersion: adaptation.ProposalAPIVersion, PolicyVersion: adaptation.PolicyVersion,
		ProposalID: proposalID, ImpactAPIVersion: change.ImpactAPIVersion,
		ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-1",
				},
				Owner: "example", Name: "orders",
			},
			PullRequestNumber: 42,
			BaseRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "1111111111111111111111111111111111111111",
			},
			HeadRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "2222222222222222222222222222222222222222",
			},
			ObservedAt: time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Test: adaptation.TestReference{
			Repository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
				},
				Owner: "example", Name: "orders-tests",
			},
			Revision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "cccccccccccccccccccccccccccccccccccccccc",
			},
			SuiteKey: "orders-api", TestKey: "list-orders", Name: "Lists orders",
			Adapter: "playwright", Capabilities: []string{"list-orders"},
		},
		Classification: adaptation.ClassificationInvalidated,
		Decision:       adaptation.DecisionPatchAndValidate,
		Rename: adaptation.EndpointRename{
			Method: "GET", OperationID: "listOrders", PreviousPath: "/v1/orders",
			Path: "/v2/orders", Capabilities: []string{"list-orders"},
		},
		AdapterID: "playwright", AdapterVersion: "proposal-v1",
		Edit: adaptation.TextEdit{
			Path: "tests/orders.spec.ts", BeforeSHA256: digest(source),
			StartByte: start, EndByte: start + len("/v1/orders"), Original: "/v1/orders",
			Replacement: "/v2/orders", SemanticRole: "request-target",
		},
	}

	return proposal, source
}
