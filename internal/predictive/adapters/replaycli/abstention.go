package replaycli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunAbstention evaluates offline safety and writes no partial report.
func RunAbstention(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-abstention", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-abstention -index <path>")
	}
	replay, archive, err := loadDataset(*indexPath, true)
	if err != nil {
		return err
	}
	report, err := predictive.EvaluateAbstention(replay, archive)
	if err != nil {
		return err
	}
	output := exportAbstention(report)
	if err := contracts.ValidatePredictiveAbstentionV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("abstention report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func exportAbstention(report predictive.AbstentionReport) contracts.PredictiveAbstentionV1 {
	output := contracts.PredictiveAbstentionV1{APIVersion: report.APIVersion,
		CalibrationVersion: report.CalibrationVersion, MinimumBinExamples: report.MinimumBinExamples,
		FitEpisodeIDs: report.FitEpisodeIDs, CalibrationEpisodeIDs: report.CalibrationEpisodeIDs,
		Episodes: make([]contracts.PredictiveAbstentionEpisode, 0, len(report.Episodes))}
	for _, episode := range report.Episodes {
		entry := contracts.PredictiveAbstentionEpisode{ID: episode.ID, Mode: episode.Mode,
			EvidenceState: episode.EvidenceState, CandidateTestIDs: episode.CandidateTestIDs,
			RequiredTestIDs: episode.RequiredTestIDs,
			Tests:           make([]contracts.PredictiveAbstentionTest, 0, len(episode.Tests))}
		for _, test := range episode.Tests {
			entry.Tests = append(entry.Tests, contracts.PredictiveAbstentionTest{
				ID: test.ID, RepositoryKey: test.RepositoryKey, Family: test.Family,
				Probability: test.Probability, CalibrationSource: test.CalibrationSource,
				PriorControls: test.PriorControls, CohortExamples: test.CohortExamples,
				CalibrationBin: test.CalibrationBin, BinExamples: test.BinExamples, Reasons: test.Reasons})
		}
		output.Episodes = append(output.Episodes, entry)
	}

	return output
}
