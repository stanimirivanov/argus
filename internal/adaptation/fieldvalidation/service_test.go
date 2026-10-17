package fieldvalidation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

type memoryWorkspace struct {
	data        []byte
	restores    int
	failRestore bool
}

func (w *memoryWorkspace) Read(_ context.Context, _ string) ([]byte, error) {
	return append([]byte{}, w.data...), nil
}
func (w *memoryWorkspace) WriteIfUnchanged(_ context.Context, _ string, sha string, data []byte) error {
	if digest(w.data) != sha {
		return adaptation.ErrInvalid
	}
	w.data = append([]byte{}, data...)

	return nil
}
func (w *memoryWorkspace) Restore(_ context.Context, _ string, data []byte) error {
	if w.failRestore {
		return errors.New("restore unavailable")
	}
	w.restores++
	w.data = append([]byte{}, data...)

	return nil
}

type fakeRunner struct {
	workspace        *memoryWorkspace
	calls            int
	outcomes         map[Phase][2]adaptation.ValidationOutcome
	mutate           Phase
	fail             Phase
	cancel           context.CancelFunc
	observedNegative string
}

func (r *fakeRunner) ExecuteField(ctx context.Context, request Request) (Result, error) {
	r.calls++
	if digest(r.workspace.data) != request.SourceSHA256 {
		return Result{}, adaptation.ErrInvalid
	}
	if request.Phase == NegativeControl {
		r.observedNegative = string(r.workspace.data)
	}
	if request.Phase == r.mutate {
		r.workspace.data = append(r.workspace.data, '!')
	}
	if request.Phase == r.fail {
		if r.cancel != nil {
			r.cancel()
		}
		return Result{}, context.Canceled
	}
	outcome := r.outcomes[request.Phase][request.Repeat-1]
	base := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC).Add(time.Duration(r.calls) * time.Second)
	result := Result{APIVersion: ResultVersion, ValidationID: request.ValidationID, ProposalID: request.ProposalID,
		Phase: request.Phase, Repeat: request.Repeat, SuiteKey: request.Test.SuiteKey, TestKey: request.Test.TestKey,
		AdapterID: request.Test.Adapter, AdapterVersion: "1.58.2", SourceSHA256: request.SourceSHA256,
		StartedAt: base, CompletedAt: base.Add(time.Millisecond), Outcome: outcome}
	if outcome != adaptation.ValidationPassed {
		result.Failure = &adaptation.ValidationFailure{Code: "assertion-failed", Message: "test failed"}
	}

	return result, nil
}
func proposal(direction string) (fieldrepair.Proposal, []byte) {
	name, next, role, source := "customerId", "clientId", "request-body-key", []byte("const body = { customerId: 'buyer-1' }; expect(status).toBe(200)")
	if direction == "response" {
		name, next, role, source = "orderId", "purchaseId", "response-field-access", []byte("expect(json.orderId).toBe('order-1')")
	}
	start := bytes.Index(source, []byte(name))
	test := adaptation.TestReference{Repository: catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "R_tests_1"}, Owner: "example", Name: "orders-tests"},
		Revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40)}, SuiteKey: "orders-api",
		TestKey: "field-test", Name: "field test", Adapter: "playwright", Capabilities: []string{"orders"}}
	changeRef := change.Reference{SourceRepository: catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "R_source_1"}, Owner: "example", Name: "orders"}, PullRequestNumber: 42,
		BaseRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("1", 40)},
		HeadRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("2", 40)},
		ObservedAt:   time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC),
		Trigger:      change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"}}
	method := "POST"
	status := ""
	if direction == "response" {
		method, status = "GET", "200"
	}
	value := fieldrepair.Proposal{APIVersion: fieldrepair.ProposalAPIVersion, PolicyVersion: fieldrepair.PolicyVersion,
		ProposalID: strings.Repeat("a", 64), ImpactAPIVersion: change.ImpactAPIVersion,
		ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion, Change: changeRef, Test: test,
		Classification: "INVALIDATED", Decision: "PATCH_AND_VALIDATE",
		Rename: change.FieldRename{DocumentPath: "api/orders.yaml", BaseSHA256: strings.Repeat("1", 64),
			HeadSHA256: strings.Repeat("2", 64), Method: method, Path: "/orders", OperationID: "orders",
			Capabilities: []string{"orders"}, Direction: direction, StatusCode: status, PreviousName: name, Name: next},
		AdapterID: "playwright", AdapterVersion: "1.58.2",
		Edit: adaptation.TextEdit{Path: "tests/fields.spec.mjs", BeforeSHA256: digest(source), StartByte: start,
			EndByte: start + len(name), Original: name, Replacement: next, SemanticRole: role}}

	return value, source
}
func goodOutcomes() map[Phase][2]adaptation.ValidationOutcome {
	return map[Phase][2]adaptation.ValidationOutcome{Original: {adaptation.ValidationFailed, adaptation.ValidationFailed},
		Candidate: {adaptation.ValidationPassed, adaptation.ValidationPassed}, NegativeControl: {adaptation.ValidationFailed, adaptation.ValidationFailed}}
}
func TestRepeatedFieldValidationForRequestAndResponse(t *testing.T) {
	for _, direction := range []string{"request", "response"} {
		t.Run(direction, func(t *testing.T) {
			proposal, source := proposal(direction)
			workspace := &memoryWorkspace{data: append([]byte{}, source...)}
			runner := &fakeRunner{workspace: workspace, outcomes: goodOutcomes()}
			evidence, err := NewService(workspace, runner).Validate(context.Background(), proposal)
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.Runs) != 6 || workspace.restores != 4 || !bytes.Equal(workspace.data, source) ||
				!strings.Contains(runner.observedNegative, evidence.Source.NegativeControlField) ||
				!strings.Contains(runner.observedNegative, "toBe(") {
				t.Fatalf("incomplete field proof: %+v", evidence)
			}
		})
	}
}
func TestFieldPolicyRejectionsAndRepeatDisagreement(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		phase   Phase
		outcome adaptation.ValidationOutcome
		reason  string
	}{
		{"original-passed", Original, adaptation.ValidationPassed, "original-passed"},
		{"candidate-failed", Candidate, adaptation.ValidationFailed, "candidate-failed"},
		{"negative-control-passed", NegativeControl, adaptation.ValidationPassed, "negative-control-passed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			proposal, source := proposal("request")
			workspace := &memoryWorkspace{data: append([]byte{}, source...)}
			outcomes := goodOutcomes()
			outcomes[scenario.phase] = [2]adaptation.ValidationOutcome{scenario.outcome, scenario.outcome}
			_, err := NewService(workspace, &fakeRunner{workspace: workspace, outcomes: outcomes}).Validate(context.Background(), proposal)
			var rejected *RejectedError
			if !errors.As(err, &rejected) || rejected.Evidence.Reason != scenario.reason || !bytes.Equal(workspace.data, source) {
				t.Fatalf("rejection: %v", err)
			}
			if err := ValidateRejection(rejected.Evidence); err != nil {
				t.Fatal(err)
			}
		})
	}
	proposal, source := proposal("request")
	workspace := &memoryWorkspace{data: append([]byte{}, source...)}
	outcomes := goodOutcomes()
	outcomes[Candidate] = [2]adaptation.ValidationOutcome{adaptation.ValidationPassed, adaptation.ValidationFailed}
	_, err := NewService(workspace, &fakeRunner{workspace: workspace, outcomes: outcomes}).Validate(context.Background(), proposal)
	if errors.Is(err, adaptation.ErrValidationRejected) || !errors.Is(err, adaptation.ErrUnavailable) || !bytes.Equal(workspace.data, source) {
		t.Fatalf("repeat disagreement misclassified: %v", err)
	}
}
func TestFieldCancellationMutationAndRestorationFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "mutate", "restore"} {
		t.Run(mode, func(t *testing.T) {
			proposal, source := proposal("response")
			workspace := &memoryWorkspace{data: append([]byte{}, source...)}
			runner := &fakeRunner{workspace: workspace, outcomes: goodOutcomes()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "cancel":
				runner.fail, runner.cancel = Candidate, cancel
			case "mutate":
				runner.mutate = Candidate
			case "restore":
				workspace.failRestore = true
			}
			_, err := NewService(workspace, runner).Validate(ctx, proposal)
			if err == nil || errors.Is(err, adaptation.ErrValidationRejected) {
				t.Fatalf("unsafe success or quality label: %v", err)
			}
			if mode != "restore" && !bytes.Equal(workspace.data, source) {
				t.Fatalf("source not restored after %s", mode)
			}
		})
	}
}
