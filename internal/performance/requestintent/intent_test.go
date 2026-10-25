package requestintent_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestIntentOwnsEvidenceAndRejectsInvalidInwardValues(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"request":"frozen"}`)
	h := sha256.Sum256(payload)
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	d := requestintent.Data{Scope: requestintent.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "principal"}, ReviewSHA256: strings.Repeat("a", 64), CatalogSHA256: strings.Repeat("b", 64), Evidence: []byte(`{"review":"retained"}`), Children: []requestintent.Child{{Request: performance.Identity{Issuer: "argus", ID: "request"}, Attempt: performance.Identity{Issuer: "argus", ID: "attempt"}, EventID: "event", RequestedAt: now, Deadline: now.Add(time.Minute), Payload: payload, SHA256: hex.EncodeToString(h[:]), Reasons: []requestintent.Reason{{MappingID: "mapping", Capability: "capability", Reason: "approved"}}}}}
	intent, err := requestintent.New(d)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Fingerprint() != "6668f1020e5767233e7af47f9b5c7d9a42679b184dd0257ca4d4300ef454b3e5" {
		t.Fatal("version-one fingerprint encoding changed")
	}
	offset := intent.Data()
	offset.Children[0].RequestedAt = now.In(time.FixedZone("offset", 3600))
	canonical, err := requestintent.New(offset)
	if err != nil || canonical.Fingerprint() != intent.Fingerprint() {
		t.Fatalf("timezone changed durable identity: %v", err)
	}
	unicode := intent.Data()
	unicode.Scope.PrincipalID = strings.Repeat("界", 256)
	if _, err := requestintent.New(unicode); err != nil {
		t.Fatalf("rejected valid Unicode scope: %v", err)
	}
	unicode.Scope.PrincipalID += "界"
	if _, err := requestintent.New(unicode); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("overlong Unicode scope accepted")
	}
	copy := intent.Data()
	copy.Children[0].Payload[0] = '!'
	copy.Children[0].Reasons[0].Reason = "changed"
	d.Evidence[0] = '!'
	if intent.Data().Children[0].Payload[0] != '{' || intent.Data().Children[0].Reasons[0].Reason != "approved" || intent.Data().Evidence[0] != '{' {
		t.Fatal("evidence aliases caller memory")
	}
	for _, mutate := range []func(*requestintent.Data){func(d *requestintent.Data) { d.Scope.PrincipalID = "" }, func(d *requestintent.Data) { d.Children[0].SHA256 = "wrong" }, func(d *requestintent.Data) { d.Children[0].Deadline = now }, func(d *requestintent.Data) { d.Children = nil }, func(d *requestintent.Data) { d.Children[0].RequestedAt = now.Add(time.Nanosecond) }} {
		invalid := intent.Data()
		mutate(&invalid)
		if _, err := requestintent.New(invalid); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatalf("invalid input accepted: %v", err)
		}
	}
}
