package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/execution/browsershadow"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImportBrowserManifest checks browser selection semantics and projects only
// execution-relevant fields into the browser use case.
func ImportBrowserManifest(doc contracts.BrowserSelectionManifestV1) (browser.Manifest, error) {
	if err := contracts.ValidateBrowserSelectionManifestV1(doc); err != nil {
		return browser.Manifest{}, err
	}
	if len(doc.Decisions) == 0 || (doc.Mode == "targeted" && (doc.Evidence == nil || !doc.PathsComplete)) {
		return browser.Manifest{}, browser.ErrInvalid
	}
	manifest := browser.Manifest{APIVersion: doc.APIVersion, Decisions: make([]browser.Decision, 0, len(doc.Decisions))}
	for _, d := range doc.Decisions {
		if (d.Outcome == string(selection.OutcomeRunRequired) && d.RemainingExecution != "none") || (d.Outcome == string(selection.OutcomeSkipForNow) && d.RemainingExecution != "full-suite") || (doc.Mode == "fallback" && (d.Outcome != string(selection.OutcomeRunRequired) || len(d.Projects) > 0)) || (doc.Mode == "targeted" && len(d.Projects) == 0) {
			return browser.Manifest{}, browser.ErrInvalid
		}
		manifest.Decisions = append(manifest.Decisions, browser.Decision{Repository: importExecutionRepository(d.TestRepository), SuiteKey: d.SuiteKey, TestKey: d.TestKey, Name: d.Name, Revision: importExecutionRevision(d.Revision), Projects: d.Projects, Outcome: selection.Outcome(d.Outcome)})
	}
	if err := browser.ValidateManifest(manifest); err != nil {
		return browser.Manifest{}, err
	}

	return manifest, nil
}

// ImportBrowserExecutionBindings projects reviewed coordinates into planning.
func ImportBrowserExecutionBindings(doc contracts.BrowserExecutionBindingsV1) (browserplanning.Bindings, error) {
	bindings := browserplanning.Bindings{APIVersion: doc.APIVersion, Groups: make([]browserplanning.Binding, 0, len(doc.Groups))}
	for _, g := range doc.Groups {
		bindings.Groups = append(bindings.Groups, browserplanning.Binding{GroupKey: g.GroupKey, TestRepository: importExecutionRepository(g.TestRepository), TestRevision: importExecutionRevision(g.TestRevision), Adapter: g.Adapter, Project: g.Project, Environment: g.Environment})
	}

	return bindings, nil
}

func exportBrowserBinding(b browserplanning.Binding) contracts.BrowserExecutionGroupBinding {
	return contracts.BrowserExecutionGroupBinding{GroupKey: b.GroupKey, TestRepository: repositoryReference(b.TestRepository), TestRevision: revisionReference(b.TestRevision), Adapter: b.Adapter, Project: b.Project, Environment: b.Environment}
}

// ExportBrowserExecutionPlan validates a deterministic plan for CI transport.
func ExportBrowserExecutionPlan(plan browserplanning.Plan) (contracts.BrowserExecutionPlanV1, error) {
	plan = browserplanning.CanonicalPlan(plan)
	if err := browserplanning.ValidatePlan(plan); err != nil {
		return contracts.BrowserExecutionPlanV1{}, err
	}
	doc := contracts.BrowserExecutionPlanV1{APIVersion: plan.APIVersion, Manifest: contracts.ManifestDigestReference{APIVersion: plan.Manifest.APIVersion, SHA256: plan.Manifest.SHA256}, Jobs: make([]contracts.BrowserExecutionJob, 0, len(plan.Jobs))}
	for _, j := range plan.Jobs {
		doc.Jobs = append(doc.Jobs, contracts.BrowserExecutionJob{BrowserExecutionGroupBinding: exportBrowserBinding(j.Binding), Stage: string(j.Stage), TestCount: j.TestCount})
	}
	if err := contracts.ValidateBrowserExecutionPlanV1(doc); err != nil {
		return contracts.BrowserExecutionPlanV1{}, err
	}

	return doc, nil
}

// ImportBrowserExecutionPlan validates a stored plan before comparison.
func ImportBrowserExecutionPlan(doc contracts.BrowserExecutionPlanV1) (browserplanning.Plan, error) {
	plan := browserplanning.Plan{APIVersion: doc.APIVersion, Manifest: execution.ManifestReference{APIVersion: doc.Manifest.APIVersion, SHA256: doc.Manifest.SHA256}, Jobs: make([]browserplanning.Job, 0, len(doc.Jobs))}
	for _, j := range doc.Jobs {
		g := j.BrowserExecutionGroupBinding
		plan.Jobs = append(plan.Jobs, browserplanning.Job{Binding: browserplanning.Binding{GroupKey: g.GroupKey, TestRepository: importExecutionRepository(g.TestRepository), TestRevision: importExecutionRevision(g.TestRevision), Adapter: g.Adapter, Project: g.Project, Environment: g.Environment}, Stage: execution.Stage(j.Stage), TestCount: j.TestCount})
	}
	plan = browserplanning.CanonicalPlan(plan)
	if err := browserplanning.ValidatePlan(plan); err != nil {
		return browserplanning.Plan{}, err
	}

	return plan, nil
}

// BrowserPlanSHA256 hashes the canonical browser plan contract JSON.
func BrowserPlanSHA256(plan browserplanning.Plan) (string, error) {
	doc, err := ExportBrowserExecutionPlan(plan)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encode browser plan: %w", err)
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}

// ImportBrowserPlanAttemptBindings projects explicit attempt IDs for comparison.
func ImportBrowserPlanAttemptBindings(doc contracts.BrowserPlanAttemptBindingsV1) browsershadow.Bindings {
	bindings := browsershadow.Bindings{APIVersion: doc.APIVersion, Groups: make([]browsershadow.GroupBinding, 0, len(doc.Groups))}
	for _, g := range doc.Groups {
		bindings.Groups = append(bindings.Groups, browsershadow.GroupBinding{GroupKey: g.GroupKey, SelectedAttemptID: g.SelectedAttemptID, FullSuiteAttemptID: g.FullSuiteAttemptID})
	}

	return bindings
}

// ExportBrowserPlanShadowReport validates complete browser shadow evidence.
func ExportBrowserPlanShadowReport(report browsershadow.Report) (contracts.BrowserPlanShadowReportV1, error) {
	metrics := func(m browsershadow.Metrics) contracts.BrowserPlanMetrics {
		return contracts.BrowserPlanMetrics{SelectedTestCount: m.SelectedTestCount, FullSuiteTestCount: m.FullSuiteTestCount, SelectedDurationMS: m.SelectedDurationMS, FullSuiteDurationMS: m.FullSuiteDurationMS, DurationReductionMS: m.DurationReductionMS, SelectedFailureCount: m.SelectedFailureCount, FullSuiteFailureCount: m.FullSuiteFailureCount, CaughtFullSuiteFailureCount: m.CaughtFullSuiteFailureCount, FailureRecallBasisPoints: m.FailureRecallBasisPoints}
	}
	doc := contracts.BrowserPlanShadowReportV1{APIVersion: report.APIVersion, Plan: contracts.PlanDigestReference{APIVersion: browserplanning.PlanVersion, SHA256: report.PlanSHA256}, Manifest: contracts.ManifestDigestReference{APIVersion: report.Manifest.APIVersion, SHA256: report.Manifest.SHA256}, GroupCount: report.GroupCount, GroupsWithSelection: report.GroupsWithSelection, FullOnlyGroupCount: report.FullOnlyGroupCount, BrowserPlanMetrics: metrics(report.Metrics), Groups: make([]contracts.BrowserPlanShadowGroup, 0, len(report.Groups))}
	for _, g := range report.Groups {
		misses := make([]contracts.BrowserFailureMiss, 0, len(g.MissedFailures))
		for _, m := range g.MissedFailures {
			var observed *string
			if m.SelectedOutcome != nil {
				value := string(*m.SelectedOutcome)
				observed = &value
			}
			misses = append(misses, contracts.BrowserFailureMiss{SuiteKey: m.SuiteKey, TestKey: m.TestKey, FullSuiteOutcome: string(m.FullSuiteOutcome), SelectedOutcome: observed, Reason: m.Reason})
		}
		doc.Groups = append(doc.Groups, contracts.BrowserPlanShadowGroup{BrowserExecutionGroupBinding: exportBrowserBinding(g.Binding), AdapterVersion: g.AdapterVersion, SelectedAttemptID: g.SelectedAttemptID, FullSuiteAttemptID: g.FullSuiteAttemptID, BrowserPlanMetrics: metrics(g.Metrics), MissedFailures: misses})
	}
	if err := contracts.ValidateBrowserPlanShadowReportV1(doc); err != nil {
		return contracts.BrowserPlanShadowReportV1{}, err
	}

	return doc, nil
}
