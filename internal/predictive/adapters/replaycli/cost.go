package replaycli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunCost emits complete stage estimates only after review and graph validation.
func RunCost(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-cost", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	constraintsPath := flags.String("constraints", "", "reviewed stage constraints JSON")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || *constraintsPath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-cost -index <path> -constraints <path>")
	}
	data, err := readBounded(*constraintsPath, 16<<20)
	if err != nil {
		return err
	}
	document, err := contracts.DecodePredictiveStageConstraintsV1(data)
	if err != nil {
		return err
	}
	constraints, err := importStageConstraints(document)
	if err != nil {
		return err
	}
	replay, archive, err := loadDataset(*indexPath, true)
	if err != nil {
		return err
	}
	report, err := predictive.EstimateStageCost(replay, archive, constraints)
	if err != nil {
		return err
	}
	checksum := sha256.Sum256(data)
	output := exportStageCost(report, hex.EncodeToString(checksum[:]))
	if err := contracts.ValidatePredictiveStageCostV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("stage cost report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func importStageConstraints(document contracts.PredictiveStageConstraintsV1) (predictive.StageConstraints, error) {
	constraints := predictive.StageConstraints{APIVersion: document.APIVersion, Episodes: make([]predictive.StageReview, 0, len(document.Episodes))}
	for _, episode := range document.Episodes {
		reviewedAt, err := time.Parse(time.RFC3339Nano, episode.ReviewedAt)
		if err != nil {
			return predictive.StageConstraints{}, err
		}
		cutoff, err := time.Parse(time.RFC3339Nano, episode.FeatureCutoffAt)
		if err != nil {
			return predictive.StageConstraints{}, err
		}
		review := predictive.StageReview{EpisodeID: episode.ID, ReviewID: episode.ReviewID,
			ReviewedBy: episode.ReviewedBy, ReviewedAt: reviewedAt, FeatureCutoffAt: cutoff, ManifestSHA256: episode.ManifestSHA256,
			Environments: make([]predictive.StageEnvironment, 0, len(episode.Environments)),
			Tests:        make([]predictive.StageTestConstraint, 0, len(episode.Tests))}
		for _, environment := range episode.Environments {
			review.Environments = append(review.Environments, predictive.StageEnvironment{
				Key: environment.Key, CapacityUnits: environment.CapacityUnits, SetupDurationMS: environment.SetupDurationMS})
		}
		for _, test := range episode.Tests {
			review.Tests = append(review.Tests, predictive.StageTestConstraint{ID: test.ID, Environment: test.Environment,
				ResourceUnits: test.ResourceUnits, ColdStartDurationMS: test.ColdStartDurationMS, Dependencies: test.Dependencies})
		}
		constraints.Episodes = append(constraints.Episodes, review)
	}

	return constraints, nil
}

func exportStageCost(report predictive.StageCostReport, checksum string) contracts.PredictiveStageCostV1 {
	output := contracts.PredictiveStageCostV1{APIVersion: report.APIVersion, ConstraintsSHA256: checksum,
		Episodes: make([]contracts.PredictiveStageCostEpisode, 0, len(report.Episodes))}
	for _, episode := range report.Episodes {
		entry := contracts.PredictiveStageCostEpisode{ID: episode.EpisodeID, ReviewID: episode.ReviewID,
			ManifestSHA256: episode.ManifestSHA256, FeatureCutoffAt: episode.FeatureCutoffAt.Format(time.RFC3339Nano),
			ReviewedBy: episode.ReviewedBy, ReviewedAt: episode.ReviewedAt.Format(time.RFC3339Nano),
			SerialDurationMS: episode.SerialDurationMS, DependencyLowerBoundMS: episode.DependencyLowerBoundMS,
			LowerBoundMS: episode.LowerBoundMS, Environments: make([]contracts.PredictiveStageEnvironmentCost, 0, len(episode.Environments)),
			Tests: make([]contracts.PredictiveStageTestCost, 0, len(episode.Tests))}
		for _, environment := range episode.Environments {
			entry.Environments = append(entry.Environments, contracts.PredictiveStageEnvironmentCost{
				PredictiveStageEnvironment: contracts.PredictiveStageEnvironment{Key: environment.Key,
					CapacityUnits: environment.CapacityUnits, SetupDurationMS: environment.SetupDurationMS},
				WorkUnitMS: environment.WorkUnitMS, ResourceLowerBoundMS: environment.ResourceLowerBoundMS})
		}
		for _, test := range episode.Tests {
			entry.Tests = append(entry.Tests, contracts.PredictiveStageTestCost{
				PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: test.ID, Environment: test.Environment,
					ResourceUnits: test.ResourceUnits, ColdStartDurationMS: test.ColdStartDurationMS, Dependencies: test.Dependencies},
				DurationMS: test.DurationMS, Samples: test.Samples, Source: test.Source})
		}
		output.Episodes = append(output.Episodes, entry)
	}

	return output
}
