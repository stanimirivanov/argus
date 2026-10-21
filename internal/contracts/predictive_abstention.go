package contracts

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveAbstentionSchemaJSON = mustReadSchema("predictive-abstention/v1/predictive-abstention.schema.json")

var loadPredictiveAbstentionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveAbstentionSchemaJSON, "https://argus.dev/contracts/predictive-abstention/v1/schema.json", "predictive abstention")
})

// PredictiveAbstentionV1 is one offline safety decision for each holdout episode.
type PredictiveAbstentionV1 struct {
	APIVersion            string                        `json:"apiVersion"`
	CalibrationVersion    string                        `json:"calibrationVersion"`
	MinimumBinExamples    int                           `json:"minimumBinExamples"`
	FitEpisodeIDs         []string                      `json:"fitEpisodeIds"`
	CalibrationEpisodeIDs []string                      `json:"calibrationEpisodeIds"`
	Episodes              []PredictiveAbstentionEpisode `json:"episodes"`
}

// PredictiveAbstentionEpisode requires the complete test set when uncertain.
type PredictiveAbstentionEpisode struct {
	ID               string                     `json:"id"`
	Mode             string                     `json:"mode"`
	EvidenceState    string                     `json:"evidenceState"`
	CandidateTestIDs []string                   `json:"candidateTestIds"`
	RequiredTestIDs  []string                   `json:"requiredTestIds"`
	Tests            []PredictiveAbstentionTest `json:"tests"`
}

// PredictiveAbstentionTest exposes support and stable abstention reasons.
type PredictiveAbstentionTest struct {
	ID                string   `json:"id"`
	RepositoryKey     string   `json:"repositoryKey"`
	Family            string   `json:"family"`
	Probability       float64  `json:"probability"`
	CalibrationSource string   `json:"calibrationSource"`
	PriorControls     int      `json:"priorControls"`
	CohortExamples    int      `json:"cohortExamples"`
	CalibrationBin    int      `json:"calibrationBin"`
	BinExamples       int      `json:"binExamples"`
	Reasons           []string `json:"reasons"`
}

// ValidatePredictiveAbstentionV1 checks the portable report shape.
func ValidatePredictiveAbstentionV1(report PredictiveAbstentionV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if err := validateDocument(data, loadPredictiveAbstentionSchema, "predictive abstention"); err != nil {
		return err
	}
	seenEpisodes := make(map[string]bool, len(report.Episodes))
	for _, episode := range report.Episodes {
		if seenEpisodes[episode.ID] {
			return fmt.Errorf("duplicate abstention episode %q", episode.ID)
		}
		seenEpisodes[episode.ID] = true
		if err := validateAbstentionEpisode(episode, report); err != nil {
			return err
		}
	}

	return nil
}

func validateAbstentionEpisode(episode PredictiveAbstentionEpisode, report PredictiveAbstentionV1) error {
	ids, uncertain, err := abstentionTestIDs(episode.Tests, report.MinimumBinExamples)
	if err != nil {
		return err
	}
	if !strictlySortedIDs(episode.CandidateTestIDs) ||
		(episode.EvidenceState == "calibrated" && (!slices.Equal(ids, episode.CandidateTestIDs) ||
			uncertain != (episode.Mode == "full-control") ||
			len(report.FitEpisodeIDs) < 2 || len(report.CalibrationEpisodeIDs) == 0)) ||
		(episode.EvidenceState == "unavailable" && (len(episode.Tests) != 0 || episode.Mode != "full-control" ||
			len(report.FitEpisodeIDs) != 0 || len(report.CalibrationEpisodeIDs) != 0)) ||
		(episode.Mode == "full-control" && !slices.Equal(episode.CandidateTestIDs, episode.RequiredTestIDs)) ||
		(episode.Mode == "eligible" && len(episode.RequiredTestIDs) != 0) {
		return fmt.Errorf("incomplete abstention fallback for episode %q", episode.ID)
	}

	return nil
}

func abstentionTestIDs(tests []PredictiveAbstentionTest, minimumBinExamples int) ([]string, bool, error) {
	ids := make([]string, 0, len(tests))
	seen := make(map[string]bool, len(tests))
	uncertain := false
	for _, test := range tests {
		if seen[test.ID] {
			return nil, false, fmt.Errorf("duplicate abstention test %q", test.ID)
		}
		seen[test.ID] = true
		ids = append(ids, test.ID)
		if err := validateAbstentionReasons(test, minimumBinExamples); err != nil {
			return nil, false, err
		}
		uncertain = uncertain || len(test.Reasons) > 0
	}
	slices.Sort(ids)

	return ids, uncertain, nil
}

func strictlySortedIDs(ids []string) bool {
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			return false
		}
	}

	return true
}

func validateAbstentionReasons(test PredictiveAbstentionTest, minimumBinExamples int) error {
	want := make([]string, 0, 3)
	if test.PriorControls == 0 {
		want = append(want, "cold-start")
	}
	if test.CalibrationSource == "global" {
		want = append(want, "global-calibration")
	} else if test.BinExamples < minimumBinExamples {
		want = append(want, "sparse-calibration-bin")
	}
	if test.CalibrationBin != min(int(test.Probability*10), 9) || !slices.Equal(test.Reasons, want) {
		return fmt.Errorf("inconsistent abstention evidence for test %q", test.ID)
	}

	return nil
}
