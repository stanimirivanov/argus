package requestintent

import (
	"encoding/json"
	"slices"

	"github.com/stanimirivanov/argus/internal/performance"
)

// OutcomeData is the version-one dispatch settlement projection. Payload is a
// canonical, adapter-validated provider message, not original HTTP response bytes.
type OutcomeData struct {
	Request, Attempt      performance.Identity
	Source, EventID, Kind string
	Payload               []byte
}

// Outcome owns immutable admission/recovery evidence, not ordered run state.
// Only NewOutcome constructs valid values; no value authorizes execution.
type Outcome struct {
	data        OutcomeData
	fingerprint string
}

// NewOutcome checks inward bounds and copies evidence. The driving adapter must
// authenticate provenance and validate the complete frozen wire context first.
func NewOutcome(data OutcomeData) (Outcome, error) {
	if !text(data.Source, 512) || !text(data.EventID, 256) || !text(data.Request.Issuer, 512) || !text(data.Request.ID, 256) || !text(data.Attempt.Issuer, 512) || !text(data.Attempt.ID, 256) || data.Request == data.Attempt || len(data.Payload) > 64<<10 || !json.Valid(data.Payload) {
		return Outcome{}, ErrInvalid
	}
	if data.Kind != "accepted" && data.Kind != "rejected" && data.Kind != "lifecycle" {
		return Outcome{}, ErrInvalid
	}
	raw, err := json.Marshal(struct {
		Version string
		Data    OutcomeData
	}{"argus.performance-dispatch-outcome/v1", data})
	if err != nil {
		return Outcome{}, ErrInvalid
	}
	data.Payload = slices.Clone(data.Payload)

	return Outcome{data: data, fingerprint: sum(raw)}, nil
}

// Fingerprint binds all version-one metadata and canonical provider evidence.
func (o Outcome) Fingerprint() string { return o.fingerprint }

// Data returns an owned copy. Outcomes are safe for concurrent reads.
func (o Outcome) Data() OutcomeData {
	d := o.data
	d.Payload = slices.Clone(d.Payload)

	return d
}
