package perfeng

import (
	"reflect"
	"strings"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// DispatchOutcome converts validated, correlated provider evidence into owned
// inward values. Provider and provenance must be independently authenticated by
// the caller. A lifecycle snapshot settles admission, not ordered run lifecycle.
func DispatchOutcome(frozen []byte, reply IntegrationMessage, provider string) (requestintent.Outcome, error) {
	original, err := DecodeIntegration(frozen)
	raw, replyErr := EncodeIntegration(reply)
	kind := strings.TrimSuffix(strings.TrimPrefix(reply.Type, "dev.perfeng.integration."), ".v1")
	if err != nil || replyErr != nil || original.Type != "dev.perfeng.integration.requested.v1" || reply.Source != provider || !reflect.DeepEqual(reply.Data.Context, original.Data.Context) || (reply.Data.Run != nil && reply.Data.Run.Issuer != provider) {
		return requestintent.Outcome{}, ErrIntegration
	}
	c := reply.Data.Context
	outcome, err := requestintent.NewOutcome(requestintent.OutcomeData{Request: performance.Identity(c.Request), Attempt: performance.Identity(c.Attempt), Source: provider, EventID: reply.ID, Kind: kind, Payload: raw})
	if err != nil {
		return requestintent.Outcome{}, ErrIntegration
	}

	return outcome, nil
}
