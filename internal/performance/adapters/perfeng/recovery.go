package perfeng

import (
	"context"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// LookupOutcome implements requestintent.RecoveryProvider using independently
// authorized HTTP configuration. It checks retained metadata against frozen wire
// bytes before any network access, including for expired requests. Only lookup is
// performed; absence and transport failures never authorize submission.
func (source IntegrationHTTP) LookupOutcome(ctx context.Context, scope requestintent.Scope, child requestintent.Child) (requestintent.Outcome, error) {
	original, err := DecodeIntegration(child.Payload)
	if err != nil || !requestintent.ValidScope(scope) || !child.Valid() || scope.Issuer != source.Provider || scope.PrincipalIssuer != source.Principal.Issuer || scope.PrincipalID != source.Principal.ID || original.Type != "dev.perfeng.integration.requested.v1" {
		return requestintent.Outcome{}, ErrIntegration
	}
	c := original.Data.Context
	requested, requestedErr := time.Parse(time.RFC3339Nano, c.RequestedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, c.Deadline)
	if requestedErr != nil || deadlineErr != nil || !requested.Equal(child.RequestedAt) || !deadline.Equal(child.Deadline) || performance.Identity(c.Request) != child.Request || performance.Identity(c.Attempt) != child.Attempt || original.ID != child.EventID || c.Principal != source.Principal {
		return requestintent.Outcome{}, ErrIntegration
	}
	reply, err := source.Lookup(ctx, child.Payload)
	if err != nil {
		return requestintent.Outcome{}, err
	}

	return DispatchOutcome(child.Payload, reply, source.Provider)
}
