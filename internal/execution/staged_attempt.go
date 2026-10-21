package execution

import (
	"fmt"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// StagedAttemptAPIVersion is opt-in functional API staged evidence.
const StagedAttemptAPIVersion = "argus.dev/execution-attempt/v2"

// Staging pins the immutable schedule and reviewed invocation scope. It grants
// no promotion authority; complete-plan evaluation remains a separate operation.
type Staging struct {
	Schedule                               ScheduleReference
	EpisodeID, GroupKey, Environment, Mode string
}

// ValidateStaging rejects missing provenance and incompatible mode/stage pairs.
func ValidateStaging(value *Staging, stage Stage) error {
	if value == nil || ValidateScheduleReference(value.Schedule) != nil ||
		!catalog.IsLocalKey(value.EpisodeID) || !catalog.IsLocalKey(value.GroupKey) || !catalog.IsLocalKey(value.Environment) ||
		ValidateScheduledStage(stage) != nil ||
		(value.Mode != "shadow" && value.Mode != "staged") ||
		(value.Mode == "shadow" && stage == StageRemainingLater) ||
		(value.Mode == "staged" && stage == StageShadowControl) {
		return fmt.Errorf("%w: staged invocation provenance", ErrInvalid)
	}

	return nil
}

// ValidateRecordedAttempt accepts the two explicitly supported storage versions.
// Legacy consumers continue to call ValidateAttempt and reject v2 evidence.
func ValidateRecordedAttempt(attempt Attempt) error {
	if attempt.APIVersion == AttemptAPIVersion {
		return ValidateAttempt(attempt)
	}
	if attempt.APIVersion != StagedAttemptAPIVersion || ValidateStaging(attempt.Staging, attempt.Stage) != nil {
		return fmt.Errorf("%w: staged attempt", ErrInvalid)
	}
	// Evidence invariants are identical; only the versioned envelope differs.
	attempt.APIVersion, attempt.Stage, attempt.Staging = AttemptAPIVersion, StageSelected, nil

	return ValidateAttempt(attempt)
}
