package replaycli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunPortfolio emits a bounded offline portfolio only after complete validation.
func RunPortfolio(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-portfolio", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	constraintsPath := flags.String("constraints", "", "reviewed stage constraints JSON")
	budget := flags.Int64("budget-ms", 0, "estimated stage budget in milliseconds")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || *constraintsPath == "" || *budget < 1 || *budget > 86400000 || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-portfolio -index <path> -constraints <path> -budget-ms <milliseconds>")
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
	report, err := predictive.SchedulePortfolio(replay, archive, constraints, *budget)
	if err != nil {
		return err
	}
	checksum := sha256.Sum256(data)
	output := exportPortfolio(report, hex.EncodeToString(checksum[:]))
	if err := contracts.ValidatePredictivePortfolioV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("portfolio report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func exportPortfolio(report predictive.PortfolioReport, checksum string) contracts.PredictivePortfolioV1 {
	output := contracts.PredictivePortfolioV1{APIVersion: report.APIVersion, BudgetMS: report.BudgetMS,
		Costs: exportStageCost(report.Costs, checksum), Abstention: exportAbstention(report.Abstention),
		Episodes: make([]contracts.PredictivePortfolioEpisode, 0, len(report.Episodes))}
	for _, episode := range report.Episodes {
		entry := contracts.PredictivePortfolioEpisode{ID: episode.ID, Mode: episode.Mode, FallbackReason: episode.FallbackReason,
			RequiredTestIDs: episode.RequiredTestIDs, OmittedTestIDs: episode.OmittedTestIDs,
			CoveredCapabilities: episode.CoveredCapabilities, EstimatedDurationMS: episode.EstimatedDurationMS,
			OverBudget: episode.OverBudget, Selected: make([]contracts.PredictiveScheduledTest, 0, len(episode.Selected))}
		for _, test := range episode.Selected {
			entry.Selected = append(entry.Selected, contracts.PredictiveScheduledTest{ID: test.ID, Reason: test.Reason, StartMS: test.StartMS, EndMS: test.EndMS})
		}
		output.Episodes = append(output.Episodes, entry)
	}

	return output
}
