package scheduledplan_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/execution/planning"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

func id(n int) string { return fmt.Sprintf("%064x", n) }

func fixture() scheduledplan.Schedule {
	binding := scheduledplan.Binding{GroupKey: "api", TestRepository: catalog.Repository{
		Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests"},
		Owner:    "example", Name: "tests"},
		TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("d", 40)},
		Family:       catalog.TestFamilyFunctionalAPI, Adapter: "playwright", Environment: "ci"}
	browser := binding
	browser.GroupKey, browser.Family, browser.Project = "browser", catalog.TestFamilyFunctionalUI, "chromium"

	return scheduledplan.Schedule{Reference: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: id(90)},
		EpisodeID: "holdout", ManifestSHA256: id(91), Groups: []scheduledplan.Selection{
			{Binding: binding, Candidates: []scheduledplan.Test{
				{ID: id(1), SuiteKey: "api", TestKey: "one", Decision: selection.OutcomeRunRequired},
				{ID: id(2), SuiteKey: "api", TestKey: "two", Decision: selection.OutcomeSkipForNow},
				{ID: id(3), SuiteKey: "api", TestKey: "three", Decision: selection.OutcomeSkipForNow},
			}, RelevantNow: []string{id(3), id(1)}},
			{Binding: browser, Candidates: []scheduledplan.Test{
				{ID: id(4), SuiteKey: "ui", TestKey: "four", Decision: selection.OutcomeRunRequired},
				{ID: id(5), SuiteKey: "ui", TestKey: "five", Decision: selection.OutcomeSkipForNow},
			}, RelevantNow: []string{id(4)}},
		}}
}

func TestPartitionsAndShadowControls(t *testing.T) {
	source := fixture()
	for _, mode := range []scheduledplan.Mode{scheduledplan.Staged, scheduledplan.Shadow} {
		plan, err := scheduledplan.Build(source, mode)
		if err != nil {
			t.Fatal(err)
		}
		early, err := scheduledplan.TestsForStage(plan, source, mode, "api", execution.StageRelevantNow)
		if err != nil || len(early) != 2 || early[0].ID != id(3) || early[1].ID != id(1) {
			t.Fatalf("early order lost: %+v %v", early, err)
		}
		stage, want := execution.StageRemainingLater, 1
		if mode == scheduledplan.Shadow {
			stage, want = execution.StageShadowControl, 3
		}
		later, err := scheduledplan.TestsForStage(plan, source, mode, "api", stage)
		if err != nil || len(later) != want {
			t.Fatalf("incorrect later/control set: %+v %v", later, err)
		}
		if mode == scheduledplan.Staged && later[0].ID != id(2) {
			t.Fatal("complement was not exact")
		}
		early[0].TestKey = "changed"
		again, err := scheduledplan.TestsForStage(plan, source, mode, "api", execution.StageRelevantNow)
		if err != nil || again[0].TestKey != "three" {
			t.Fatal("stage results alias plan")
		}
	}
}

func TestStagesMustMatchCallerMode(t *testing.T) {
	source := fixture()
	for _, mode := range []scheduledplan.Mode{scheduledplan.Staged, scheduledplan.Shadow} {
		plan, err := scheduledplan.Build(source, mode)
		if err != nil {
			t.Fatal(err)
		}
		invalidStage := execution.StageShadowControl
		if mode == scheduledplan.Shadow {
			invalidStage = execution.StageRemainingLater
		}
		for _, invalid := range []execution.Stage{invalidStage, execution.StageSelected, execution.StageFullSuite, "unknown"} {
			if _, err := scheduledplan.TestsForStage(plan, source, mode, "api", invalid); !errors.Is(err, execution.ErrInvalid) {
				t.Fatalf("stage accepted: %s", invalid)
			}
		}
		if _, err := scheduledplan.TestsForStage(plan, source, mode, "missing", execution.StageRelevantNow); !errors.Is(err, execution.ErrInvalid) {
			t.Fatal("unknown group accepted")
		}
	}
}
func TestPlanCorrelationRejectsTampering(t *testing.T) {
	cases := map[string]func(*scheduledplan.Plan){
		"digest":              func(p *scheduledplan.Plan) { p.Schedule.SHA256 = id(92) },
		"version":             func(p *scheduledplan.Plan) { p.Schedule.APIVersion = "argus.dev/other/v1" },
		"episode":             func(p *scheduledplan.Plan) { p.EpisodeID = "other" },
		"manifest":            func(p *scheduledplan.Plan) { p.ManifestSHA256 = id(92) },
		"mode-downgrade":      func(p *scheduledplan.Plan) { p.Mode = scheduledplan.Staged },
		"missing-group":       func(p *scheduledplan.Plan) { p.Groups = p.Groups[:1] },
		"duplicate-group":     func(p *scheduledplan.Plan) { p.Groups[1] = p.Groups[0] },
		"missing-candidate":   func(p *scheduledplan.Plan) { p.Groups[0].Candidates = p.Groups[0].Candidates[:2] },
		"missing-later":       func(p *scheduledplan.Plan) { p.Groups[0].RemainingLater = nil },
		"overlap":             func(p *scheduledplan.Plan) { p.Groups[0].RemainingLater = []string{id(1), id(2)} },
		"duplicate":           func(p *scheduledplan.Plan) { p.Groups[0].RemainingLater = []string{id(2), id(2)} },
		"cross-group":         func(p *scheduledplan.Plan) { p.Groups[0].RemainingLater = []string{id(5)} },
		"unknown":             func(p *scheduledplan.Plan) { p.Groups[0].RemainingLater = []string{id(99)} },
		"early-order":         func(p *scheduledplan.Plan) { slices.Reverse(p.Groups[0].RelevantNow) },
		"unpin":               func(p *scheduledplan.Plan) { p.Groups[0].Candidates[0].Decision = selection.OutcomeSkipForNow },
		"revision":            func(p *scheduledplan.Plan) { p.Groups[0].TestRevision.Digest = strings.Repeat("e", 40) },
		"repository":          func(p *scheduledplan.Plan) { p.Groups[0].TestRepository.Identity.ProviderRepositoryID = "other" },
		"display-coordinates": func(p *scheduledplan.Plan) { p.Groups[0].TestRepository.Name = "other" },
		"adapter":             func(p *scheduledplan.Plan) { p.Groups[0].Adapter = "other" },
		"project":             func(p *scheduledplan.Plan) { p.Groups[1].Project = "firefox" },
		"environment":         func(p *scheduledplan.Plan) { p.Groups[1].Environment = "production" },
		"identity":            func(p *scheduledplan.Plan) { p.Groups[0].Candidates[0].TestKey = "different" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			source := fixture()
			plan, err := scheduledplan.Build(source, scheduledplan.Shadow)
			if err != nil {
				t.Fatal(err)
			}
			change(&plan)
			if err := scheduledplan.Validate(plan, source, scheduledplan.Shadow); !errors.Is(err, execution.ErrInvalid) {
				t.Fatalf("tampered plan accepted: %v", err)
			}
			if _, err := scheduledplan.TestsForStage(plan, source, scheduledplan.Shadow, "api", execution.StageShadowControl); !errors.Is(err, execution.ErrInvalid) {
				t.Fatal("invalid plan produced runnable tests")
			}
		})
	}
}

func TestSourceValidationAndBounds(t *testing.T) {
	cases := map[string]func(*scheduledplan.Schedule){
		"digest":           func(s *scheduledplan.Schedule) { s.Reference.SHA256 = "bad" },
		"episode":          func(s *scheduledplan.Schedule) { s.EpisodeID = "" },
		"manifest":         func(s *scheduledplan.Schedule) { s.ManifestSHA256 = "bad" },
		"empty":            func(s *scheduledplan.Schedule) { s.Groups = nil },
		"empty-candidates": func(s *scheduledplan.Schedule) { s.Groups[0].Candidates = nil },
		"too-many-groups": func(s *scheduledplan.Schedule) {
			s.Groups = make([]scheduledplan.Selection, execution.MaxAttemptTests+1)
		},
		"too-many-tests": func(s *scheduledplan.Schedule) {
			s.Groups[0].Candidates = make([]scheduledplan.Test, execution.MaxAttemptTests+1)
		},
		"required-deferred":       func(s *scheduledplan.Schedule) { s.Groups[0].RelevantNow = []string{id(3)} },
		"duplicate-id":            func(s *scheduledplan.Schedule) { s.Groups[0].Candidates[1].ID = id(1) },
		"duplicate-runner-key":    func(s *scheduledplan.Schedule) { s.Groups[0].Candidates[1].TestKey = "one" },
		"cross-group-id":          func(s *scheduledplan.Schedule) { s.Groups[1].Candidates[0].ID = id(1) },
		"duplicate-early":         func(s *scheduledplan.Schedule) { s.Groups[0].RelevantNow = []string{id(1), id(1)} },
		"unknown-early":           func(s *scheduledplan.Schedule) { s.Groups[0].RelevantNow = []string{id(1), id(99)} },
		"missing-decision":        func(s *scheduledplan.Schedule) { s.Groups[0].Candidates[1].Decision = "" },
		"unknown-decision":        func(s *scheduledplan.Schedule) { s.Groups[0].Candidates[1].Decision = "unknown" },
		"invalid-key":             func(s *scheduledplan.Schedule) { s.Groups[0].Candidates[0].SuiteKey = "../api" },
		"unsupported-family":      func(s *scheduledplan.Schedule) { s.Groups[0].Family = catalog.TestFamilyPerformance },
		"api-project":             func(s *scheduledplan.Schedule) { s.Groups[0].Project = "chromium" },
		"missing-browser-project": func(s *scheduledplan.Schedule) { s.Groups[1].Project = "" },
		"missing-environment":     func(s *scheduledplan.Schedule) { s.Groups[0].Environment = "" },
		"invalid-revision":        func(s *scheduledplan.Schedule) { s.Groups[0].TestRevision.Digest = "HEAD" },
		"invalid-repository":      func(s *scheduledplan.Schedule) { s.Groups[0].TestRepository.Identity.Host = "" },
		"scope-alias": func(s *scheduledplan.Schedule) {
			alias := s.Groups[0]
			alias.GroupKey = "alias"
			alias.TestRevision.Digest = strings.Repeat("f", 40)
			alias.Candidates = []scheduledplan.Test{{ID: id(8), SuiteKey: "api", TestKey: "eight", Decision: selection.OutcomeSkipForNow}}
			alias.RelevantNow = nil
			s.Groups = append(s.Groups, alias)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			source := fixture()
			change(&source)
			if _, err := scheduledplan.Build(source, scheduledplan.Staged); !errors.Is(err, execution.ErrInvalid) {
				t.Fatalf("invalid source accepted: %v", err)
			}
		})
	}
	if _, err := scheduledplan.Build(fixture(), "unknown"); !errors.Is(err, execution.ErrInvalid) {
		t.Fatal("unknown mode accepted")
	}
}

func TestCanonicalOwnershipAndEmptyPhases(t *testing.T) {
	source := fixture()
	want, err := scheduledplan.Build(source, scheduledplan.Staged)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(source.Groups)
	slices.Reverse(source.Groups[1].Candidates)
	got, err := scheduledplan.Build(source, scheduledplan.Staged)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("source ordering affected plan: %v", err)
	}
	copy := scheduledplan.Canonical(got)
	copy.Groups[0].Candidates[0].TestKey = "changed"
	copy.Groups[0].RelevantNow[0] = id(99)
	copy.Groups[0].RemainingLater[0] = id(99)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("canonical copy aliases original")
	}
	source = fixture()
	source.Groups[0].Candidates[0].Decision = selection.OutcomeSkipForNow
	for _, early := range [][]string{{}, {id(1), id(2), id(3)}} {
		source.Groups[0].RelevantNow = early
		plan, err := scheduledplan.Build(source, scheduledplan.Staged)
		if err != nil {
			t.Fatal(err)
		}
		now, err := scheduledplan.TestsForStage(plan, source, scheduledplan.Staged, "api", execution.StageRelevantNow)
		if err != nil || len(now) != len(early) || len(plan.Groups[0].RemainingLater) != 3-len(early) {
			t.Fatalf("empty phase invalid: %v", err)
		}
	}
}

func TestLegacyPlansRejectStagedValues(t *testing.T) {
	binding := fixture().Groups[0].Binding
	manifest := execution.ManifestReference{APIVersion: selection.ManifestAPIVersion, SHA256: id(91)}
	api := planning.Plan{APIVersion: planning.PlanAPIVersion, Manifest: manifest, Jobs: []planning.Job{{
		GroupKey: binding.GroupKey, Stage: execution.StageFullSuite, TestRepository: binding.TestRepository,
		TestRevision: binding.TestRevision, Adapter: binding.Adapter, TestCount: 1}}}
	ui := browserplanning.Plan{APIVersion: browserplanning.PlanVersion,
		Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: id(91)},
		Jobs: []browserplanning.Job{{Binding: browserplanning.Binding{GroupKey: "ui", TestRepository: binding.TestRepository,
			TestRevision: binding.TestRevision, Adapter: "playwright", Project: "chromium", Environment: "ci"},
			Stage: execution.StageFullSuite, TestCount: 1}}}
	if err := planning.ValidatePlan(api); err != nil {
		t.Fatal(err)
	}
	if err := browserplanning.ValidatePlan(ui); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []execution.Stage{execution.StageRelevantNow, execution.StageRemainingLater, execution.StageShadowControl} {
		api.Jobs[0].Stage, ui.Jobs[0].Stage = stage, stage
		if planning.ValidatePlan(api) == nil || browserplanning.ValidatePlan(ui) == nil {
			t.Fatalf("v1 accepted staged value %s", stage)
		}
	}
}
func TestDistinctBrowserScopesKeepIndependentPartitions(t *testing.T) {
	for _, dimension := range []string{"project", "environment"} {
		t.Run(dimension, func(t *testing.T) {
			source := fixture()
			extra := source.Groups[1]
			extra.GroupKey = "variant"
			if dimension == "project" {
				extra.Project = "firefox"
			} else {
				extra.Environment = "staging"
			}
			extra.Candidates = []scheduledplan.Test{source.Groups[1].Candidates[0]}
			extra.Candidates[0].ID = id(6)
			extra.RelevantNow = []string{id(6)}
			source.Groups = append(source.Groups, extra)
			plan, err := scheduledplan.Build(source, scheduledplan.Shadow)
			if err != nil {
				t.Fatal(err)
			}
			tests, err := scheduledplan.TestsForStage(plan, source, scheduledplan.Shadow, "variant", execution.StageShadowControl)
			if err != nil || len(tests) != 1 || tests[0].ID != id(6) {
				t.Fatalf("scope leaked candidates: %v %+v", err, tests)
			}
		})
	}
}
