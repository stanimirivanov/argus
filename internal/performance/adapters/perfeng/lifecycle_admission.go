package perfeng

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// BindLifecycleAdmission validates the exact retained settlement against the
// frozen request and independently authenticated provider. This keeps vendor wire
// parsing outside storage. Storage must compare the resulting evidence and request
// digest with its scoped authoritative rows before retaining any lifecycle event.
func BindLifecycleAdmission(frozen []byte, retained requestintent.Outcome, provider string) (requestintent.LifecycleAdmission, error) {
	reply, err := DecodeIntegration(retained.Data().Payload)
	if err != nil || (reply.Type != "dev.perfeng.integration.accepted.v1" && reply.Type != "dev.perfeng.integration.lifecycle.v1") {
		return requestintent.LifecycleAdmission{}, ErrIntegration
	}
	checked, err := DispatchOutcome(frozen, reply, provider)
	if err != nil || checked.Fingerprint() != retained.Fingerprint() {
		return requestintent.LifecycleAdmission{}, ErrIntegration
	}
	digest := sha256.Sum256(frozen)
	data := requestintent.LifecycleAdmissionData{Evidence: checked, Run: performance.Identity(*reply.Data.Run), RequestSHA256: hex.EncodeToString(digest[:])}
	if reply.Type == "dev.perfeng.integration.lifecycle.v1" {
		data.Initial, err = LifecycleObservation(frozen, reply, provider)
		if err != nil {
			return requestintent.LifecycleAdmission{}, ErrIntegration
		}
	}
	admission, err := requestintent.NewLifecycleAdmission(data)
	if err != nil {
		return requestintent.LifecycleAdmission{}, ErrIntegration
	}

	return admission, nil
}
