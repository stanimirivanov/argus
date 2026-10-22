package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/predictive"
)

type safetyArchive struct {
	root  string
	bytes int
}

// RunSafety emits one complete as-of report or no output on invalid evidence.
// Archive paths are bounded, relative and confined after resolving symlinks.
func RunSafety(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-safety", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("index", "", "explicit safety archive index")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-safety -index <path>")
	}
	report, inputSHA, err := loadSafetySnapshot(*path)
	if err != nil {
		return err
	}
	output := exportSafety(report, inputSHA)
	if err := contracts.ValidatePredictiveSafetyReportV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > maxInputBytes {
		return predictive.ErrInvalid
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func safetySHA(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func safetyWindow(index contracts.PredictiveSafetyIndexV1) (predictive.SafetyWindow, error) {
	start, err := time.Parse(time.RFC3339Nano, index.WindowStart)
	if err != nil {
		return predictive.SafetyWindow{}, err
	}
	end, err := time.Parse(time.RFC3339Nano, index.WindowEnd)
	if err != nil {
		return predictive.SafetyWindow{}, err
	}
	asOf, err := time.Parse(time.RFC3339Nano, index.AsOf)

	return predictive.SafetyWindow{Start: start, End: end, AsOf: asOf}, err
}

func (a *safetyArchive) read(ref contracts.SafetyFileReference) ([]byte, error) {
	if filepath.IsAbs(ref.Path) || filepath.VolumeName(ref.Path) != "" || strings.Contains(ref.Path, "\\") {
		return nil, predictive.ErrInvalid
	}
	target, err := filepath.EvalSymlinks(filepath.Join(a.root, filepath.FromSlash(ref.Path)))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(a.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, predictive.ErrInvalid
	}
	data, err := readBounded(target)
	if err != nil {
		return nil, err
	}
	a.bytes += len(data)
	if a.bytes > 256<<20 || safetySHA(data) != ref.SHA256 {
		return nil, predictive.ErrInvalid
	}

	return data, nil
}

func (a *safetyArchive) loadSafetyRun(run contracts.PredictiveSafetyRun) ([]predictive.SafetyInput, error) {
	schedule, err := a.read(run.Schedule)
	if err != nil {
		return nil, err
	}
	portfolio, err := contracts.DecodePredictivePortfolioV1(schedule)
	if err != nil {
		return nil, err
	}
	manifest, err := a.read(run.Manifest)
	if err != nil {
		return nil, err
	}
	bindings, err := a.read(run.Bindings)
	if err != nil {
		return nil, err
	}
	planBytes, err := a.read(run.Plan)
	if err != nil {
		return nil, err
	}
	plan, policy, err := a.safetyPlan(run, schedule, manifest, bindings, planBytes)
	if err != nil {
		return nil, err
	}
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(manifest, &header); err != nil {
		return nil, err
	}
	inputs, err := safetyInputs(run, plan, policy, header.APIVersion, portfolio)
	if err != nil {
		return nil, err
	}
	if err := bindSafetyNames(inputs, manifest); err != nil {
		return nil, err
	}
	if err := a.safetyCalibration(run, inputs, portfolio); err != nil {
		return nil, err
	}
	for _, ref := range run.Attempts {
		data, err := a.read(ref)
		if err != nil {
			return nil, err
		}
		attempt, err := decodeSafetyAttempt(data)
		if err != nil {
			return nil, err
		}
		if err := bindSafetyAttempt(inputs, attempt); err != nil {
			return nil, err
		}
	}

	return inputs, nil
}

func safetyInputs(run contracts.PredictiveSafetyRun, plan scheduledplan.Plan, policy, manifestVersion string, portfolio contracts.PredictivePortfolioV1) ([]predictive.SafetyInput, error) {
	var cost contracts.PredictiveStageCostEpisode
	for _, c := range portfolio.Costs.Episodes {
		if c.ID == run.EpisodeID {
			cost = c
		}
	}
	costs := map[string]contracts.PredictiveStageTestCost{}
	for _, c := range cost.Tests {
		costs[c.ID] = c
	}
	inputs := []predictive.SafetyInput{}
	cutoff, err := time.Parse(time.RFC3339Nano, cost.FeatureCutoffAt)
	if err != nil {
		return nil, err
	}
	for _, group := range plan.Groups {
		input := predictive.SafetyInput{FeatureCutoffAt: cutoff, RunID: run.ID, EpisodeID: run.EpisodeID, GroupKey: group.GroupKey, PlanSHA256: run.Plan.SHA256, PolicyVersion: policy, Schedule: plan.Schedule, Manifest: execution.ManifestReference{APIVersion: manifestVersion, SHA256: plan.ManifestSHA256}, Mode: run.Mode,
			Scope: predictive.SafetyScope{Repository: group.TestRepository, Revision: group.TestRevision, Family: group.Family, Adapter: group.Adapter, Project: group.Project, Environment: group.Environment}, Candidates: []predictive.SafetyCandidate{}}
		if run.Discovery != nil {
			input.DiscoverySHA256 = run.Discovery.SHA256
		}
		for _, env := range cost.Environments {
			if env.Key == group.Environment {
				input.SetupMS, input.CapacityUnits = env.SetupDurationMS, env.CapacityUnits
			}
		}
		early := map[string]bool{}
		for _, id := range group.RelevantNow {
			early[id] = true
		}
		for _, test := range group.Candidates {
			c, exists := costs[test.ID]
			if !exists {
				return nil, predictive.ErrInvalid
			}
			input.Candidates = append(input.Candidates, predictive.SafetyCandidate{ID: test.ID, SuiteKey: test.SuiteKey, TestKey: test.TestKey, Early: early[test.ID], DurationMS: c.DurationMS, ResourceUnits: c.ResourceUnits})
		}
		inputs = append(inputs, input)
	}

	return inputs, nil
}

func bindSafetyAttempt(inputs []predictive.SafetyInput, attempt predictive.SafetyAttempt) error {
	for i := range inputs {
		input := &inputs[i]
		if input.GroupKey != attempt.Staging.GroupKey {
			continue
		}
		switch attempt.Stage {
		case "relevant-now":
			if input.Early != nil {
				return predictive.ErrInvalid
			}
			input.Early = &attempt
		case "remaining-later", "shadow-control":
			if input.Final != nil {
				return predictive.ErrInvalid
			}
			input.Final = &attempt
		default:
			return predictive.ErrInvalid
		}

		return nil
	}

	return predictive.ErrInvalid
}

func (a *safetyArchive) loadSafetyInputs(declarations []contracts.PredictiveSafetyRun) ([]predictive.SafetyInput, error) {
	inputs := []predictive.SafetyInput{}
	runs, episodes := map[string]bool{}, map[string]bool{}
	count := 0
	for _, run := range declarations {
		key := run.Schedule.SHA256 + "\x00" + run.EpisodeID
		if runs[run.ID] || episodes[key] {
			return nil, predictive.ErrInvalid
		}
		runs[run.ID], episodes[key] = true, true
		groups, err := a.loadSafetyRun(run)
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", run.ID, err)
		}
		for _, g := range groups {
			count += len(g.Candidates)
		}
		if count > 100000 {
			return nil, predictive.ErrInvalid
		}
		inputs = append(inputs, groups...)
	}

	return inputs, nil
}

func loadSafetySnapshot(path string) (predictive.SafetyReport, string, error) {
	data, err := readBounded(path)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	index, err := contracts.DecodePredictiveSafetyIndexV1(data)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	window, err := safetyWindow(index)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	archive := safetyArchive{root: root}
	inputs, err := archive.loadSafetyInputs(index.Runs)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}
	report, err := predictive.Monitor(window, inputs)
	if err != nil {
		return predictive.SafetyReport{}, "", err
	}

	return report, safetySHA(data), nil
}
