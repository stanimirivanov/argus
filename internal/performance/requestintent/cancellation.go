package requestintent

import (
	"encoding/json"
	"slices"
)

// CancellationData binds canonical adapter-validated intent to frozen request bytes.
// It is neither provider acknowledgement nor execution/termination authority.
type CancellationData struct {
	RequestSHA256, EventID string
	Payload                []byte
}

// Cancellation owns immutable cancellation evidence, separate from settlement.
type Cancellation struct {
	data        CancellationData
	fingerprint string
}

// CancellationVerifier must be pure, bounded and concurrent-safe, validating full
// frozen context and canonical cancellation bytes without I/O or granting authority.
type CancellationVerifier func(frozen, cancellation []byte) (Cancellation, error)

// NewCancellation validates inward bounds and takes an owned payload copy.
func NewCancellation(data CancellationData) (Cancellation, error) {
	if !digest(data.RequestSHA256) || !text(data.EventID, 256) || len(data.Payload) > 64<<10 || !json.Valid(data.Payload) {
		return Cancellation{}, ErrInvalid
	}
	raw, err := json.Marshal(struct {
		Version string
		Data    CancellationData
	}{"argus.performance-cancellation/v1", data})
	if err != nil {
		return Cancellation{}, ErrInvalid
	}
	data.Payload = slices.Clone(data.Payload)

	return Cancellation{data: data, fingerprint: sum(raw)}, nil
}

// Fingerprint binds the versioned metadata and canonical intent bytes.
func (c Cancellation) Fingerprint() string { return c.fingerprint }

// Data returns owned bytes; Cancellation values support concurrent reads.
func (c Cancellation) Data() CancellationData {
	d := c.data
	d.Payload = slices.Clone(d.Payload)

	return d
}
