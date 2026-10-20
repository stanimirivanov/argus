package predictive_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/predictive"
)

func replayEpisode(id, pull string, minute int) predictive.Episode {
	cutoff := time.Date(2026, 10, 20, 10, minute, 0, 0, time.UTC)

	return predictive.Episode{ID: id, ChangeKey: "change-" + id, SourcePullKey: pull,
		ManifestKey: "manifest-" + id, FilesKey: "files-" + id, Family: "functional-api", FeatureCutoffAt: cutoff,
		Groups: []predictive.GroupReference{{ObservationSHA256: "digest-" + id, Coordinates: "repo-" + id,
			ControlAttemptID: "attempt-" + id, ControlStartedAt: cutoff.Add(time.Minute), ControlCompletedAt: cutoff.Add(2 * time.Minute)}}}
}

func TestReplayOrdersEpisodesAndKeepsHoldoutHistoryBeforeCutoff(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	train := replayEpisode("train", "pull-1", 10)
	train.Groups = append(train.Groups, predictive.GroupReference{ObservationSHA256: "digest-train-b", Coordinates: "repo-b",
		ControlAttemptID: "attempt-b", ControlStartedAt: train.FeatureCutoffAt.Add(time.Minute), ControlCompletedAt: train.FeatureCutoffAt.Add(3 * time.Minute)})
	holdout := replayEpisode("holdout", "pull-2", 40)
	first, err := predictive.BuildReplay([]predictive.Episode{holdout, train}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	second, err := predictive.BuildReplay([]predictive.Episode{train, holdout}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Episodes[0].Split != "train" ||
		first.Episodes[1].Split != "holdout" || !reflect.DeepEqual(first.Episodes[1].History, []string{"train"}) ||
		!reflect.DeepEqual(first.Episodes[0].ObservationSHA256, []string{"digest-train", "digest-train-b"}) {
		t.Fatalf("non-deterministic or leaking replay: %+v", first)
	}
}

func TestReplayExcludesLateLabelsAndRejectsCrossSplitPull(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	late := replayEpisode("late", "pull-1", 10)
	late.Groups[0].ControlCompletedAt = start.Add(time.Second)
	holdout := replayEpisode("holdout", "pull-2", 40)
	replay, err := predictive.BuildReplay([]predictive.Episode{late, holdout}, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if replay.Episodes[0].Split != "excluded" || replay.Episodes[0].ExclusionReason != "control-after-training-cutoff" || len(replay.Episodes[1].History) != 0 {
		t.Fatalf("late label entered history: %+v", replay)
	}
	train := replayEpisode("train", "pull-2", 10)
	if _, err := predictive.BuildReplay([]predictive.Episode{train, holdout}, start, start.Add(time.Hour)); err == nil {
		t.Fatal("source pull request crossed training and holdout")
	}
}

func TestReplayRejectsDuplicateAndIncompleteGroups(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	valid := replayEpisode("one", "pull-1", 10)
	cases := []struct {
		name   string
		mutate func(*predictive.Episode)
	}{
		{"duplicate coordinate", func(e *predictive.Episode) { e.Groups = append(e.Groups, e.Groups[0]) }},
		{"missing group", func(e *predictive.Episode) { e.Groups = nil }},
		{"future feature", func(e *predictive.Episode) { e.Groups[0].ControlStartedAt = e.FeatureCutoffAt.Add(-time.Second) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			episode := valid
			episode.Groups = append([]predictive.GroupReference(nil), valid.Groups...)
			test.mutate(&episode)
			if _, err := predictive.BuildReplay([]predictive.Episode{episode}, start, start.Add(time.Hour)); err == nil {
				t.Fatal("invalid group accepted")
			}
		})
	}
	if _, err := predictive.BuildReplay([]predictive.Episode{valid, valid}, start, start.Add(time.Hour)); err == nil {
		t.Fatal("duplicate decision accepted")
	}
}
