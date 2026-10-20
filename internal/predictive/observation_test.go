package predictive_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
	"github.com/stanimirivanov/argus/internal/selection"
)

func fixtureInput() predictive.Input {
	source := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-1"}, Owner: "example", Name: "source"}
	tests := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "tests"}
	base := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	head := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)}
	observed := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	set := change.Set{APIVersion: change.SetAPIVersion, SourceRepository: source, PullRequestNumber: 42,
		BaseRevision: base, HeadRevision: head, ObservedAt: observed,
		Trigger: change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"},
		Files:   []change.File{{Path: "api/spec.yaml", Kind: change.KindModified, Additions: 1, PatchStatus: change.PatchUnavailable}}}
	manifest := execution.ManifestReference{APIVersion: selection.ManifestAPIVersion, SHA256: strings.Repeat("c", 64)}

	return predictive.Input{Change: set, ManifestChange: set.Reference(), Manifest: manifest,
		FeatureCutoffAt: observed, Family: catalog.TestFamilyFunctionalAPI,
		Candidates: []predictive.Candidate{
			{Repository: tests, Revision: head, SuiteKey: "api", TestKey: "b", Adapter: "playwright", Outcome: selection.OutcomeSkipForNow, Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"}},
			{Repository: tests, Revision: head, SuiteKey: "api", TestKey: "a", Adapter: "playwright", Outcome: selection.OutcomeRunRequired, Reasons: []string{"DIRECT_CAPABILITY_IMPACT"}, Capabilities: []string{"orders"}},
		},
		Control: predictive.Control{AttemptID: "full-42", Manifest: manifest, Stage: execution.StageFullSuite,
			Repository: tests, Revision: head, Adapter: "playwright", StartedAt: observed.Add(time.Minute), CompletedAt: observed.Add(2 * time.Minute),
			Results: []execution.TestResult{{SuiteKey: "api", TestKey: "b", Outcome: execution.TestFailed, Duration: time.Second}, {SuiteKey: "api", TestKey: "a", Outcome: execution.TestPassed, Duration: 2 * time.Second}}},
	}
}

func TestBuildCanonicalObservation(t *testing.T) {
	input := fixtureInput()
	first, err := predictive.Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Candidates[0], input.Candidates[1] = input.Candidates[1], input.Candidates[0]
	input.Control.Results[0], input.Control.Results[1] = input.Control.Results[1], input.Control.Results[0]
	second, err := predictive.Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Tests[0].TestKey != "a" || first.Tests[1].Outcome != execution.TestFailed {
		t.Fatalf("noncanonical or incorrect observation: %+v", first)
	}
}

func TestBuildRejectsMismatchedAndIncompleteEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*predictive.Input)
	}{
		{"change", func(i *predictive.Input) { i.ManifestChange.HeadRevision.Digest = strings.Repeat("d", 40) }},
		{"manifest digest", func(i *predictive.Input) { i.Control.Manifest.SHA256 = strings.Repeat("d", 64) }},
		{"future feature cutoff", func(i *predictive.Input) { i.FeatureCutoffAt = i.Control.StartedAt.Add(time.Second) }},
		{"missing result", func(i *predictive.Input) { i.Control.Results = i.Control.Results[:1] }},
		{"extra result", func(i *predictive.Input) {
			i.Control.Results = append(i.Control.Results, execution.TestResult{SuiteKey: "api", TestKey: "c", Outcome: execution.TestPassed})
		}},
		{"skipped control", func(i *predictive.Input) { i.Control.Results[0].Outcome = execution.TestSkipped }},
		{"wrong revision", func(i *predictive.Input) { i.Candidates[0].Revision.Digest = strings.Repeat("e", 40) }},
		{"wrong stage", func(i *predictive.Input) { i.Control.Stage = execution.StageSelected }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fixtureInput()
			tt.mutate(&input)
			if _, err := predictive.Build(input); err == nil {
				t.Fatal("invalid observation accepted")
			}
		})
	}
}

func TestBuildBrowserProjectCoverage(t *testing.T) {
	input := fixtureInput()
	input.Family = catalog.TestFamilyFunctionalUI
	input.Manifest.APIVersion = "argus.dev/browser-selection-manifest/v1"
	input.Control.Manifest = input.Manifest
	input.Control.Project, input.Control.Environment = "chromium", "staging"
	input.Candidates[0].Projects = []string{"firefox"}
	input.Control.Results = input.Control.Results[1:]
	if got, err := predictive.Build(input); err != nil || len(got.Tests) != 1 {
		t.Fatalf("browser project: %+v, %v", got, err)
	}
	input.Control.Results = append(input.Control.Results, execution.TestResult{SuiteKey: "api", TestKey: "b", Outcome: execution.TestPassed})
	if _, err := predictive.Build(input); err == nil {
		t.Fatal("unplanned project result accepted")
	}
}
