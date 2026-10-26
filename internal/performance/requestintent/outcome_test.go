package requestintent

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance"
)

func TestOutcomeOwnsVersionedEvidenceAndRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	data := OutcomeData{Request: performance.Identity{Issuer: "requester", ID: "request"}, Attempt: performance.Identity{Issuer: "requester", ID: "attempt"}, Source: "provider", EventID: "event", Kind: "accepted", Payload: []byte(`{"validated":true}`)}
	first, err := NewOutcome(data)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := NewOutcome(data)
	if err != nil || first.Fingerprint() != replay.Fingerprint() || len(first.Fingerprint()) != 64 {
		t.Fatal("unstable fingerprint", err)
	}
	data.Payload[0] = '!'
	if bytes.Equal(data.Payload, first.Data().Payload) {
		t.Fatal("constructor retained caller bytes")
	}
	data = first.Data()
	for _, change := range []func(*OutcomeData){
		func(d *OutcomeData) { d.Source = "" },
		func(d *OutcomeData) { d.Request.ID = "" },
		func(d *OutcomeData) { d.Attempt = d.Request },
		func(d *OutcomeData) { d.Kind = "completed" },
		func(d *OutcomeData) { d.EventID = " \t" },
		func(d *OutcomeData) { d.Payload = []byte(`{`) },
		func(d *OutcomeData) { d.Payload = bytes.Repeat([]byte{'1'}, (64<<10)+1) },
	} {
		invalid := first.Data()
		change(&invalid)
		if got, err := NewOutcome(invalid); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" {
			t.Fatal("invalid evidence accepted", err)
		}
	}
	data.Kind = "lifecycle"
	changed, err := NewOutcome(data)
	if err != nil || changed.Fingerprint() == first.Fingerprint() {
		t.Fatal("changed evidence unbound", err)
	}
}
