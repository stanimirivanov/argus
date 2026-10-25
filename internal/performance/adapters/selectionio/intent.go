package selectionio

import (
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Intent projects successful compilation into owned inward persistence evidence.
// The zero compilation is rejected. It performs no I/O or renewed authorization;
// persistence rechecks new-work deadlines against its own transaction clock.
func (c Compilation) Intent() (requestintent.Intent, error) {
	s := c.Snapshot()
	var selection contracts.PerformanceSelectionV1
	if json.Unmarshal(s.Selection, &selection) != nil {
		return requestintent.Intent{}, requestintent.ErrInvalid
	}
	evidence, err := json.Marshal(struct{ Policy, Selection, Review []byte }{s.Policy, s.Selection, s.Review})
	if err != nil {
		return requestintent.Intent{}, requestintent.ErrInvalid
	}
	d := requestintent.Data{Scope: requestintent.Scope{Issuer: selection.Catalog.Scope.Issuer, PrincipalIssuer: selection.Catalog.Scope.PrincipalIssuer, PrincipalID: selection.Catalog.Scope.PrincipalID}, ReviewSHA256: hash(s.Review), CatalogSHA256: selection.Catalog.Fingerprint, Evidence: evidence}
	for _, child := range s.Children {
		message, err := perfeng.DecodeIntegration(child.Payload)
		if err != nil {
			return requestintent.Intent{}, requestintent.ErrInvalid
		}
		context := message.Data.Context
		requested, err := time.Parse(time.RFC3339Nano, context.RequestedAt)
		if err != nil {
			return requestintent.Intent{}, requestintent.ErrInvalid
		}
		deadline, err := time.Parse(time.RFC3339Nano, context.Deadline)
		if err != nil {
			return requestintent.Intent{}, requestintent.ErrInvalid
		}
		projected := requestintent.Child{Request: performance.Identity(context.Request), Attempt: performance.Identity(context.Attempt), EventID: message.ID, SHA256: child.SHA256, RequestedAt: requested, Deadline: deadline, Payload: child.Payload}
		for _, reason := range child.Reasons {
			projected.Reasons = append(projected.Reasons, requestintent.Reason(reason))
		}
		d.Children = append(d.Children, projected)
	}

	return requestintent.New(d)
}
