package browsershadow

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
)

type plannedGroup struct {
	Selected    browserplanning.Job
	Full        browserplanning.Job
	HasSelected bool
}

// Compare rejects missing, repeated, or cross-group attempts and returns no
// partial report. The full-suite result is the authoritative failure set.
func Compare(ctx context.Context, reader AttemptReader, plan browserplanning.Plan, planSHA256 string, bindings Bindings) (Report, error) {
	if reader == nil || browserplanning.ValidatePlan(plan) != nil || !digestPattern.MatchString(planSHA256) || bindings.APIVersion != BindingsVersion || len(bindings.Groups) == 0 || len(bindings.Groups) > browserplanning.MaxGroups {
		return Report{}, browser.ErrInvalid
	}
	groups := indexJobs(plan.Jobs)
	if err := validateBindings(groups, bindings.Groups); err != nil {
		return Report{}, err
	}
	report := Report{APIVersion: ReportVersion, PlanSHA256: planSHA256, Manifest: plan.Manifest, Groups: make([]GroupReport, 0, len(bindings.Groups))}
	for _, binding := range bindings.Groups {
		entry, err := compareGroup(ctx, reader, plan.Manifest, groups[binding.GroupKey], binding)
		if err != nil {
			return Report{}, err
		}
		report.add(entry)
	}
	slices.SortFunc(report.Groups, func(a, b GroupReport) int {
		if a.GroupKey < b.GroupKey {
			return -1
		}
		if a.GroupKey > b.GroupKey {
			return 1
		}

		return 0
	})
	report.DurationReductionMS = report.FullSuiteDurationMS - report.SelectedDurationMS
	if report.FullSuiteFailureCount > 0 {
		recall := report.CaughtFullSuiteFailureCount * 10000 / report.FullSuiteFailureCount
		report.FailureRecallBasisPoints = &recall
	}

	return report, nil
}

func indexJobs(jobs []browserplanning.Job) map[string]plannedGroup {
	groups := make(map[string]plannedGroup, len(jobs))
	for _, job := range jobs {
		group := groups[job.GroupKey]
		if job.Stage == execution.StageSelected {
			group.Selected = job
			group.HasSelected = true
		} else {
			group.Full = job
		}
		groups[job.GroupKey] = group
	}

	return groups
}

func validateBindings(groups map[string]plannedGroup, bindings []GroupBinding) error {
	if len(groups) != len(bindings) {
		return browser.ErrInvalid
	}
	seenGroups := map[string]bool{}
	seenAttempts := map[string]bool{}
	for _, binding := range bindings {
		group, ok := groups[binding.GroupKey]
		if !ok || seenGroups[binding.GroupKey] || group.HasSelected != (binding.SelectedAttemptID != nil) {
			return browser.ErrInvalid
		}
		seenGroups[binding.GroupKey] = true
		if err := includeAttempt(seenAttempts, binding.FullSuiteAttemptID); err != nil {
			return err
		}
		if binding.SelectedAttemptID != nil {
			if err := includeAttempt(seenAttempts, *binding.SelectedAttemptID); err != nil {
				return err
			}
		}
	}

	return nil
}

func includeAttempt(seen map[string]bool, id string) error {
	if execution.ValidateAttemptID(id) != nil || seen[id] {
		return browser.ErrInvalid
	}
	seen[id] = true

	return nil
}

func compareGroup(ctx context.Context, reader AttemptReader, manifest execution.ManifestReference, group plannedGroup, binding GroupBinding) (GroupReport, error) {
	full, err := load(ctx, reader, binding.FullSuiteAttemptID, manifest, group.Full)
	if err != nil {
		return GroupReport{}, err
	}
	entry := GroupReport{Binding: group.Full.Binding, AdapterVersion: full.Result.AdapterVersion, SelectedAttemptID: binding.SelectedAttemptID, FullSuiteAttemptID: binding.FullSuiteAttemptID, MissedFailures: []Miss{}}
	var selected browser.Attempt
	if group.HasSelected {
		selected, err = load(ctx, reader, *binding.SelectedAttemptID, manifest, group.Selected)
		if err != nil {
			return GroupReport{}, err
		}
		if selected.Result.AdapterVersion != full.Result.AdapterVersion {
			return GroupReport{}, fmt.Errorf("%w: browser adapter versions differ", browser.ErrInvalid)
		}
		entry.SelectedTestCount = len(selected.Result.Results)
		entry.SelectedDurationMS = duration(selected.Result.Results)
	}
	entry.FullSuiteTestCount = len(full.Result.Results)
	entry.FullSuiteDurationMS = duration(full.Result.Results)
	entry.DurationReductionMS = entry.FullSuiteDurationMS - entry.SelectedDurationMS
	if err := classifyFailures(&entry, selected.Result.Results, full.Result.Results); err != nil {
		return GroupReport{}, err
	}

	return entry, nil
}

func classifyFailures(entry *GroupReport, selected, full []execution.TestResult) error {
	selectedByKey := map[string]execution.TestResult{}
	fullByKey := map[string]bool{}
	for _, result := range selected {
		selectedByKey[result.SuiteKey+"\x00"+result.TestKey] = result
		if failed(result.Outcome) {
			entry.SelectedFailureCount++
		}
	}
	for _, result := range full {
		fullByKey[result.SuiteKey+"\x00"+result.TestKey] = true
	}
	for key := range selectedByKey {
		if !fullByKey[key] {
			return fmt.Errorf("%w: selected test absent from full suite", browser.ErrInvalid)
		}
	}
	for _, result := range full {
		if !failed(result.Outcome) {
			continue
		}
		entry.FullSuiteFailureCount++
		observed, found := selectedByKey[result.SuiteKey+"\x00"+result.TestKey]
		if found && failed(observed.Outcome) {
			entry.CaughtFullSuiteFailureCount++
			continue
		}
		miss := Miss{SuiteKey: result.SuiteKey, TestKey: result.TestKey, FullSuiteOutcome: result.Outcome, Reason: "not-selected"}
		if found {
			outcome := observed.Outcome
			miss.SelectedOutcome = &outcome
			miss.Reason = "not-reproduced"
		}
		entry.MissedFailures = append(entry.MissedFailures, miss)
	}
	slices.SortFunc(entry.MissedFailures, func(a, b Miss) int {
		return cmp.Or(cmp.Compare(a.SuiteKey, b.SuiteKey), cmp.Compare(a.TestKey, b.TestKey))
	})
	if entry.FullSuiteFailureCount > 0 {
		recall := entry.CaughtFullSuiteFailureCount * 10000 / entry.FullSuiteFailureCount
		entry.FailureRecallBasisPoints = &recall
	}

	return nil
}

func (report *Report) add(group GroupReport) {
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
