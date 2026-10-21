package predictive

import (
	"errors"
	"slices"
)

// AbstentionVersion identifies the first offline predictive safety report.
const AbstentionVersion = "argus.dev/predictive-abstention/v1"

const minimumCalibrationBinExamples = 3

const (
	// AbstentionColdStart means no completed control exists for this test.
	AbstentionColdStart = "cold-start"
	// AbstentionGlobalCalibration means the local cohort curve is unavailable.
	AbstentionGlobalCalibration = "global-calibration"
	// AbstentionSparseBin means the local probability decile has thin support.
	AbstentionSparseBin = "sparse-calibration-bin"
)

// AbstentionTest records only decision-time evidence, never a held-out label.
type AbstentionTest struct {
	ID                string
	RepositoryKey     string
	Family            string
	Probability       float64
	CalibrationSource string
	PriorControls     int
	CohortExamples    int
	CalibrationBin    int
	BinExamples       int
	Reasons           []string
}

// AbstentionEpisode is eligible for later scheduling or requires full control.
type AbstentionEpisode struct {
	ID               string
	Mode             string
	EvidenceState    string
	CandidateTestIDs []string
	RequiredTestIDs  []string
	Tests            []AbstentionTest
}

// AbstentionReport records a frozen, offline safety gate over calibration.
type AbstentionReport struct {
	APIVersion            string
	CalibrationVersion    string
	MinimumBinExamples    int
	FitEpisodeIDs         []string
	CalibrationEpisodeIDs []string
	Episodes              []AbstentionEpisode
}

// EvaluateAbstention refuses predictive eligibility when a held-out test has
// no completed prior control, no local cohort curve, or thin calibration-bin
// support. A single refusal requires the entire episode's full control set.
func EvaluateAbstention(replay Replay, archive map[string][]EvaluationTest) (AbstentionReport, error) {
	calibration, err := CalibrateRanking(replay, archive)
	if err != nil {
		if !errors.Is(err, ErrInvalid) || replay.APIVersion != ReplayVersion || replay.HoldoutCount == 0 {
			return AbstentionReport{}, err
		}
		if validationErr := validateEvaluationArchive(replay, archive); validationErr != nil {
			return AbstentionReport{}, validationErr
		}
		if validationErr := validateCalibrationCoordinates(replay, archive); validationErr != nil {
			return AbstentionReport{}, validationErr
		}

		return unavailableAbstention(replay, archive), nil
	}
	rows, history, err := causalTrainingRows(replay, archive)
	if err != nil {
		return AbstentionReport{}, err
	}
	fitIDs := make(map[string]bool, len(calibration.FitEpisodeIDs))
	for _, id := range calibration.FitEpisodeIDs {
		fitIDs[id] = true
	}
	_, calibrationRows := splitTrainingRows(rows, fitIDs)
	_, samples := scoreCalibrationRows(calibrationRows, calibration.Model)
	support := calibrationBinSupport(calibration.Cohorts, samples)
	cohorts := make(map[cohortKey]CalibrationCohort, len(calibration.Cohorts))
	for _, cohort := range calibration.Cohorts {
		cohorts[cohortKey{cohort.RepositoryKey, cohort.Family}] = cohort
	}
	report := AbstentionReport{APIVersion: AbstentionVersion, CalibrationVersion: calibration.APIVersion,
		MinimumBinExamples: minimumCalibrationBinExamples,
		FitEpisodeIDs:      calibration.FitEpisodeIDs, CalibrationEpisodeIDs: calibration.CalibrationEpisodeIDs,
		Episodes: make([]AbstentionEpisode, 0, len(calibration.Episodes))}
	for _, episode := range calibration.Episodes {
		entry := AbstentionEpisode{ID: episode.ID, Mode: "eligible", EvidenceState: "calibrated",
			CandidateTestIDs: make([]string, 0, len(episode.Predictions)), RequiredTestIDs: []string{},
			Tests: make([]AbstentionTest, 0, len(episode.Predictions))}
		for _, prediction := range episode.Predictions {
			key := cohortKey{prediction.RepositoryKey, prediction.Family}
			cohort := cohorts[key]
			test := assessAbstentionTest(prediction, cohort, history[prediction.ID].Seen, support[key])
			if len(test.Reasons) > 0 {
				entry.Mode = "full-control"
			}
			entry.Tests = append(entry.Tests, test)
			entry.CandidateTestIDs = append(entry.CandidateTestIDs, test.ID)
		}
		slices.Sort(entry.CandidateTestIDs)
		if entry.Mode == "full-control" {
			entry.RequiredTestIDs = slices.Clone(entry.CandidateTestIDs)
		}
		report.Episodes = append(report.Episodes, entry)
	}

	return report, nil
}

func unavailableAbstention(replay Replay, archive map[string][]EvaluationTest) AbstentionReport {
	report := AbstentionReport{APIVersion: AbstentionVersion, CalibrationVersion: CalibrationVersion,
		MinimumBinExamples: minimumCalibrationBinExamples, FitEpisodeIDs: []string{},
		CalibrationEpisodeIDs: []string{}, Episodes: make([]AbstentionEpisode, 0, replay.HoldoutCount)}
	for _, episode := range replay.Episodes {
		if episode.Split != "holdout" {
			continue
		}
		ids := make([]string, 0, len(archive[episode.ID]))
		for _, test := range archive[episode.ID] {
			ids = append(ids, test.ID)
		}
		slices.Sort(ids)
		report.Episodes = append(report.Episodes, AbstentionEpisode{ID: episode.ID, Mode: "full-control",
			EvidenceState: "unavailable", CandidateTestIDs: ids,
			RequiredTestIDs: slices.Clone(ids), Tests: []AbstentionTest{}})
	}

	return report
}

func assessAbstentionTest(prediction CalibratedTest, cohort CalibrationCohort, priorControls int, support [10]int) AbstentionTest {
	bin := probabilityDecile(prediction.Probability)
	test := AbstentionTest{ID: prediction.ID, RepositoryKey: prediction.RepositoryKey, Family: prediction.Family,
		Probability: prediction.Probability, CalibrationSource: cohort.Source,
		PriorControls: priorControls, CohortExamples: cohort.Examples,
		CalibrationBin: bin, BinExamples: support[bin], Reasons: []string{}}
	if test.PriorControls == 0 {
		test.Reasons = append(test.Reasons, AbstentionColdStart)
	}
	if cohort.Curve == nil {
		test.Reasons = append(test.Reasons, AbstentionGlobalCalibration)
	} else if test.BinExamples < minimumCalibrationBinExamples {
		test.Reasons = append(test.Reasons, AbstentionSparseBin)
	}

	return test
}

func calibrationBinSupport(cohorts []CalibrationCohort, samples map[cohortKey][]calibrationSample) map[cohortKey][10]int {
	support := make(map[cohortKey][10]int, len(cohorts))
	for _, cohort := range cohorts {
		if cohort.Curve == nil {
			continue
		}
		key := cohortKey{cohort.RepositoryKey, cohort.Family}
		var counts [10]int
		for _, sample := range samples[key] {
			counts[probabilityDecile(cohort.Curve.probability(sample.Score))]++
		}
		support[key] = counts
	}

	return support
}

func probabilityDecile(probability float64) int {
	return min(int(probability*10), 9)
}
