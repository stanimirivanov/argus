package predictive

import (
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/selection"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

// SafetyVersion identifies observational evidence, never promotion authority.
const SafetyVersion = "argus.dev/predictive-safety-report/v1"

// SafetyWindow is explicit caller time; the monitor never reads a clock.
type SafetyWindow struct{ Start, End, AsOf time.Time }

// SafetyScope pins the immutable execution variant independently of results.
type SafetyScope struct {
	Repository                    catalog.Repository
	Revision                      catalog.Revision
	Family                        catalog.TestFamily
	Adapter, Project, Environment string
}

// SafetyCandidate is projected from a verified complete plan and cost review.
// Probability is supplied only when its frozen calibration artifact matches.
type SafetyCandidate struct {
	ID, SuiteKey, TestKey, Name string
	Early                       bool
	DurationMS, ResourceUnits   int64
	Probability                 *float64
}

// SafetyNamedTest is a complete browser inventory item, not a result label.
type SafetyNamedTest struct{ SuiteKey, TestKey, Name string }

// SafetyAttempt is a normalized v2 attempt after protocol/artifact-shape checks.
type SafetyAttempt struct {
	ID, AdapterVersion, DiscoverySHA256 string
	Inventory                           []SafetyNamedTest
	Scope                               SafetyScope
	Manifest                            execution.ManifestReference
	Staging                             execution.Staging
	Stage                               execution.Stage
	StartedAt, CompletedAt              time.Time
	Results                             []execution.TestResult
}

// SafetyInput is one independently verified run/group. Nil attempts mean missing
// evidence, not successful empty work. Empty phases follow candidate membership.
type SafetyInput struct {
	RunID, EpisodeID, GroupKey, PlanSHA256, PolicyVersion, DiscoverySHA256 string
	FeatureCutoffAt                                                        time.Time
	Schedule                                                               execution.ScheduleReference
	Manifest                                                               execution.ManifestReference
	Mode                                                                   string
	Scope                                                                  SafetyScope
	Candidates                                                             []SafetyCandidate
	Early, Final                                                           *SafetyAttempt
	SetupMS, CapacityUnits                                                 int64
	Calibration                                                            *execution.ManifestReference
	ModelSHA256                                                            *string
	BaselineECE                                                            *float64
	CalibrationCutoffAt                                                    time.Time
}

// SafetyPhase distinguishes completed evidence from pending, late and empty work.
// Estimated work includes reviewed setup once per nonempty group invocation.
type SafetyPhase struct {
	Stage                                        execution.Stage
	State                                        string
	AttemptID                                    *string
	StartedAt, CompletedAt                       *time.Time
	DurationMS, ElapsedMS                        *int64
	EstimatedDurationMS, EstimatedResourceUnitMS int64
}

// SafetyMiss names each known full/remaining failure not caught early.
type SafetyMiss struct{ ID, SuiteKey, TestKey, Reason string }

// SafetyGroup retains source inputs and exposes undefined measurements as nil.
type SafetyGroup struct {
	Input                                     SafetyInput
	State                                     string
	CandidateCount, EarlyCount, UnknownLabels int
	SelectionRate                             float64
	Failures, CaughtFailures                  *int
	Recall, Brier, ECE, ECEDrift              *float64
	FirstFailureMS                            *int64
	Misses                                    []SafetyMiss
	Phases                                    []SafetyPhase
}

// SafetyCohort aggregates exposures by repository/family. Incomplete groups
// withhold aggregate recall; repeated groups are observations, not unique tests.
type SafetyCohort struct {
	Repository                                catalog.RepositoryIdentity
	Family                                    catalog.TestFamily
	Groups, CompleteGroups, Candidates, Early int
	SelectionRate                             float64
	Failures, CaughtFailures                  *int
	Recall                                    *float64
}

// SafetyReport is a reproducible as-of snapshot; it contains no safe/approved bit.
type SafetyReport struct {
	Window      SafetyWindow
	Groups      []SafetyGroup
	Cohorts     []SafetyCohort
	Unavailable []string
}

// Monitor compares complete explicit stages; late/missing/unknown evidence never
// becomes perfect recall or a negative label. Inputs must come from verified
// independent plans, not inventories reconstructed from attempts.
func Monitor(window SafetyWindow, inputs []SafetyInput) (SafetyReport, error) {
	if window.Start.IsZero() || !window.Start.Before(window.End) || window.AsOf.Before(window.Start) || len(inputs) == 0 || len(inputs) > 10000 {
		return SafetyReport{}, ErrInvalid
	}
	report := SafetyReport{Window: window, Groups: []SafetyGroup{}, Unavailable: []string{"criticality-not-recorded", "measured-resource-and-billed-cost-not-recorded", "analysis-and-review-overhead-not-recorded", "per-test-completion-not-recorded", "artifact-arrival-time-not-recorded", "portfolio-model-digest-not-recorded"}}
	groups, attempts := map[string]bool{}, map[string]bool{}
	count := 0
	for _, input := range inputs {
		key := input.RunID + "\x00" + input.GroupKey
		count += len(input.Candidates)
		if groups[key] || count > 100000 {
			return SafetyReport{}, ErrInvalid
		}
		groups[key] = true
		group, err := monitorGroup(window, ownedSafetyInput(input), attempts)
		if err != nil {
			return SafetyReport{}, fmt.Errorf("%w: run %s group %s", err, input.RunID, input.GroupKey)
		}
		report.Groups = append(report.Groups, group)
	}
	slices.SortFunc(report.Groups, func(a, b SafetyGroup) int {
		if a.Input.RunID != b.Input.RunID {
			return compareText(a.Input.RunID, b.Input.RunID)
		}
		return compareText(a.Input.GroupKey, b.Input.GroupKey)
	})
	report.Cohorts = safetyCohorts(report.Groups)

	return report, nil
}

func monitorGroup(window SafetyWindow, input SafetyInput, used map[string]bool) (SafetyGroup, error) {
	if err := validateSafetyInput(input); err != nil {
		return SafetyGroup{}, err
	}
	if input.Calibration != nil && (input.CalibrationCutoffAt.IsZero() || input.CalibrationCutoffAt.After(window.Start)) {
		return SafetyGroup{}, ErrInvalid
	}
	group := SafetyGroup{Input: input, State: "complete", CandidateCount: len(input.Candidates), Misses: []SafetyMiss{}, Phases: []SafetyPhase{}}
	for _, c := range input.Candidates {
		if c.Early {
			group.EarlyCount++
		}
	}
	group.SelectionRate = float64(group.EarlyCount) / float64(group.CandidateCount)
	finalStage := execution.StageRemainingLater
	if input.Mode == "shadow" {
		finalStage = execution.StageShadowControl
	}
	for _, p := range []struct {
		stage   execution.Stage
		attempt *SafetyAttempt
	}{{execution.StageRelevantNow, input.Early}, {finalStage, input.Final}} {
		phase, err := monitorPhase(window, input, p.stage, p.attempt, used)
		if err != nil {
			return SafetyGroup{}, err
		}
		group.Phases = append(group.Phases, phase)
		group.State = mergeSafetyState(group.State, phase.State)
	}
	if input.Early != nil && input.Final != nil && (input.Final.StartedAt.Before(input.Early.CompletedAt) || input.Final.AdapterVersion != input.Early.AdapterVersion) {
		return SafetyGroup{}, ErrInvalid
	}
	group.FirstFailureMS = firstObservedFailure(input, window, group.EarlyCount)
	group.Misses = observedSafetyMisses(input, window)
	labels := safetyLabels(input, window)
	for _, label := range labels {
		if label != execution.TestPassed && label != execution.TestFailed {
			group.UnknownLabels++
		}
	}
	if group.State == "complete" {
		measureSafety(&group, labels)
	}

	return group, nil
}

func validateSafetyInput(input SafetyInput) error {
	ref := execution.Staging{Schedule: input.Schedule, EpisodeID: input.EpisodeID, GroupKey: input.GroupKey, Environment: input.Scope.Environment, Mode: input.Mode}
	if execution.ValidateStaging(&ref, execution.StageRelevantNow) != nil || !catalog.IsLocalKey(input.RunID) || !safetyDigest(input.PlanSHA256) || !safetyDigest(input.Manifest.SHA256) || input.PolicyVersion == "" || input.FeatureCutoffAt.IsZero() ||
		execution.ValidateTestRepository(input.Scope.Repository) != nil || execution.ValidateTestRevision(input.Scope.Revision) != nil || !catalog.IsLocalKey(input.Scope.Adapter) || len(input.Candidates) == 0 || len(input.Candidates) > execution.MaxAttemptTests || input.SetupMS < 0 || input.SetupMS > 86400000 || input.CapacityUnits < 1 || input.CapacityUnits > 10000 {
		return ErrInvalid
	}
	if (input.Scope.Family == catalog.TestFamilyFunctionalAPI && (input.Scope.Project != "" || input.DiscoverySHA256 != "")) || (input.Scope.Family == catalog.TestFamilyFunctionalUI && (!catalog.IsLocalKey(input.Scope.Project) || !safetyDigest(input.DiscoverySHA256))) ||
		(input.Scope.Family != catalog.TestFamilyFunctionalAPI && input.Scope.Family != catalog.TestFamilyFunctionalUI) {
		return ErrInvalid
	}
	if input.Scope.Family == catalog.TestFamilyFunctionalAPI && input.Manifest.APIVersion != selection.ManifestAPIVersion && input.Manifest.APIVersion != selection.ReviewedManifestAPIVersion {
		return ErrInvalid
	}
	if input.Scope.Family == catalog.TestFamilyFunctionalUI && input.Manifest.APIVersion != "argus.dev/browser-selection-manifest/v1" {
		return ErrInvalid
	}
	if err := validateSafetyCalibration(input); err != nil {
		return err
	}

	return validateSafetyCandidates(input)
}

func validateSafetyCandidates(input SafetyInput) error {
	ids, keys := map[string]bool{}, map[string]bool{}
	for _, c := range input.Candidates {
		key := c.SuiteKey + "\x00" + c.TestKey
		if !safetyDigest(c.ID) || ids[c.ID] || keys[key] || !catalog.IsLocalKey(c.SuiteKey) || !catalog.IsLocalKey(c.TestKey) || c.DurationMS < 1 || c.DurationMS > 86400000 || c.ResourceUnits < 1 || c.ResourceUnits > input.CapacityUnits {
			return ErrInvalid
		}
		if c.Probability != nil && (!validProbability(*c.Probability) || input.Calibration == nil) {
			return ErrInvalid
		}
		ids[c.ID], keys[key] = true, true
	}

	return nil
}

func validateSafetyCalibration(input SafetyInput) error {
	if input.Calibration == nil {
		if input.ModelSHA256 != nil || input.BaselineECE != nil {
			return ErrInvalid
		}
		return nil
	}
	if input.Calibration.APIVersion != CalibrationVersion || !safetyDigest(input.Calibration.SHA256) || input.ModelSHA256 == nil || !safetyDigest(*input.ModelSHA256) {
		return ErrInvalid
	}
	if input.BaselineECE != nil && !validProbability(*input.BaselineECE) {
		return ErrInvalid
	}

	return nil
}

func monitorPhase(window SafetyWindow, input SafetyInput, stage execution.Stage, attempt *SafetyAttempt, used map[string]bool) (SafetyPhase, error) {
	phase := SafetyPhase{Stage: stage, State: "empty"}
	expected := map[string]bool{}
	for _, c := range input.Candidates {
		if stage == execution.StageShadowControl || (stage == execution.StageRelevantNow) == c.Early {
			expected[c.SuiteKey+"\x00"+c.TestKey] = true
			phase.EstimatedDurationMS += c.DurationMS
			phase.EstimatedResourceUnitMS += c.DurationMS * c.ResourceUnits
		}
	}
	if len(expected) == 0 {
		if attempt != nil {
			return phase, ErrInvalid
		}
		return phase, nil
	}
	phase.EstimatedDurationMS += input.SetupMS
	phase.EstimatedResourceUnitMS += input.SetupMS * input.CapacityUnits
	if attempt == nil {
		phase.State = "pending"
		return phase, nil
	}
	if err := validateSafetyAttempt(input, stage, attempt, used); err != nil {
		return phase, err
	}
	if attempt.StartedAt.Before(window.Start) || !attempt.StartedAt.Before(window.End) || len(expected) != len(attempt.Results) {
		return phase, ErrInvalid
	}
	phase.State = "complete"
	phase.AttemptID = &attempt.ID
	phase.StartedAt = &attempt.StartedAt
	phase.CompletedAt = &attempt.CompletedAt
	var duration int64
	for _, r := range attempt.Results {
		key := r.SuiteKey + "\x00" + r.TestKey
		if !expected[key] {
			return phase, ErrInvalid
		}
		delete(expected, key)
		duration += r.Duration.Milliseconds()
		if r.Outcome == execution.TestSkipped || r.Outcome == execution.TestError {
			phase.State = "incomplete"
		}
	}
	if attempt.CompletedAt.After(window.AsOf) || attempt.CompletedAt.After(window.End) {
		phase.State = "late"
		return phase, nil
	}
	elapsed := attempt.CompletedAt.Sub(attempt.StartedAt).Milliseconds()
	phase.DurationMS = &duration
	phase.ElapsedMS = &elapsed

	return phase, nil
}

func validateSafetyAttempt(input SafetyInput, stage execution.Stage, attempt *SafetyAttempt, used map[string]bool) error {
	if attempt.DiscoverySHA256 != input.DiscoverySHA256 || attempt.StartedAt.Before(input.FeatureCutoffAt) || execution.ValidateAttemptID(attempt.ID) != nil || used[attempt.ID] || attempt.Scope != input.Scope || attempt.Manifest != input.Manifest || attempt.Stage != stage || attempt.AdapterVersion == "" ||
		attempt.Staging != (execution.Staging{Schedule: input.Schedule, EpisodeID: input.EpisodeID, GroupKey: input.GroupKey, Environment: input.Scope.Environment, Mode: input.Mode}) || execution.ValidateStaging(&attempt.Staging, stage) != nil {
		return ErrInvalid
	}
	if execution.ValidateResultEvidence(attempt.StartedAt, attempt.CompletedAt, attempt.Results, nil) != nil {
		return ErrInvalid
	}
	if err := validateSafetyInventory(input, attempt); err != nil {
		return err
	}
	used[attempt.ID] = true

	return nil
}

func safetyLabels(input SafetyInput, window SafetyWindow) map[string]execution.TestOutcome {
	labels := map[string]execution.TestOutcome{}
	attempts := []*SafetyAttempt{input.Final}
	if input.Mode == "staged" {
		attempts = append(attempts, input.Early)
	}
	for _, a := range attempts {
		if safetyObserved(a, window) {
			for _, r := range a.Results {
				labels[r.SuiteKey+"\x00"+r.TestKey] = r.Outcome
			}
		}
	}

	return labels
}

func measureSafety(group *SafetyGroup, labels map[string]execution.TestOutcome) {
	early := map[string]execution.TestOutcome{}
	if group.Input.Early != nil {
		for _, r := range group.Input.Early.Results {
			early[r.SuiteKey+"\x00"+r.TestKey] = r.Outcome
		}
	}
	failures, caught := 0, 0
	for _, c := range group.Input.Candidates {
		key := c.SuiteKey + "\x00" + c.TestKey
		if labels[key] != execution.TestFailed {
			continue
		}
		failures++
		if early[key] == execution.TestFailed {
			caught++
			continue
		}
	}
	group.Failures, group.CaughtFailures = &failures, &caught
	if failures > 0 {
		v := float64(caught) / float64(failures)
		group.Recall = &v
	}
	calibrationSafety(group, labels)
}

func calibrationSafety(group *SafetyGroup, labels map[string]execution.TestOutcome) {
	var counts [10]int
	var expected, actual [10]float64
	brier := 0.0
	for _, c := range group.Input.Candidates {
		if c.Probability == nil {
			return
		}
		p := *c.Probability
		y := 0.0
		if labels[c.SuiteKey+"\x00"+c.TestKey] == execution.TestFailed {
			y = 1
		}
		bin := min(9, int(p*10))
		counts[bin]++
		expected[bin] += p
		actual[bin] += y
		brier += (p - y) * (p - y)
	}
	brier /= float64(group.CandidateCount)
	ece := 0.0
	for i, n := range counts {
		if n > 0 {
			ece += math.Abs(expected[i]-actual[i]) / float64(group.CandidateCount)
		}
	}
	group.Brier, group.ECE = &brier, &ece
	if group.Input.BaselineECE != nil {
		drift := ece - *group.Input.BaselineECE
		group.ECEDrift = &drift
	}
}

func firstObservedFailure(input SafetyInput, window SafetyWindow, earlyCount int) *int64 {
	if earlyCount > 0 && input.Early == nil {
		return nil
	}
	var start, first time.Time
	for _, a := range []*SafetyAttempt{input.Early, input.Final} {
		if a == nil {
			continue
		}
		if start.IsZero() || a.StartedAt.Before(start) {
			start = a.StartedAt
		}
		if a.CompletedAt.After(window.AsOf) || a.CompletedAt.After(window.End) {
			continue
		}
		if safetyHasFailure(a.Results) && (first.IsZero() || a.CompletedAt.Before(first)) {
			first = a.CompletedAt
		}
	}
	if first.IsZero() {
		return nil
	}
	value := first.Sub(start).Milliseconds()

	return &value
}

func safetyHasFailure(results []execution.TestResult) bool {
	return slices.ContainsFunc(results, func(r execution.TestResult) bool { return r.Outcome == execution.TestFailed })
}

func safetyCohorts(groups []SafetyGroup) []SafetyCohort {
	cohorts := []SafetyCohort{}
	for _, g := range groups {
		i := slices.IndexFunc(cohorts, func(c SafetyCohort) bool {
			return c.Repository == g.Input.Scope.Repository.Identity && c.Family == g.Input.Scope.Family
		})
		if i < 0 {
			cohorts = append(cohorts, SafetyCohort{Repository: g.Input.Scope.Repository.Identity, Family: g.Input.Scope.Family, Failures: new(int), CaughtFailures: new(int)})
			i = len(cohorts) - 1
		}
		c := &cohorts[i]
		c.Groups++
		c.Candidates += g.CandidateCount
		c.Early += g.EarlyCount
		if g.State == "complete" {
			c.CompleteGroups++
			*c.Failures += *g.Failures
			*c.CaughtFailures += *g.CaughtFailures
		}
	}
	for i := range cohorts {
		c := &cohorts[i]
		c.SelectionRate = float64(c.Early) / float64(c.Candidates)
		if c.Groups != c.CompleteGroups {
			c.Failures, c.CaughtFailures = nil, nil
		} else if *c.Failures > 0 {
			v := float64(*c.CaughtFailures) / float64(*c.Failures)
			c.Recall = &v
		}
	}

	return cohorts
}

func safetyDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)

	return err == nil && s == strings.ToLower(s)
}

func validProbability(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func compareText(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}

	return 0
}

func validateSafetyInventory(input SafetyInput, attempt *SafetyAttempt) error {
	if input.Scope.Family == catalog.TestFamilyFunctionalAPI {
		if len(attempt.Inventory) != 0 {
			return ErrInvalid
		}
		return nil
	}
	if len(attempt.Inventory) != len(input.Candidates) {
		return ErrInvalid
	}
	expected := map[string]string{}
	for _, c := range input.Candidates {
		if c.Name == "" {
			return ErrInvalid
		}
		expected[c.SuiteKey+"\x00"+c.TestKey] = c.Name
	}
	for _, c := range attempt.Inventory {
		key := c.SuiteKey + "\x00" + c.TestKey
		if name, exists := expected[key]; !exists || name != c.Name {
			return ErrInvalid
		}
		delete(expected, key)
	}

	return nil
}

func ownedSafetyInput(input SafetyInput) SafetyInput {
	input.Candidates = slices.Clone(input.Candidates)
	for i := range input.Candidates {
		input.Candidates[i].Probability = ownedSafetyValue(input.Candidates[i].Probability)
	}
	input.Calibration = ownedSafetyValue(input.Calibration)
	input.ModelSHA256 = ownedSafetyValue(input.ModelSHA256)
	input.BaselineECE = ownedSafetyValue(input.BaselineECE)
	input.Early = ownedSafetyAttempt(input.Early)
	input.Final = ownedSafetyAttempt(input.Final)

	return input
}
func ownedSafetyAttempt(input *SafetyAttempt) *SafetyAttempt {
	value := ownedSafetyValue(input)
	if value == nil {
		return nil
	}
	value.Inventory = slices.Clone(value.Inventory)
	value.Results = slices.Clone(value.Results)
	for i := range value.Results {
		value.Results[i].Failure = ownedSafetyValue(value.Results[i].Failure)
	}

	return value
}
func ownedSafetyValue[T any](input *T) *T {
	if input == nil {
		return nil
	}
	value := *input

	return &value
}

func mergeSafetyState(current, phase string) string {
	if current == "pending" || phase == "pending" || phase == "late" {
		return "pending"
	}
	if current == "incomplete" || phase == "incomplete" {
		return "incomplete"
	}

	return "complete"
}

func safetyObserved(a *SafetyAttempt, window SafetyWindow) bool {
	return a != nil && !a.CompletedAt.After(window.AsOf) && !a.CompletedAt.After(window.End)
}

func observedSafetyMisses(input SafetyInput, window SafetyWindow) []SafetyMiss {
	misses := []SafetyMiss{}
	if !safetyObserved(input.Final, window) {
		return misses
	}
	labels := map[string]execution.TestOutcome{}
	for _, r := range input.Final.Results {
		labels[r.SuiteKey+"\x00"+r.TestKey] = r.Outcome
	}
	early := map[string]execution.TestOutcome{}
	if safetyObserved(input.Early, window) {
		for _, r := range input.Early.Results {
			early[r.SuiteKey+"\x00"+r.TestKey] = r.Outcome
		}
	}
	for _, c := range input.Candidates {
		key := c.SuiteKey + "\x00" + c.TestKey
		if labels[key] != execution.TestFailed {
			continue
		}
		reason := "not-selected"
		if c.Early {
			if early[key] != execution.TestPassed {
				continue
			}
			reason = "not-reproduced-early"
		}
		misses = append(misses, SafetyMiss{ID: c.ID, SuiteKey: c.SuiteKey, TestKey: c.TestKey, Reason: reason})
	}
	slices.SortFunc(misses, func(a, b SafetyMiss) int { return compareText(a.ID, b.ID) })

	return misses
}
