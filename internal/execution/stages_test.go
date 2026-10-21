package execution_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestScheduledVocabularyDoesNotBroadenV1Attempts(t *testing.T) {
	attempt := execution.Attempt{APIVersion: execution.AttemptAPIVersion, AttemptID: "attempt-1",
		Manifest:       execution.ManifestReference{APIVersion: selection.ManifestAPIVersion, SHA256: strings.Repeat("a", 64)},
		TestRepository: catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests"}, Owner: "example", Name: "tests"},
		TestRevision:   catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)},
		AdapterID:      "playwright", AdapterVersion: "1.0", StartedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
		Outcome: execution.AttemptPassed, Results: []execution.TestResult{{SuiteKey: "api", TestKey: "one", Outcome: execution.TestPassed}}}
	attempt.CompletedAt = attempt.StartedAt.Add(time.Second)
	for _, stage := range []execution.Stage{execution.StageSelected, execution.StageFullSuite} {
		attempt.Stage = stage
		if err := execution.ValidateAttempt(attempt); err != nil {
			t.Fatal(err)
		}
		if err := execution.ValidateScheduledStage(stage); !errors.Is(err, execution.ErrInvalid) {
			t.Fatal("legacy stage accepted as staged")
		}
	}
	for _, stage := range []execution.Stage{execution.StageRelevantNow, execution.StageRemainingLater, execution.StageShadowControl} {
		if err := execution.ValidateScheduledStage(stage); err != nil {
			t.Fatal(err)
		}
		attempt.Stage = stage
		if err := execution.ValidateAttempt(attempt); !errors.Is(err, execution.ErrInvalid) {
			t.Fatalf("v1 attempt accepted %s", stage)
		}
	}
	if execution.ValidateScheduledStage("") == nil {
		t.Fatal("empty stage accepted")
	}
	reference := execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("c", 64)}
	if err := execution.ValidateScheduleReference(reference); err != nil {
		t.Fatal(err)
	}
	reference.SHA256 = strings.Repeat("C", 64)
	if execution.ValidateScheduleReference(reference) == nil {
		t.Fatal("noncanonical digest accepted")
	}
	reference.SHA256 = strings.Repeat("c", 64)
	reference.APIVersion = "argus.dev/unknown/v1"
	if execution.ValidateScheduleReference(reference) == nil {
		t.Fatal("unknown reference version accepted")
	}
}
