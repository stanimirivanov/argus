package evidencecli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
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
