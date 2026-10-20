package predictive

import (
	"fmt"
	"slices"
	"time"
)

// ReplayVersion identifies the deterministic offline temporal split.
const ReplayVersion = "argus.dev/predictive-replay/v1"

// GroupReference binds one complete control group to its archived observation.
type GroupReference struct {
	ObservationSHA256  string
	Coordinates        string
	ControlAttemptID   string
	ControlStartedAt   time.Time
	ControlCompletedAt time.Time
}

// Episode is one decision-time manifest and every declared executable group.
// Keys are canonical, adapter-produced identities; raw labels remain in the
// referenced observation envelopes.
type Episode struct {
	ID              string
	ChangeKey       string
	SourcePullKey   string
	ManifestKey     string
	FilesKey        string
	Family          string
	FeatureCutoffAt time.Time
	Groups          []GroupReference
}

// ReplayEpisode records a chronological split and the eligible training set.
type ReplayEpisode struct {
	ID                string
	FeatureCutoffAt   time.Time
	ObservationSHA256 []string
	Split             string
	ExclusionReason   string
	History           []string
}

// Replay is a fixed-cutoff holdout. History contains only pre-cutoff labels.
type Replay struct {
	APIVersion    string
	HoldoutStart  time.Time
	HoldoutEnd    time.Time
	TrainCount    int
	HoldoutCount  int
	ExcludedCount int
	Episodes      []ReplayEpisode
}

// BuildReplay validates explicit groups, orders episodes, and rejects temporal
// contamination across a source pull request. It never infers missing groups.
func BuildReplay(episodes []Episode, holdoutStart, holdoutEnd time.Time) (Replay, error) {
	if len(episodes) == 0 || len(episodes) > 1000 || holdoutStart.IsZero() || !holdoutEnd.After(holdoutStart) {
		return Replay{}, fmt.Errorf("%w: replay window", ErrInvalid)
	}
	ordered := append([]Episode(nil), episodes...)
	slices.SortFunc(ordered, func(a, b Episode) int {
		if n := a.FeatureCutoffAt.Compare(b.FeatureCutoffAt); n != 0 {
			return n
		}
		return compare(a.ID, b.ID)
	})
	seenIDs, seenDecisions, seenDigests, seenAttempts := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	result := Replay{APIVersion: ReplayVersion, HoldoutStart: holdoutStart.UTC(), HoldoutEnd: holdoutEnd.UTC(), Episodes: make([]ReplayEpisode, 0, len(ordered))}
	trainIDs := make([]string, 0, len(ordered))
	sourceSplits := map[string]string{}
	for _, episode := range ordered {
		if err := validateEpisode(episode, seenIDs, seenDecisions, seenDigests, seenAttempts); err != nil {
			return Replay{}, err
		}
		entry := classifyEpisode(episode, holdoutStart, holdoutEnd)
		if entry.Split == "train" || entry.Split == "holdout" {
			if prior := sourceSplits[episode.SourcePullKey]; prior != "" && prior != entry.Split {
				return Replay{}, fmt.Errorf("%w: source pull request crosses temporal split", ErrInvalid)
			}
			sourceSplits[episode.SourcePullKey] = entry.Split
		}
		if entry.Split == "train" {
			trainIDs = append(trainIDs, entry.ID)
		}
		result.Episodes = append(result.Episodes, entry)
	}
	finalizeReplay(&result, trainIDs)

	return result, nil
}

func finalizeReplay(replay *Replay, trainIDs []string) {
	for index := range replay.Episodes {
		entry := &replay.Episodes[index]
		entry.History = []string{}
		switch entry.Split {
		case "train":
			replay.TrainCount++
		case "holdout":
			replay.HoldoutCount++
			entry.History = append([]string{}, trainIDs...)
		default:
			replay.ExcludedCount++
		}
	}
}

func classifyEpisode(episode Episode, start, end time.Time) ReplayEpisode {
	entry := ReplayEpisode{ID: episode.ID, FeatureCutoffAt: episode.FeatureCutoffAt.UTC(), ObservationSHA256: make([]string, 0, len(episode.Groups))}
	for _, group := range episode.Groups {
		entry.ObservationSHA256 = append(entry.ObservationSHA256, group.ObservationSHA256)
	}
	slices.Sort(entry.ObservationSHA256)
	switch {
	case episode.FeatureCutoffAt.Before(start):
		entry.Split = "train"
		for _, group := range episode.Groups {
			if !group.ControlCompletedAt.Before(start) {
				entry.Split, entry.ExclusionReason = "excluded", "control-after-training-cutoff"
				break
			}
		}
	case episode.FeatureCutoffAt.Before(end):
		entry.Split = "holdout"
	default:
		entry.Split, entry.ExclusionReason = "excluded", "outside-holdout-window"
	}

	return entry
}

func validateEpisode(episode Episode, ids, decisions, digests, attempts map[string]bool) error {
	if episode.ID == "" || episode.ChangeKey == "" || episode.SourcePullKey == "" || episode.ManifestKey == "" || episode.FilesKey == "" ||
		episode.Family == "" || episode.FeatureCutoffAt.IsZero() || len(episode.Groups) == 0 || len(episode.Groups) > 100 {
		return fmt.Errorf("%w: incomplete episode", ErrInvalid)
	}
	decision := episode.ChangeKey + "\x00" + episode.ManifestKey
	if ids[episode.ID] || decisions[decision] {
		return fmt.Errorf("%w: duplicate decision", ErrInvalid)
	}
	ids[episode.ID], decisions[decision] = true, true
	coordinates := map[string]bool{}
	for _, group := range episode.Groups {
		if group.ObservationSHA256 == "" || group.Coordinates == "" || group.ControlAttemptID == "" ||
			group.ControlStartedAt.Before(episode.FeatureCutoffAt) || group.ControlCompletedAt.Before(group.ControlStartedAt) ||
			digests[group.ObservationSHA256] || attempts[group.ControlAttemptID] || coordinates[group.Coordinates] {
			return fmt.Errorf("%w: duplicate or invalid control group", ErrInvalid)
		}
		digests[group.ObservationSHA256], attempts[group.ControlAttemptID], coordinates[group.Coordinates] = true, true, true
	}

	return nil
}
