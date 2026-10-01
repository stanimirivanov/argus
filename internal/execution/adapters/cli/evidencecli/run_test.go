package evidencecli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/planning"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

func TestRunValidatesArgumentsBeforeInfrastructure(t *testing.T) {
	t.Parallel()

	if err := Run(t.Context(), nil, "", nil, &bytes.Buffer{}, nil); err == nil ||
		!strings.Contains(err.Error(), "usage: execution-evidence") {
		t.Fatalf("missing command error = %v", err)
	}
	if err := Run(
		t.Context(), []string{"shadow-report", "-selected-attempt", "selected", "-full-suite-attempt", "full"},
		"", nil, &bytes.Buffer{}, nil,
	); err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("missing database error = %v", err)
	}
	if err := Run(
		t.Context(), []string{"plan-shadow-report", "-plan", "-", "-attempt-bindings", "-"},
		"postgres://test", strings.NewReader("{}"), &bytes.Buffer{}, nil,
	); err == nil || !strings.Contains(err.Error(), "usage: execution-evidence plan-shadow-report") {
		t.Fatalf("two stdin documents error = %v", err)
	}
}

func TestRunIngestsAttemptAndProducesShadowReport(t *testing.T) {
	store := &runtimeStub{attempts: make(map[string]execution.Attempt)}
	open := func(context.Context, string) (Runtime, error) { return store, nil }
	selected := cliAttempt("selected-42", execution.StageSelected)
	document, err := executioncontract.ExportAttemptV1(selected)
	if err != nil {
		t.Fatalf("export selected attempt: %v", err)
	}
	input, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode selected attempt: %v", err)
	}
	var output bytes.Buffer
	if err := Run(
		t.Context(), []string{"ingest", "-file", "-"}, "postgres://test",
		bytes.NewReader(input), &output, open,
	); err != nil {
		t.Fatalf("ingest selected attempt: %v", err)
	}
	if !strings.Contains(output.String(), `"created": true`) {
		t.Fatalf("ingest output = %s", output.String())
	}

	fullSuite := cliAttempt("full-42", execution.StageFullSuite)
	fullSuite.Results = append(fullSuite.Results, execution.TestResult{
		SuiteKey: "orders", TestKey: "delete", Outcome: execution.TestFailed,
		Duration: time.Second,
		Failure:  &execution.Failure{Code: "assertion", Message: "expected 204"},
	})
	fullSuite.Outcome = execution.DeriveAttemptOutcome(fullSuite.Results)
	store.attempts[fullSuite.AttemptID] = execution.CanonicalAttempt(fullSuite)

	output.Reset()
	if err := Run(
		t.Context(),
		[]string{"shadow-report", "-selected-attempt", selected.AttemptID, "-full-suite-attempt", fullSuite.AttemptID},
		"postgres://test", nil, &output, open,
	); err != nil {
		t.Fatalf("produce shadow report: %v", err)
	}
	if !strings.Contains(output.String(), `"failureRecallBasisPoints": 5000`) ||
		!strings.Contains(output.String(), `"reason": "not-selected"`) {
		t.Fatalf("shadow output = %s", output.String())
	}
}

func TestRunProducesAggregatePlanShadowReport(t *testing.T) {
	store := &runtimeStub{attempts: make(map[string]execution.Attempt)}
	open := func(context.Context, string) (Runtime, error) { return store, nil }
	selected := cliAttempt("orders-selected", execution.StageSelected)
	fullSuite := cliAttempt("orders-full", execution.StageFullSuite)
	fullSuite.Results = append(fullSuite.Results, execution.TestResult{
		SuiteKey: "orders", TestKey: "list", Outcome: execution.TestPassed, Duration: time.Second,
	})
	fullSuite.Outcome = execution.DeriveAttemptOutcome(fullSuite.Results)
	payments := cliAttempt("payments-full", execution.StageFullSuite)
	payments.TestRepository = catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "payments-42",
		},
		Owner: "example", Name: "payments-tests",
	}
	payments.TestRevision = catalog.Revision{
		Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40),
	}
	payments.AdapterID = "pytest"
	payments.AdapterVersion = "9.1.0"
	store.attempts[selected.AttemptID] = selected
	store.attempts[fullSuite.AttemptID] = fullSuite
	store.attempts[payments.AttemptID] = payments

	plan := planning.Plan{
		APIVersion: planning.PlanAPIVersion, Manifest: selected.Manifest,
		Jobs: []planning.Job{
			{
				GroupKey: "orders-api", Stage: execution.StageSelected,
				TestRepository: selected.TestRepository, TestRevision: selected.TestRevision,
				Adapter: selected.AdapterID, TestCount: len(selected.Results),
			},
			{
				GroupKey: "orders-api", Stage: execution.StageFullSuite,
				TestRepository: fullSuite.TestRepository, TestRevision: fullSuite.TestRevision,
				Adapter: fullSuite.AdapterID, TestCount: len(fullSuite.Results),
			},
			{
				GroupKey: "payments-api", Stage: execution.StageFullSuite,
				TestRepository: payments.TestRepository, TestRevision: payments.TestRevision,
				Adapter: payments.AdapterID, TestCount: len(payments.Results),
			},
		},
	}
	planDocument, err := executioncontract.ExportFunctionalAPIExecutionPlanV1(plan)
	if err != nil {
		t.Fatalf("export plan: %v", err)
	}
	planData, err := json.Marshal(planDocument)
	if err != nil {
		t.Fatalf("encode plan: %v", err)
	}
	wantPlanSHA256, err := executioncontract.PlanSHA256V1(plan)
	if err != nil {
		t.Fatalf("digest plan: %v", err)
	}
	selectedID := selected.AttemptID
	bindingsDocument := contracts.ExecutionPlanAttemptBindingsV1{
		APIVersion: contracts.ExecutionPlanAttemptBindingsV1APIVersion,
		Groups: []contracts.ExecutionPlanAttemptGroupBinding{
			{
				GroupKey: "orders-api", SelectedAttemptID: &selectedID,
				FullSuiteAttemptID: fullSuite.AttemptID,
			},
			{GroupKey: "payments-api", FullSuiteAttemptID: payments.AttemptID},
		},
	}
	bindingsData, err := json.Marshal(bindingsDocument)
	if err != nil {
		t.Fatalf("encode bindings: %v", err)
	}
	temporary := t.TempDir()
	planPath := filepath.Join(temporary, "execution-plan.json")
	bindingsPath := filepath.Join(temporary, "attempt-bindings.json")
	if err := os.WriteFile(planPath, planData, 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := os.WriteFile(bindingsPath, bindingsData, 0o600); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	var output bytes.Buffer
	if err := Run(
		t.Context(),
		[]string{"plan-shadow-report", "-plan", planPath, "-attempt-bindings", bindingsPath},
		"postgres://test", nil, &output, open,
	); err != nil {
		t.Fatalf("produce plan shadow report: %v", err)
	}
	var report contracts.SelectionPlanShadowReportV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode plan shadow report: %v", err)
	}
	if report.APIVersion != shadow.PlanReportAPIVersion || report.GroupCount != 2 ||
		report.FullOnlyGroupCount != 1 || report.FailureRecallBasisPoints == nil ||
		*report.FailureRecallBasisPoints != 5000 || report.Plan.SHA256 != wantPlanSHA256 {
		t.Fatalf("plan shadow report = %+v", report)
	}
}

type runtimeStub struct {
	attempts map[string]execution.Attempt
}

func (runtime *runtimeStub) SaveExecutionAttempt(
	_ context.Context,
	attempt execution.Attempt,
) (bool, error) {
	if existing, ok := runtime.attempts[attempt.AttemptID]; ok {
		if !reflect.DeepEqual(execution.CanonicalAttempt(existing), execution.CanonicalAttempt(attempt)) {
			return false, execution.ErrConflict
		}

		return false, nil
	}
	runtime.attempts[attempt.AttemptID] = execution.CanonicalAttempt(attempt)

	return true, nil
}

func (runtime *runtimeStub) FindExecutionAttempt(
	_ context.Context,
	attemptID string,
) (execution.Attempt, error) {
	attempt, ok := runtime.attempts[attemptID]
	if !ok {
		return execution.Attempt{}, execution.ErrNotFound
	}

	return attempt, nil
}

func (runtime *runtimeStub) Close() {}

func cliAttempt(attemptID string, stage execution.Stage) execution.Attempt {
	return execution.Attempt{
		APIVersion: execution.AttemptAPIVersion, AttemptID: attemptID,
		Manifest: execution.ManifestReference{
			APIVersion: "argus.dev/execution-manifest/v1", SHA256: strings.Repeat("a", 64),
		},
		Stage: stage,
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-42",
			},
			Owner: "example", Name: "orders-tests",
		},
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40),
		},
		AdapterID: "rest-assured", AdapterVersion: "1.0.0",
		StartedAt:   time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 8, 0, 1, 0, time.UTC),
		Outcome:     execution.AttemptFailed,
		Results: []execution.TestResult{{
			SuiteKey: "orders", TestKey: "create", Outcome: execution.TestFailed,
			Duration: time.Second,
			Failure:  &execution.Failure{Code: "assertion", Message: "expected 201"},
		}},
		Artifacts: []execution.ArtifactReference{},
	}
}
