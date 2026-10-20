package replaycli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunBaselines compares five offline policies against the fixed temporal split.
func RunBaselines(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-baselines", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	budgetMS := flags.Int64("budget-ms", 0, "estimated stage budget in milliseconds")
	seedText := flags.String("seed", "", "unsigned decimal random baseline seed")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || *seedText == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-baselines -index <path> -budget-ms <milliseconds> -seed <uint64>")
	}
	seed, err := strconv.ParseUint(*seedText, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid baseline seed: %w", err)
	}
	replay, archive, err := loadDataset(*indexPath, true)
	if err != nil {
		return err
	}
	report, err := predictive.CompareBaselines(replay, archive, *budgetMS, seed)
	if err != nil {
		return err
	}
	output := exportBaselines(report)
	if err := contracts.ValidatePredictiveBaselinesV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("baseline report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func exportBaselines(report predictive.BaselineReport) contracts.PredictiveBaselinesV1 {
	output := contracts.PredictiveBaselinesV1{APIVersion: report.APIVersion, BudgetMS: report.BudgetMS,
		Seed: strconv.FormatUint(report.Seed, 10), HoldoutStart: report.HoldoutStart.Format(time.RFC3339Nano),
		HoldoutEnd: report.HoldoutEnd.Format(time.RFC3339Nano),
		Episodes:   make([]contracts.PredictiveBaselineEpisode, 0, len(report.Episodes))}
	for _, episode := range report.Episodes {
		entry := contracts.PredictiveBaselineEpisode{ID: episode.ID,
			Comparisons: make([]contracts.PredictiveBaselineComparison, 0, len(episode.Comparisons))}
		for _, comparison := range episode.Comparisons {
			entry.Comparisons = append(entry.Comparisons, contracts.PredictiveBaselineComparison{Name: comparison.Name,
				SelectedTestIDs:             comparison.SelectedTestIDs,
				EstimatedSelectedDurationMS: comparison.EstimatedSelectedDurationMS,
				ActualSelectedDurationMS:    comparison.ActualSelectedDurationMS, FullDurationMS: comparison.FullDurationMS,
				FullTestCount: comparison.FullTestCount, FullFailureCount: comparison.FullFailureCount,
				SelectedFailureCount: comparison.SelectedFailureCount, MissedFailureCount: comparison.MissedFailureCount,
				TimeToFirstFailureMS: comparison.TimeToFirstFailureMS})
		}
		output.Episodes = append(output.Episodes, entry)
	}

	return output
}
