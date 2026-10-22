package browsercli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	discoverycontract "github.com/stanimirivanov/argus/internal/catalog/adapters/contract/browser"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func stageJSON(t *testing.T, dir, name string, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}

	return data
}

type browserStageFixture struct{ directory, sha string }

func (f browserStageFixture) args(mode string) []string {
	return []string{"-manifest", filepath.Join(f.directory, "manifest.json"), "-bindings", filepath.Join(f.directory, "bindings.json"), "-discovery", filepath.Join(f.directory, "discovery.json"), "-schedule", filepath.Join(f.directory, "schedule.json"), "-schedule-sha256", f.sha, "-episode", "episode", "-mode", mode}
}

func browserStageInputs(t *testing.T) browserStageFixture {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "fixtures", "execution-manifest", "v1", "valid", "targeted.json"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := contracts.DecodeExecutionManifestV1(data)
	if err != nil {
		t.Fatal(err)
	}
	repo := base.Decisions[0].TestRepository
	revision := contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("b", 40)}
	catalogRef := contracts.CatalogSnapshotReference{SourceRepository: base.Catalog.SourceRepository, Revision: base.Catalog.Revision, DescriptorAPIVersion: base.Catalog.DescriptorAPIVersion}
	discovery := contracts.BrowserDiscoveryV1{APIVersion: "argus.dev/browser-discovery/v1", ObservationID: "discovery", Catalog: catalogRef, CollectorVersion: "test", CollectedAt: "2026-10-21T10:00:00Z", ExpiresAt: "2026-10-23T10:00:00Z", Complete: true, MappingComplete: true, MappingRevision: revision}
	discovery.Suites = []contracts.BrowserSuiteObservation{{TestRepository: repo, Revision: revision, SuiteKey: "shop", Cases: []contracts.BrowserCase{}}}
	discovery.Mappings = []contracts.BrowserMapping{}
	manifest := contracts.BrowserSelectionManifestV1{APIVersion: browser.ManifestVersion, PolicyVersion: "argus.dev/selection-policy/browser-mapped/v1", Change: base.Change, ChangedPaths: []string{"ui/shop.js"}, PathsComplete: true, Catalog: catalogRef, EvaluatedAt: "2026-10-22T10:00:00Z", Family: "functional-ui", Mode: "targeted", Warnings: []string{}, Decisions: []contracts.BrowserSelectionDecision{}}
	for i, key := range []string{"checkout", "receipt"} {
		outcome, remaining, reason := "RUN_REQUIRED", "none", "DIRECT_UI_MAPPING"
		if i == 1 {
			outcome, remaining, reason = "SKIP_FOR_NOW", "full-suite", "NOT_AFFECTED_IN_EARLY_STAGE"
		}
		manifest.Decisions = append(manifest.Decisions, contracts.BrowserSelectionDecision{TestRepository: repo, SuiteKey: "shop", TestKey: key, Name: key, Revision: revision, Projects: []string{"chromium-desktop", "chromium-mobile"}, Outcome: outcome, RemainingExecution: remaining, Reason: reason, MappingKeys: []string{}})
		for _, project := range []string{"chromium-desktop", "chromium-mobile"} {
			discovery.Suites[0].Cases = append(discovery.Suites[0].Cases, contracts.BrowserCase{TestKey: key, Project: project, File: "tests/shop.spec.mjs", Title: key, Tags: []string{}, Owners: []string{}})
		}
	}
	canonicalSHA, err := discoverycontract.Digest(discovery)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Evidence = &contracts.BrowserSelectionEvidenceReference{ObservationID: discovery.ObservationID, Digest: canonicalSHA, CollectorVersion: discovery.CollectorVersion, MappingRevision: revision}
	manifestBytes := stageJSON(t, dir, "manifest.json", manifest)
	manifestSum := sha256.Sum256(manifestBytes)
	stageJSON(t, dir, "discovery.json", discovery)
	bindings := contracts.BrowserExecutionBindingsV1{APIVersion: "argus.dev/browser-execution-bindings/v1", Groups: []contracts.BrowserExecutionGroupBinding{}}
	ids, required, omitted := []string{}, []string{}, []string{}
	costTests, gateTests := []contracts.PredictiveStageTestCost{}, []contracts.PredictiveAbstentionTest{}
	for _, project := range []string{"chromium-desktop", "chromium-mobile"} {
		bindings.Groups = append(bindings.Groups, contracts.BrowserExecutionGroupBinding{GroupKey: project, TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: project, Environment: "ci"})
		for i, key := range []string{"checkout", "receipt"} {
			env := "ci"
			encoded, err := json.Marshal(struct {
				Provider, Host, ProviderRepositoryID, Family, Adapter string
				Project, Environment                                  *string
				SuiteKey, TestKey                                     string
			}{repo.Provider, repo.Host, repo.ProviderRepositoryID, "functional-ui", "playwright", &project, &env, "shop", key})
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(encoded)
			id := hex.EncodeToString(sum[:])
			ids = append(ids, id)
			if i == 0 {
				required = append(required, id)
			} else {
				omitted = append(omitted, id)
			}
			costTests = append(costTests, contracts.PredictiveStageTestCost{PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: id, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 1000, Dependencies: []string{}}, DurationMS: 1000, Source: "reviewed-cold-start"})
			gateTests = append(gateTests, contracts.PredictiveAbstentionTest{ID: id, RepositoryKey: strings.Repeat("a", 64), Family: "functional-ui", Probability: 0.1, CalibrationSource: "cohort", PriorControls: 2, CohortExamples: 10, CalibrationBin: 1, BinExamples: 10, Reasons: []string{}})
		}
	}
	slices.Sort(ids)
	slices.Sort(required)
	slices.Sort(omitted)
	selected := []contracts.PredictiveScheduledTest{}
	for i, id := range required {
		selected = append(selected, contracts.PredictiveScheduledTest{ID: id, Reason: "required", StartMS: int64(i) * 1000, EndMS: int64(i+1) * 1000})
	}
	report := contracts.PredictivePortfolioV1{APIVersion: execution.ScheduleAPIVersion, BudgetMS: 2000,
		Costs:      contracts.PredictiveStageCostV1{APIVersion: "argus.dev/predictive-stage-cost/v1", ConstraintsSHA256: strings.Repeat("a", 64), Episodes: []contracts.PredictiveStageCostEpisode{{ID: "episode", ReviewID: "review", ReviewedBy: "maintainer", ReviewedAt: "2026-10-21T10:00:00Z", FeatureCutoffAt: "2026-10-22T10:00:00Z", ManifestSHA256: hex.EncodeToString(manifestSum[:]), Environments: []contracts.PredictiveStageEnvironmentCost{{PredictiveStageEnvironment: contracts.PredictiveStageEnvironment{Key: "ci", CapacityUnits: 1}, WorkUnitMS: 4000, ResourceLowerBoundMS: 4000}}, Tests: costTests, SerialDurationMS: 4000, LowerBoundMS: 4000, DependencyLowerBoundMS: 1000}}},
		Abstention: contracts.PredictiveAbstentionV1{APIVersion: "argus.dev/predictive-abstention/v1", CalibrationVersion: "argus.dev/predictive-calibration/v1", MinimumBinExamples: 1, FitEpisodeIDs: []string{"fit-a", "fit-b"}, CalibrationEpisodeIDs: []string{"calibration"}, Episodes: []contracts.PredictiveAbstentionEpisode{{ID: "episode", Mode: "eligible", EvidenceState: "calibrated", CandidateTestIDs: ids, RequiredTestIDs: []string{}, Tests: gateTests}}},
		Episodes:   []contracts.PredictivePortfolioEpisode{{ID: "episode", Mode: "portfolio", RequiredTestIDs: required, OmittedTestIDs: omitted, CoveredCapabilities: []string{"shop"}, Selected: selected, EstimatedDurationMS: 2000}}}
	if err := contracts.ValidatePredictivePortfolioV1(report); err != nil {
		t.Fatal(err)
	}
	schedule := stageJSON(t, dir, "schedule.json", report)
	sum := sha256.Sum256(schedule)
	stageJSON(t, dir, "bindings.json", bindings)

	return browserStageFixture{directory: dir, sha: hex.EncodeToString(sum[:])}
}

type fakeStageAdapter struct {
	t     *testing.T
	count int
}

func (a fakeStageAdapter) Execute(_ context.Context, r browser.Request) (browser.Result, error) {
	a.t.Helper()
	if len(r.Tests) != a.count || len(r.Candidates) != 2 || browser.ValidateStagedRequest(r) != nil {
		a.t.Fatalf("invalid request: %+v", r)
	}
	stamp := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	out := browser.Result{APIVersion: browser.ResultVersion, AttemptID: r.AttemptID, AdapterID: r.Adapter, AdapterVersion: "test", Project: r.Project, Environment: r.Environment, StartedAt: stamp, CompletedAt: stamp.Add(time.Second), Results: []execution.TestResult{}, Artifacts: []browser.Artifact{}}
	for _, test := range r.Tests {
		out.Results = append(out.Results, execution.TestResult{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: execution.TestSkipped})
	}

	return out, nil
}
func (fakeStageAdapter) VerifyArtifacts(context.Context, browser.Result) error { return nil }

func TestBrowserStagedCLIAndScopeIsolation(t *testing.T) {
	f := browserStageInputs(t)
	for _, mode := range []string{"staged", "shadow"} {
		var output bytes.Buffer
		if err := planningcli.RunBrowser(t.Context(), f.args(mode), nil, &output); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.directory, "plan.json"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		stages := []string{"relevant-now", "remaining-later"}
		if mode == "shadow" {
			stages[1] = "shadow-control"
		}
		for _, stage := range stages {
			checkBrowserCLIStage(t, f, mode, stage)
		}
	}
}

func checkBrowserCLIStage(t *testing.T, f browserStageFixture, mode, stage string) {
	t.Helper()
	count := 1
	if stage == "shadow-control" {
		count = 2
	}
	args := append(f.args(mode), "-plan", filepath.Join(f.directory, "plan.json"), "-group", "chromium-desktop", "-project", "chromium-desktop", "-environment", "ci", "-attempt-id", "browser-"+stage, "-stage", stage, "-test-repository-id", "tests-1", "-test-revision", strings.Repeat("b", 40), "--", "reviewed-adapter")
	var output bytes.Buffer
	err := Run(t.Context(), args, nil, &output, io.Discard, func([]string, io.Writer) (browser.Adapter, error) { return fakeStageAdapter{t: t, count: count}, nil })
	if !errors.Is(err, execution.ErrAttemptNotPassed) {
		t.Fatalf("incomplete result: %v", err)
	}
	attempt, err := executioncontract.DecodeRecordedBrowserAttempt(output.Bytes())
	if err != nil || attempt.Request.Project != "chromium-desktop" || attempt.Request.Staging.Schedule.SHA256 != f.sha || attempt.Request.Staging.Mode != mode {
		t.Fatalf("v2 evidence: %+v %v", attempt, err)
	}
}

func TestBrowserStagedCLIRejectsSubstitutionsBeforeAdapter(t *testing.T) {
	f := browserStageInputs(t)
	var output bytes.Buffer
	if err := planningcli.RunBrowser(t.Context(), f.args("shadow"), nil, &output); err != nil {
		t.Fatal(err)
	}
	original := slices.Clone(output.Bytes())
	for name, change := range map[string]func(*contracts.BrowserStagePlanV2){
		"project":       func(p *contracts.BrowserStagePlanV2) { p.Groups[0].Project = "other" },
		"environment":   func(p *contracts.BrowserStagePlanV2) { p.Groups[0].Environment = "other" },
		"revision":      func(p *contracts.BrowserStagePlanV2) { p.Groups[0].TestRevision.Digest = strings.Repeat("f", 40) },
		"cross-project": func(p *contracts.BrowserStagePlanV2) { p.Groups[1].RelevantNow = p.Groups[0].RelevantNow },
		"missing-later": func(p *contracts.BrowserStagePlanV2) { p.Groups[0].RemainingLater = []string{} },
		"discovery":     func(p *contracts.BrowserStagePlanV2) { p.DiscoverySHA256 = strings.Repeat("f", 64) },
		"mode":          func(p *contracts.BrowserStagePlanV2) { p.Mode = "staged" },
	} {
		t.Run(name, func(t *testing.T) {
			var plan contracts.BrowserStagePlanV2
			if err := json.Unmarshal(original, &plan); err != nil {
				t.Fatal(err)
			}
			change(&plan)
			stageJSON(t, f.directory, "plan.json", plan)
			args := append(f.args("shadow"), "-plan", filepath.Join(f.directory, "plan.json"), "-group", "chromium-desktop", "-project", "chromium-desktop", "-environment", "ci", "-attempt-id", "reject", "-stage", "relevant-now", "-test-repository-id", "tests-1", "-test-revision", strings.Repeat("b", 40), "--", "reviewed-adapter")
			var output bytes.Buffer
			err := Run(t.Context(), args, nil, &output, io.Discard, func([]string, io.Writer) (browser.Adapter, error) {
				t.Fatal("adapter opened for invalid plan")
				return nil, nil
			})
			if err == nil || output.Len() != 0 {
				t.Fatalf("invalid plan accepted: %v", err)
			}
		})
	}
}

func TestBrowserStageSourceRejectsIncompleteDiscovery(t *testing.T) {
	f := browserStageInputs(t)
	read := func(name string, value any) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(f.directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}

		return data
	}
	var manifest contracts.BrowserSelectionManifestV1
	var discovery contracts.BrowserDiscoveryV1
	var bindings contracts.BrowserExecutionBindingsV1
	var portfolio contracts.PredictivePortfolioV1
	manifestBytes := read("manifest.json", &manifest)
	discoveryBytes := read("discovery.json", &discovery)
	read("bindings.json", &bindings)
	schedule := read("schedule.json", &portfolio)
	manifestSHA, discoverySHA := sha256.Sum256(manifestBytes), sha256.Sum256(discoveryBytes)
	for name, change := range map[string]func(*contracts.BrowserDiscoveryV1){
		"partial":         func(d *contracts.BrowserDiscoveryV1) { d.Complete = false },
		"partial-mapping": func(d *contracts.BrowserDiscoveryV1) { d.MappingComplete = false },
		"missing-variant": func(d *contracts.BrowserDiscoveryV1) { d.Suites[0].Cases = d.Suites[0].Cases[1:] },
		"duplicate": func(d *contracts.BrowserDiscoveryV1) {
			d.Suites[0].Cases = append(d.Suites[0].Cases, d.Suites[0].Cases[0])
		},
		"extra-project": func(d *contracts.BrowserDiscoveryV1) {
			test := d.Suites[0].Cases[0]
			test.Project = "unbound"
			d.Suites[0].Cases = append(d.Suites[0].Cases, test)
		},
		"revision": func(d *contracts.BrowserDiscoveryV1) { d.Suites[0].Revision.Digest = strings.Repeat("f", 40) },
		"title":    func(d *contracts.BrowserDiscoveryV1) { d.Suites[0].Cases[0].Title = "different" },
	} {
		t.Run(name, func(t *testing.T) {
			var changed contracts.BrowserDiscoveryV1
			if err := json.Unmarshal(discoveryBytes, &changed); err != nil {
				t.Fatal(err)
			}
			change(&changed)
			// Exercise semantic inventory coverage independently of the CLI's
			// canonical evidence hash check, which substitutions already test.
			docs := executioncontract.BrowserStageDocuments{Manifest: manifest, Discovery: changed, Bindings: bindings, ManifestSHA256: hex.EncodeToString(manifestSHA[:]), DiscoverySHA256: hex.EncodeToString(discoverySHA[:]), DiscoveryEvidenceSHA256: manifest.Evidence.Digest}
			if _, _, err := executioncontract.BrowserStageSource(schedule, f.sha, "episode", docs); err == nil {
				t.Fatal("invalid discovered inventory accepted")
			}
		})
	}
}
