// Package cli owns bounded file input and contract conversion for observation capture.
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/predictive"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

const maxInputBytes = 16 << 20

// Run emits exactly one canonical observation envelope or no output on error.
func Run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-evidence", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	changePath := flags.String("change", "", "validated change-set JSON")
	manifestPath := flags.String("manifest", "", "execution manifest JSON")
	controlPath := flags.String("control", "", "full-suite attempt JSON")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *changePath == "" || *manifestPath == "" || *controlPath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-evidence -change <path> -manifest <path> -control <path>")
	}
	changeBytes, err := readBounded(*changePath)
	if err != nil {
		return fmt.Errorf("read change: %w", err)
	}
	manifestBytes, err := readBounded(*manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	controlBytes, err := readBounded(*controlPath)
	if err != nil {
		return fmt.Errorf("read control: %w", err)
	}
	changeDocument, err := contracts.DecodeChangeSetV1(changeBytes)
	if err != nil {
		return fmt.Errorf("decode change: %w", err)
	}
	changeSet, err := importChange(changeDocument)
	if err != nil {
		return err
	}
	input, err := importObservation(changeSet, manifestBytes, controlBytes)
	if err != nil {
		return err
	}
	observation, err := predictive.Build(input)
	if err != nil {
		return err
	}
	wire := exportObservation(observation)
	envelope, err := contracts.NewPredictiveObservationEnvelopeV1(wire)
	if err != nil {
		return err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(data) > maxInputBytes {
		return fmt.Errorf("observation exceeds %d bytes", maxInputBytes)
	}
	_, err = stdout.Write(append(data, '\n'))

	return err
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Closing a read-only input after the bounded read.
	data, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}

	return data, nil
}

func importChange(doc contracts.ChangeSetV1) (change.Set, error) {
	observed, err := time.Parse(time.RFC3339Nano, doc.ObservedAt)
	if err != nil {
		return change.Set{}, predictive.ErrInvalid
	}
	set := change.Set{APIVersion: doc.APIVersion, SourceRepository: repository(doc.SourceRepository),
		PullRequestNumber: doc.PullRequest.Number, BaseRevision: revision(doc.BaseRevision),
		HeadRevision: revision(doc.HeadRevision), ObservedAt: observed,
		Trigger:        change.Trigger{Provider: catalog.Provider(doc.Trigger.Provider), DeliveryID: doc.Trigger.DeliveryID, Event: doc.Trigger.Event, Action: doc.Trigger.Action},
		FilesTruncated: doc.FilesTruncated, Files: make([]change.File, 0, len(doc.Files))}
	for _, file := range doc.Files {
		set.Files = append(set.Files, change.File{Path: file.Path, PreviousPath: file.PreviousPath,
			Kind: change.Kind(file.Kind), Additions: file.Additions, Deletions: file.Deletions,
			Patch: file.Patch, PatchStatus: change.PatchStatus(file.PatchStatus)})
	}
	if err := change.ValidateSet(set); err != nil {
		return change.Set{}, err
	}

	return set, nil
}

func importObservation(set change.Set, manifestBytes, controlBytes []byte) (predictive.Input, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(manifestBytes, &header); err != nil {
		return predictive.Input{}, err
	}
	switch header.APIVersion {
	case selection.ManifestAPIVersion, selection.ReviewedManifestAPIVersion:
		return importAPI(set, manifestBytes, controlBytes)
	case contracts.BrowserSelectionManifestV1APIVersion:
		return importBrowser(set, manifestBytes, controlBytes)
	default:
		return predictive.Input{}, predictive.ErrInvalid
	}
}

func importAPI(set change.Set, manifestBytes, controlBytes []byte) (predictive.Input, error) {
	manifest, err := selectioncontract.ImportExecutableManifest(manifestBytes)
	if err != nil {
		return predictive.Input{}, err
	}
	digest, err := selectioncontract.ExecutableManifestSHA256(manifest)
	if err != nil {
		return predictive.Input{}, err
	}
	doc, err := contracts.DecodeExecutionAttemptV1(controlBytes)
	if err != nil {
		return predictive.Input{}, err
	}
	attempt, err := executioncontract.ImportAttemptV1(doc)
	if err != nil {
		return predictive.Input{}, err
	}
	input := predictive.Input{Change: set, ManifestChange: manifest.Change,
		Manifest:        execution.ManifestReference{APIVersion: manifest.APIVersion, SHA256: digest},
		FeatureCutoffAt: manifest.Change.ObservedAt, Family: manifest.Family,
		Candidates: make([]predictive.Candidate, 0, len(manifest.Decisions)),
		Control: predictive.Control{AttemptID: attempt.AttemptID, Manifest: attempt.Manifest,
			Stage: attempt.Stage, Repository: attempt.TestRepository, Revision: attempt.TestRevision,
			Adapter: attempt.AdapterID, StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt, Results: attempt.Results}}
	for _, decision := range manifest.Decisions {
		reasons := make([]string, 0, len(decision.Reasons))
		for _, reason := range decision.Reasons {
			reasons = append(reasons, string(reason.Code))
		}
		input.Candidates = append(input.Candidates, predictive.Candidate{Repository: decision.Test.Repository,
			SuiteKey: decision.Test.SuiteKey, TestKey: decision.Test.TestKey, Adapter: decision.Test.Adapter,
			Revision: attempt.TestRevision, Outcome: decision.Outcome, Reasons: reasons,
			Capabilities: decision.Test.Capabilities})
	}

	return input, nil
}

func importBrowser(set change.Set, manifestBytes, controlBytes []byte) (predictive.Input, error) {
	doc, err := contracts.DecodeBrowserSelectionManifestV1(manifestBytes)
	if err != nil {
		return predictive.Input{}, err
	}
	if _, err := executioncontract.ImportBrowserManifest(doc); err != nil {
		return predictive.Input{}, err
	}
	attemptDoc, err := contracts.DecodeBrowserExecutionAttemptV1(controlBytes)
	if err != nil {
		return predictive.Input{}, err
	}
	attempt, err := executioncontract.ImportBrowserAttempt(attemptDoc)
	if err != nil {
		return predictive.Input{}, err
	}
	evaluated, err := time.Parse(time.RFC3339Nano, doc.EvaluatedAt)
	if err != nil {
		return predictive.Input{}, err
	}
	observed, err := time.Parse(time.RFC3339Nano, doc.Change.ObservedAt)
	if err != nil {
		return predictive.Input{}, err
	}
	digest := sha256.Sum256(manifestBytes) // Browser runner binds exact manifest bytes.
	input := predictive.Input{Change: set, ManifestChange: changeReference(doc.Change, observed),
		Manifest:        execution.ManifestReference{APIVersion: doc.APIVersion, SHA256: hex.EncodeToString(digest[:])},
		FeatureCutoffAt: evaluated, Family: catalog.TestFamilyFunctionalUI,
		Candidates: make([]predictive.Candidate, 0, len(doc.Decisions)),
		Control: predictive.Control{AttemptID: attempt.Request.AttemptID, Manifest: attempt.Request.Manifest,
			Stage: attempt.Request.Stage, Repository: attempt.Request.TestRepository,
			Revision: attempt.Request.TestRevision, Adapter: attempt.Request.Adapter,
			Project: attempt.Request.Project, Environment: attempt.Request.Environment,
			StartedAt: attempt.Result.StartedAt, CompletedAt: attempt.Result.CompletedAt, Results: attempt.Result.Results}}
	for _, decision := range doc.Decisions {
		input.Candidates = append(input.Candidates, predictive.Candidate{Repository: repository(decision.TestRepository),
			Revision: revision(decision.Revision), SuiteKey: decision.SuiteKey, TestKey: decision.TestKey,
			Adapter: "playwright", Outcome: selection.Outcome(decision.Outcome),
			Reasons: []string{decision.Reason}, MappingKeys: decision.MappingKeys, Projects: decision.Projects})
	}

	return input, nil
}

func repository(ref contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.Provider(ref.Provider),
		Host: ref.Host, ProviderRepositoryID: ref.ProviderRepositoryID}, Owner: ref.Owner, Name: ref.Name}
}

func revision(ref contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionAlgorithm(ref.Algorithm), Digest: ref.Digest}
}

func changeReference(ref contracts.ManifestChangeReference, observed time.Time) change.Reference {
	return change.Reference{SourceRepository: repository(ref.SourceRepository), PullRequestNumber: ref.PullRequest.Number,
		BaseRevision: revision(ref.BaseRevision), HeadRevision: revision(ref.HeadRevision), ObservedAt: observed,
		Trigger: change.Trigger{Provider: catalog.Provider(ref.Trigger.Provider), DeliveryID: ref.Trigger.DeliveryID,
			Event: ref.Trigger.Event, Action: ref.Trigger.Action}}
}

func exportObservation(value predictive.Observation) contracts.PredictiveObservationV1 {
	ref := value.Change
	project, environment := (*string)(nil), (*string)(nil)
	if value.Project != "" {
		project = &value.Project
	}
	if value.Environment != "" {
		environment = &value.Environment
	}
	doc := contracts.PredictiveObservationV1{APIVersion: value.APIVersion,
		Change: contracts.ManifestChangeReference{SourceRepository: repositoryRef(ref.SourceRepository),
			PullRequest:  contracts.PullRequestReference{Number: ref.PullRequestNumber},
			BaseRevision: revisionRef(ref.BaseRevision), HeadRevision: revisionRef(ref.HeadRevision),
			ObservedAt: ref.ObservedAt.Format(time.RFC3339Nano),
			Trigger: contracts.ChangeTrigger{Provider: string(ref.Trigger.Provider), DeliveryID: ref.Trigger.DeliveryID,
				Event: ref.Trigger.Event, Action: ref.Trigger.Action}},
		Manifest:        contracts.ManifestDigestReference{APIVersion: value.Manifest.APIVersion, SHA256: value.Manifest.SHA256},
		FeatureCutoffAt: value.FeatureCutoffAt.Format(time.RFC3339Nano), Family: string(value.Family),
		TestRepository: repositoryRef(value.Repository), TestRevision: revisionRef(value.Revision),
		Adapter: value.Adapter, Project: project, Environment: environment,
		ControlAttemptID:   value.ControlAttemptID,
		ControlStartedAt:   value.ControlStartedAt.Format(time.RFC3339Nano),
		ControlCompletedAt: value.ControlCompletedAt.Format(time.RFC3339Nano),
		FilesTruncated:     value.FilesTruncated,
		Files:              make([]contracts.PredictiveFileV1, 0, len(value.Files)),
		Tests:              make([]contracts.PredictiveTestV1, 0, len(value.Tests))}
	for _, file := range value.Files {
		doc.Files = append(doc.Files, contracts.PredictiveFileV1{Path: file.Path, Kind: string(file.Kind), Additions: file.Additions, Deletions: file.Deletions, PatchStatus: string(file.PatchStatus)})
	}
	for _, test := range value.Tests {
		doc.Tests = append(doc.Tests, contracts.PredictiveTestV1{SuiteKey: test.SuiteKey, TestKey: test.TestKey,
			Decision: string(test.Decision), Reasons: test.Reasons, Capabilities: test.Capabilities, MappingKeys: test.MappingKeys,
			Outcome: string(test.Outcome), DurationMS: test.DurationMS})
	}

	return doc
}

func repositoryRef(repo catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{Provider: string(repo.Identity.Provider), Host: repo.Identity.Host,
		ProviderRepositoryID: repo.Identity.ProviderRepositoryID, Owner: repo.Owner, Name: repo.Name}
}

func revisionRef(value catalog.Revision) contracts.RevisionReference {
	return contracts.RevisionReference{Algorithm: string(value.Algorithm), Digest: value.Digest}
}
