package perfeng

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// BindCancellation validates/canonicalizes the pinned intent against the full
// frozen request. Callers authenticate scope/approval independently; wire claims
// grant no authority. This pure binding is also the persistence readback verifier.
func BindCancellation(frozen, raw []byte) (requestintent.Cancellation, error) {
	original, err := DecodeIntegration(frozen)
	if err != nil || original.Type != "dev.perfeng.integration.requested.v1" {
		return requestintent.Cancellation{}, ErrIntegration
	}
	intent, err := DecodeIntegration(raw)
	if err != nil || intent.Type != "dev.perfeng.integration.cancel-requested.v1" || intent.ID == original.ID || !reflect.DeepEqual(intent.Data.Context, original.Data.Context) {
		return requestintent.Cancellation{}, ErrIntegration
	}
	payload, err := EncodeIntegration(intent)
	if err != nil {
		return requestintent.Cancellation{}, ErrIntegration
	}
	digest := sha256.Sum256(frozen)

	return requestintent.NewCancellation(requestintent.CancellationData{RequestSHA256: hex.EncodeToString(digest[:]), EventID: intent.ID, Payload: payload})
}
