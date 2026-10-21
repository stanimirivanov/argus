package execution

import "fmt"

const (
	// StageRelevantNow runs the complete early portfolio for one group.
	StageRelevantNow Stage = "relevant-now"
	// StageRemainingLater runs exactly the complement after relevant-now.
	StageRemainingLater Stage = "remaining-later"
	// StageShadowControl independently runs every candidate during shadow evaluation.
	StageShadowControl Stage = "shadow-control"
	// ScheduleAPIVersion identifies the immutable portfolio artifact being consumed.
	ScheduleAPIVersion = "argus.dev/predictive-portfolio/v1"
)

// ScheduleReference identifies portfolio bytes, not approval or execution authority.
// Transport adapters must verify SHA256 against the actual complete artifact.
type ScheduleReference struct {
	APIVersion string
	SHA256     string
}

// ValidateScheduleReference validates reference shape, without loading its bytes.
func ValidateScheduleReference(reference ScheduleReference) error {
	if reference.APIVersion != ScheduleAPIVersion || !hexSHA256Pattern.MatchString(reference.SHA256) {
		return fmt.Errorf("%w: schedule reference", ErrInvalid)
	}
	return nil
}

// ValidateScheduledStage accepts only the new staged vocabulary. Existing v1
// attempt, request and plan validators continue to allow selected/full-suite only.
func ValidateScheduledStage(stage Stage) error {
	switch stage {
	case StageRelevantNow, StageRemainingLater, StageShadowControl:
		return nil
	default:
		return fmt.Errorf("%w: scheduled stage", ErrInvalid)
	}
}
