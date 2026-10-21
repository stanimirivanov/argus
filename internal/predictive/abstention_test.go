package predictive

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestAbstentionFallsBackToCompleteControlWithoutHeldoutLabels(t *testing.T) {
	replay, archive := calibrationFixture(t)
	archive["holdout"] = append(archive["holdout"], EvaluationTest{
		ID: "new", RepositoryKey: strings.Repeat("a", 64), Family: "functional-api",
		Decision: selection.OutcomeSkipForNow, Outcome: execution.TestPassed, DurationMS: 100})
	first, err := EvaluateAbstention(replay, archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Episodes) != 1 || first.Episodes[0].Mode != "full-control" ||
		first.Episodes[0].EvidenceState != "calibrated" ||
		!reflect.DeepEqual(first.Episodes[0].RequiredTestIDs, []string{"a", "b", "new"}) {
		t.Fatalf("uncertain decision did not require complete control: %+v", first)
	}
	reasons := make(map[string][]string, len(first.Episodes[0].Tests))
	for _, test := range first.Episodes[0].Tests {
		reasons[test.ID] = test.Reasons
	}
	if !slices.Contains(reasons["b"], AbstentionGlobalCalibration) ||
		!slices.Contains(reasons["new"], AbstentionColdStart) {
		t.Fatalf("missing fallback reasons: %+v", reasons)
	}
	changed := slicesClone(archive)
	for i := range changed["holdout"] {
		changed["holdout"][i].Outcome = execution.TestFailed
		changed["holdout"][i].DurationMS = 5000
	}
	second, err := EvaluateAbstention(replay, changed)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("held-out controls changed safety decision: %v %+v", err, second)
	}
}

func TestAbstentionUsesFullControlWhenCalibrationUnavailable(t *testing.T) {
	replay, archive := calibrationFixture(t)
	archive["calibration"] = archive["calibration"][:19]
	report, err := EvaluateAbstention(replay, archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Episodes) != 1 || report.Episodes[0].EvidenceState != "unavailable" ||
		report.Episodes[0].Mode != "full-control" || len(report.Episodes[0].Tests) != 0 ||
		!reflect.DeepEqual(report.Episodes[0].CandidateTestIDs, []string{"a", "b"}) ||
		!reflect.DeepEqual(report.Episodes[0].RequiredTestIDs, []string{"a", "b"}) {
		t.Fatalf("unavailable calibration did not require full control: %+v", report)
	}
	archive["holdout"] = nil
	if _, err := EvaluateAbstention(replay, archive); err == nil {
		t.Fatal("malformed archive accepted as safe fallback")
	}
}

func TestAbstentionKeepsSupportedEpisodeEligible(t *testing.T) {
	replay, archive := calibrationFixture(t)
	archive["holdout"] = archive["holdout"][:1]
	report, err := EvaluateAbstention(replay, archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Episodes) != 1 || report.Episodes[0].Mode != "eligible" ||
		len(report.Episodes[0].RequiredTestIDs) != 0 || len(report.Episodes[0].Tests) != 1 ||
		len(report.Episodes[0].Tests[0].Reasons) != 0 {
		t.Fatalf("supported evidence did not stay eligible: %+v", report.Episodes)
	}
}

func TestAbstentionRequiresLocalBinSupport(t *testing.T) {
	key := cohortKey{RepositoryKey: strings.Repeat("a", 64), Family: "functional-api"}
	curve := CalibrationCurve{Examples: 20, Failures: 2, Slope: 1}
	support := calibrationBinSupport([]CalibrationCohort{{RepositoryKey: key.RepositoryKey, Family: key.Family,
		Source: "cohort", Curve: &curve}}, map[cohortKey][]calibrationSample{
		key: {{Score: 0}, {Score: 0}},
	})
	if support[key][5] != 2 || support[key][4] != 0 || probabilityDecile(1) != 9 {
		t.Fatalf("unexpected decile support: %+v", support[key])
	}
	test := assessAbstentionTest(CalibratedTest{ID: "known", RepositoryKey: key.RepositoryKey,
		Family: key.Family, Probability: 0.5}, CalibrationCohort{Source: "cohort",
		Examples: 20, Curve: &curve}, 1, support[key])
	if !reflect.DeepEqual(test.Reasons, []string{AbstentionSparseBin}) || test.BinExamples != 2 {
		t.Fatalf("thin calibration bin was not rejected: %+v", test)
	}
}
