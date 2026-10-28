package perfeng

import (
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCancellationBindingOwnsCanonicalEvidenceAndRejectsChangedContext(t *testing.T) {
	messages := integrationExchange(t, "cancelled")
	intent, err := BindCancellation(messages[0], messages[2])
	if err != nil {
		t.Fatal(err)
	}
	d := intent.Data()
	d.Payload[0] = '!'
	if intent.Data().Payload[0] != '{' {
		t.Fatal("aliased cancellation")
	}
	checked, err := BindCancellation(messages[0], intent.Data().Payload)
	if err != nil || checked.Fingerprint() != intent.Fingerprint() {
		t.Fatal("noncanonical binding", err)
	}
	for _, raw := range [][]byte{messages[0], messages[1], []byte(strings.ReplaceAll(string(messages[2]), "attempt-1", "another-attempt"))} {
		if got, err := BindCancellation(messages[0], raw); err == nil || got.Fingerprint() != "" {
			t.Fatal("changed context/kind bound", err)
		}
	}
	if _, err := requestintent.NewCancellation(requestintent.CancellationData{}); err == nil {
		t.Fatal("empty inward evidence accepted")
	}
	if _, err := requestintent.NewCancellation(requestintent.CancellationData{RequestSHA256: strings.Repeat("a", 64), EventID: "event", Payload: []byte("invalid")}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
