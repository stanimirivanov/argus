package shadow

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/planning"
)

const (
	// PlanAttemptBindingsAPIVersion identifies explicit plan-to-attempt bindings v1.
	PlanAttemptBindingsAPIVersion = "argus.dev/execution-plan-attempt-bindings/v1"
	// PlanReportAPIVersion identifies aggregate selection shadow evidence v1.
	PlanReportAPIVersion = "argus.dev/selection-plan-shadow-report/v1"
)

// GroupAttemptBinding explicitly associates one planned group with its stored
// selected attempt, when planned, and mandatory full-suite attempt.
type GroupAttemptBinding struct {
	GroupKey           string
	SelectedAttemptID  *string
	FullSuiteAttemptID string
}

// PlanAttemptBindings names the immutable attempts used to evaluate every
// group in one execution plan.
type PlanAttemptBindings struct {
	APIVersion string
	Groups     []GroupAttemptBinding
}

// PlanGroupReport is one planned group's selected-versus-control evidence.
// Selected fields are zero and SelectedAttemptID is nil for a full-only group.
type PlanGroupReport struct {
	GroupKey                    string
	TestRepository              catalog.Repository
	TestRevision                catalog.Revision
	AdapterID                   string
	AdapterVersion              string
	SelectedAttemptID           *string
	FullSuiteAttemptID          string
	SelectedTestCount           int
	FullSuiteTestCount          int
	SelectedDurationMS          int64
	FullSuiteDurationMS         int64
	DurationReductionMS         int64
	SelectedFailureCount        int
	FullSuiteFailureCount       int
	CaughtFullSuiteFailureCount int
	FailureRecallBasisPoints    *int
	MissedFailures              []FailureMiss
}

// PlanReport aggregates every group in one immutable execution plan. Duration
// values sum normalized per-test durations; they are not CI wall-clock time.
type PlanReport struct {
	APIVersion                  string
	Plan                        planning.Reference
	Manifest                    execution.ManifestReference
	GroupCount                  int
	GroupsWithSelection         int
	FullOnlyGroupCount          int
	SelectedTestCount           int
	FullSuiteTestCount          int
	SelectedDurationMS          int64
	FullSuiteDurationMS         int64
	DurationReductionMS         int64
	SelectedFailureCount        int
	FullSuiteFailureCount       int
	CaughtFullSuiteFailureCount int
	FailureRecallBasisPoints    *int
	Groups                      []PlanGroupReport
}

// CanonicalPlanAttemptBindings deep-copies and orders group bindings.
func CanonicalPlanAttemptBindings(bindings PlanAttemptBindings) PlanAttemptBindings {
	canonical := bindings
	canonical.Groups = append([]GroupAttemptBinding{}, bindings.Groups...)
	for index, group := range canonical.Groups {
		if group.SelectedAttemptID != nil {
			value := *group.SelectedAttemptID
			canonical.Groups[index].SelectedAttemptID = &value
		}
	}
	slices.SortFunc(canonical.Groups, func(left, right GroupAttemptBinding) int {
		if left.GroupKey < right.GroupKey {
			return -1
		}
		if left.GroupKey > right.GroupKey {
			return 1
		}

		return 0
	})

	return canonical
}

// ValidatePlanAttemptBindings rejects ambiguous group or attempt identities.
func ValidatePlanAttemptBindings(bindings PlanAttemptBindings) error {
	if bindings.APIVersion != PlanAttemptBindingsAPIVersion ||
		len(bindings.Groups) > planning.MaxGroups {
		return fmt.Errorf("%w: execution plan attempt bindings envelope", execution.ErrInvalid)
	}
	groups := make(map[string]struct{}, len(bindings.Groups))
	attempts := make(map[string]struct{}, 2*len(bindings.Groups))
	for _, group := range bindings.Groups {
		if !catalog.IsLocalKey(group.GroupKey) ||
			execution.ValidateAttemptID(group.FullSuiteAttemptID) != nil {
			return fmt.Errorf("%w: execution plan attempt binding", execution.ErrInvalid)
		}
		if _, exists := groups[group.GroupKey]; exists {
			return fmt.Errorf("%w: duplicate execution plan attempt group", execution.ErrInvalid)
		}
		groups[group.GroupKey] = struct{}{}
		if err := addAttemptIdentity(attempts, group.FullSuiteAttemptID); err != nil {
			return err
		}
		if group.SelectedAttemptID != nil {
			if execution.ValidateAttemptID(*group.SelectedAttemptID) != nil {
				return fmt.Errorf("%w: selected execution plan attempt binding", execution.ErrInvalid)
			}
			if err := addAttemptIdentity(attempts, *group.SelectedAttemptID); err != nil {
				return err
			}
		}
	}

	return nil
}

// ComparePlan loads and validates every explicitly bound attempt before
// deriving aggregate shadow evidence. No partial report is returned.
func (service *Service) ComparePlan(
	ctx context.Context,
	plan planning.Plan,
	planSHA256 string,
	bindings PlanAttemptBindings,
) (PlanReport, error) {
	if service == nil || service.attempts == nil {
		return PlanReport{}, execution.ErrInvalid
	}
	plan = planning.CanonicalPlan(plan)
	if err := planning.ValidatePlan(plan); err != nil {
		return PlanReport{}, err
	}
	planReference := planning.Reference{APIVersion: plan.APIVersion, SHA256: planSHA256}
	if err := planning.ValidateReference(planReference); err != nil {
		return PlanReport{}, err
	}
	bindings = CanonicalPlanAttemptBindings(bindings)
	if err := ValidatePlanAttemptBindings(bindings); err != nil {
		return PlanReport{}, err
	}
	plannedGroups := collectPlannedGroups(plan)
	if len(plannedGroups) != len(bindings.Groups) {
		return PlanReport{}, fmt.Errorf("%w: incomplete execution plan attempt bindings", execution.ErrInvalid)
	}

	report := PlanReport{
		APIVersion: PlanReportAPIVersion, Plan: planReference, Manifest: plan.Manifest,
		Groups: make([]PlanGroupReport, 0, len(bindings.Groups)),
	}
	for _, binding := range bindings.Groups {
		planned, exists := plannedGroups[binding.GroupKey]
		if !exists || planned.HasSelected != (binding.SelectedAttemptID != nil) {
			return PlanReport{}, fmt.Errorf("%w: execution plan attempt group mismatch", execution.ErrInvalid)
		}
		groupReport, err := service.comparePlannedGroup(ctx, plan.Manifest, planned, binding)
		if err != nil {
			return PlanReport{}, err
		}
		report.addGroup(groupReport)
	}
	report.DurationReductionMS = report.FullSuiteDurationMS - report.SelectedDurationMS
	if report.FullSuiteFailureCount != 0 {
		recall := report.CaughtFullSuiteFailureCount * 10_000 / report.FullSuiteFailureCount
		report.FailureRecallBasisPoints = &recall
	}

	return report, nil
}

type plannedGroup struct {
	Selected    planning.Job
	FullSuite   planning.Job
	HasSelected bool
}

func collectPlannedGroups(plan planning.Plan) map[string]plannedGroup {
	groups := make(map[string]plannedGroup, len(plan.Jobs))
	for _, job := range plan.Jobs {
		group := groups[job.GroupKey]
		if job.Stage == execution.StageSelected {
			group.Selected = job
			group.HasSelected = true
		} else {
			group.FullSuite = job
		}
		groups[job.GroupKey] = group
	}

	return groups
}

func (service *Service) comparePlannedGroup(
	ctx context.Context,
	manifest execution.ManifestReference,
	planned plannedGroup,
	binding GroupAttemptBinding,
) (PlanGroupReport, error) {
	fullSuite, err := service.loadPlannedAttempt(ctx, binding.FullSuiteAttemptID, manifest, planned.FullSuite)
	if err != nil {
		return PlanGroupReport{}, err
	}
	if !planned.HasSelected {
		return buildFullOnlyGroupReport(binding.GroupKey, fullSuite), nil
	}
	selected, err := service.loadPlannedAttempt(ctx, *binding.SelectedAttemptID, manifest, planned.Selected)
	if err != nil {
		return PlanGroupReport{}, err
	}
	if err := validatePair(selected, fullSuite); err != nil {
		return PlanGroupReport{}, err
	}

	return buildSelectedGroupReport(binding.GroupKey, buildReport(selected, fullSuite)), nil
}

func (service *Service) loadPlannedAttempt(
	ctx context.Context,
	attemptID string,
	manifest execution.ManifestReference,
	job planning.Job,
) (execution.Attempt, error) {
	attempt, err := service.attempts.FindExecutionAttempt(ctx, attemptID)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt = execution.CanonicalAttempt(attempt)
	if execution.ValidateAttempt(attempt) != nil {
		return execution.Attempt{}, execution.ErrUnavailable
	}
	if attempt.Manifest != manifest || attempt.Stage != job.Stage ||
		attempt.TestRepository != job.TestRepository || attempt.TestRevision != job.TestRevision ||
		attempt.AdapterID != job.Adapter || len(attempt.Results) != job.TestCount {
		return execution.Attempt{}, fmt.Errorf("%w: attempt does not satisfy execution plan job", execution.ErrInvalid)
	}

	return attempt, nil
}

func buildSelectedGroupReport(groupKey string, report Report) PlanGroupReport {
	selectedAttemptID := report.SelectedAttemptID

	return PlanGroupReport{
		GroupKey: groupKey, TestRepository: report.TestRepository, TestRevision: report.TestRevision,
		AdapterID: report.AdapterID, AdapterVersion: report.AdapterVersion,
		SelectedAttemptID: &selectedAttemptID, FullSuiteAttemptID: report.FullSuiteAttemptID,
		SelectedTestCount: report.SelectedTestCount, FullSuiteTestCount: report.FullSuiteTestCount,
		SelectedDurationMS: report.SelectedDurationMS, FullSuiteDurationMS: report.FullSuiteDurationMS,
		DurationReductionMS:  report.DurationReductionMS,
		SelectedFailureCount: report.SelectedFailureCount, FullSuiteFailureCount: report.FullSuiteFailureCount,
		CaughtFullSuiteFailureCount: report.CaughtFullSuiteFailureCount,
		FailureRecallBasisPoints:    report.FailureRecallBasisPoints,
		MissedFailures:              append([]FailureMiss{}, report.MissedFailures...),
	}
}

func buildFullOnlyGroupReport(groupKey string, fullSuite execution.Attempt) PlanGroupReport {
	report := PlanGroupReport{
		GroupKey: groupKey, TestRepository: fullSuite.TestRepository,
		TestRevision: fullSuite.TestRevision, AdapterID: fullSuite.AdapterID,
		AdapterVersion: fullSuite.AdapterVersion, FullSuiteAttemptID: fullSuite.AttemptID,
		FullSuiteTestCount:  len(fullSuite.Results),
		FullSuiteDurationMS: sumDurationMilliseconds(fullSuite.Results),
		MissedFailures:      make([]FailureMiss, 0),
	}
	report.DurationReductionMS = report.FullSuiteDurationMS
	for _, result := range fullSuite.Results {
		if !isFailure(result.Outcome) {
			continue
		}
		report.FullSuiteFailureCount++
		report.MissedFailures = append(report.MissedFailures, newFailureMiss(result, execution.TestResult{}, false))
	}
	if report.FullSuiteFailureCount != 0 {
		recall := 0
		report.FailureRecallBasisPoints = &recall
	}
	sortFailureMisses(report.MissedFailures)

	return report
}

func (report *PlanReport) addGroup(group PlanGroupReport) {
	report.GroupCount++
	if group.SelectedAttemptID == nil {
		report.FullOnlyGroupCount++
	} else {
		report.GroupsWithSelection++
	}
	report.SelectedTestCount += group.SelectedTestCount
	report.FullSuiteTestCount += group.FullSuiteTestCount
	report.SelectedDurationMS += group.SelectedDurationMS
	report.FullSuiteDurationMS += group.FullSuiteDurationMS
	report.SelectedFailureCount += group.SelectedFailureCount
	report.FullSuiteFailureCount += group.FullSuiteFailureCount
	report.CaughtFullSuiteFailureCount += group.CaughtFullSuiteFailureCount
	report.Groups = append(report.Groups, group)
}

func addAttemptIdentity(attempts map[string]struct{}, attemptID string) error {
	if _, exists := attempts[attemptID]; exists {
		return fmt.Errorf("%w: duplicate execution plan attempt ID", execution.ErrInvalid)
	}
	attempts[attemptID] = struct{}{}

	return nil
}
