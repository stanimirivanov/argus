package perfeng

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// LifecycleObservation converts a pinned lifecycle message into an owned inward
// observation after complete frozen-context correlation. The caller must first
// authenticate the independently configured provider/principal; body claims grant
// no authority. Revision conversion preserves integer-valued exponent syntax in
// the retained payload while projecting the schema's exact safe-integer value.
func LifecycleObservation(frozen []byte, reply IntegrationMessage, provider string) (requestintent.LifecycleEvent, error) {
	outcome, err := DispatchOutcome(frozen, reply, provider)
	if err != nil || reply.Type != "dev.perfeng.integration.lifecycle.v1" {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	revision, revisionErr := strconv.ParseFloat(reply.Data.Revision.String(), 64)
	occurred, timeErr := time.Parse(time.RFC3339Nano, reply.Time)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, reply.Data.Context.Deadline)
	if revisionErr != nil || timeErr != nil || deadlineErr != nil {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	digest := sha256.Sum256(frozen)
	event, err := requestintent.NewLifecycleEvent(requestintent.LifecycleData{
		Evidence: outcome, Run: performance.Identity(*reply.Data.Run), RequestSHA256: hex.EncodeToString(digest[:]),
		Revision: int64(revision), State: requestintent.LifecycleState(reply.Data.State), OccurredAt: occurred, Deadline: deadline,
	})
	if err != nil {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}

	return event, nil
}
