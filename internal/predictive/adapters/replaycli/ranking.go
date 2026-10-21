package replaycli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunRanking fits and evaluates an offline model, writing no partial report.
func RunRanking(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-ranking", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	budgetMS := flags.Int64("budget-ms", 0, "estimated stage budget in milliseconds")
	seedText := flags.String("seed", "", "unsigned decimal reference random seed")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || *seedText == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-ranking -index <path> -budget-ms <milliseconds> -seed <uint64>")
	}
	seed, err := strconv.ParseUint(*seedText, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ranking seed: %w", err)
	}
	replay, archive, err := loadDataset(*indexPath, true)
	if err != nil {
		return err
	}
	report, err := predictive.TrainRanking(replay, archive, *budgetMS, seed)
	if err != nil {
		return err
	}
	output := exportRanking(report)
	if err := contracts.ValidatePredictiveRankingV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("ranking report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func exportRanking(report predictive.RankingReport) contracts.PredictiveRankingV1 {
	model := report.Model
	output := contracts.PredictiveRankingV1{APIVersion: report.APIVersion, Baselines: exportBaselines(report.Baselines),
		Model: contracts.PredictiveRankingModel{TrainingExamples: model.TrainingExamples, TrainingFailures: model.TrainingFailures,
			Epochs: model.Epochs, LearningRate: model.LearningRate, L2Penalty: model.L2Penalty,
			Intercept: model.Intercept, Coefficients: exportFactors(model.Coefficients)},
		Episodes: make([]contracts.PredictiveRankingEpisode, 0, len(report.Episodes))}
	for _, episode := range report.Episodes {
		comparison := episode.Comparison
		entry := contracts.PredictiveRankingEpisode{ID: episode.ID,
			Ranked: make([]contracts.PredictiveRankedTest, 0, len(episode.Ranked)),
			Comparison: contracts.PredictiveRankingComparison{SelectedTestIDs: comparison.SelectedTestIDs,
				EstimatedSelectedDurationMS: comparison.EstimatedSelectedDurationMS,
				ActualSelectedDurationMS:    comparison.ActualSelectedDurationMS, FullDurationMS: comparison.FullDurationMS,
				FullTestCount: comparison.FullTestCount, FullFailureCount: comparison.FullFailureCount,
				SelectedFailureCount: comparison.SelectedFailureCount, MissedFailureCount: comparison.MissedFailureCount,
				TimeToFirstFailureMS: comparison.TimeToFirstFailureMS}}
		for _, ranked := range episode.Ranked {
			entry.Ranked = append(entry.Ranked, contracts.PredictiveRankedTest{ID: ranked.ID,
				Features: exportFactors(ranked.Features), Contributions: exportFactors(ranked.Contributions),
				RiskScore: ranked.RiskScore, ValueScore: ranked.ValueScore,
				EstimatedDurationMS: ranked.EstimatedDurationMS, Selected: ranked.Selected})
		}
		output.Episodes = append(output.Episodes, entry)
	}

	return output
}

func exportFactors(f predictive.RankingFactors) contracts.PredictiveRankingFactors {
	return contracts.PredictiveRankingFactors{ExplicitImpact: f.ExplicitImpact,
		PriorFailureRate: f.PriorFailureRate, PriorSeen: f.PriorSeen, RecentFailure: f.RecentFailure}
}
