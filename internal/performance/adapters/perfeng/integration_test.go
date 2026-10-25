package perfeng

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func integrationExchange(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("snapshot/integration/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}

	return messages
}

func TestIntegrationArtifactsRemainPinned(t *testing.T) {
	for name, expected := range map[string]string{
		"schema.json":    "75eb03ae3f14c39291fe76285dcced92593157bd1c75f3f337a6ef2f66526960",
		"openapi.json":   "719b9b68cffd0aef9dc5ec5e059917a4df19614c047f22fcfb370a03a9a45d1a",
		"completed.json": "d997ae6d99e194e1fbcbe58b4844903929e6c8af5ca19483cc116c42b2c59c94",
		"cancelled.json": "ecfb16adaac162f1670fdf4a457232b5bbc11d18884c85c4ee9791e59e0b0ab3",
		"rejected.json":  "54894e1efe999ccb38e8a1e620cd66a36eb67f9bbc19474ea6dca970f31bbec2",
	} {
		raw, err := os.ReadFile("snapshot/integration/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != expected {
			t.Errorf("upstream bytes changed: %s", name)
		}
	}
}

func TestIntegrationExchangesRoundTrip(t *testing.T) {
	for _, name := range []string{"completed", "cancelled", "rejected"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, raw := range integrationExchange(t, name) {
				message, err := DecodeIntegration(raw)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := EncodeIntegration(message)
				if err != nil {
					t.Fatal(err)
				}
				again, err := DecodeIntegration(encoded)
				if err != nil || !reflect.DeepEqual(message, again) {
					t.Fatal("wire information lost", err)
				}
			}
		})
	}
}

func TestIntegrationDecodedValuesAreOwned(t *testing.T) {
	raw := integrationExchange(t, "completed")[0]
	message, err := DecodeIntegration(raw)
	if err != nil {
		t.Fatal(err)
	}
	message.Data.Context.Evidence[0].ID = "changed"
	message.Data.Context.Baseline.ID = "changed"
	fresh, err := DecodeIntegration(raw)
	if err != nil || fresh.Data.Context.Evidence[0].ID == "changed" || fresh.Data.Context.Baseline.ID == "changed" {
		t.Fatal("shared mutable decode state", err)
	}
}

func TestIntegrationRejectsInvalidWireWithoutPartialEvidence(t *testing.T) {
	raw := integrationExchange(t, "completed")[0]
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	valid := compact.String()
	for name, invalid := range map[string]string{
		"empty":             "",
		"oversized":         strings.Repeat(" ", IntegrationMaximumBytes+1),
		"trailer":           valid + "null",
		"duplicate":         strings.Replace(valid, `"specversion":`, `"id":"duplicate","specversion":`, 1),
		"escaped duplicate": strings.Replace(valid, `"specversion":`, `"\u0069d":"duplicate","specversion":`, 1),
		"surrogate":         strings.Replace(valid, `"event-0"`, `"\ud800"`, 1),
		"utf8":              valid + string([]byte{0xff}),
		"unknown field":     strings.TrimSuffix(valid, "}") + `,"secret-field":"private-diagnostic"}`,
		"deep":              `{"x":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}`,
		"fractional time":   strings.ReplaceAll(valid, "10:00:00Z", "10:00:00.1234567Z"),
	} {
		t.Run(name, func(t *testing.T) {
			message, err := DecodeIntegration([]byte(invalid))
			if !errors.Is(err, ErrIntegration) || !reflect.DeepEqual(message, IntegrationMessage{}) || strings.Contains(err.Error(), "private-diagnostic") {
				t.Fatal("unsafe or partial result", err)
			}
		})
	}
	for _, field := range []struct{ old, new string }{{`"specversion":"1.0"`, `"specversion":"2.0"`}, {`requested.v1`, `requested.v2`}, {IntegrationSchemaURI, "https://untrusted.example/schema"}} {
		_, err := DecodeIntegration([]byte(strings.Replace(valid, field.old, field.new, 1)))
		if !errors.Is(err, ErrIntegration) || !errors.Is(err, ErrIntegrationVersion) {
			t.Fatal("version not classified", err)
		}
	}
}

func TestIntegrationIdentityTimeAndExactRevisionSemantics(t *testing.T) {
	messages := integrationExchange(t, "completed")
	for _, tc := range []struct {
		name   string
		index  int
		mutate func(*IntegrationMessage)
	}{
		{"equal deadline", 0, func(m *IntegrationMessage) { m.Data.Context.Deadline = m.Data.Context.RequestedAt }},
		{"early event", 0, func(m *IntegrationMessage) { m.Time = "2026-10-21T10:00:00Z" }},
		{"same attempt", 0, func(m *IntegrationMessage) { m.Data.Context.Attempt = m.Data.Context.Request }},
		{"source", 0, func(m *IntegrationMessage) { m.Source = "https://untrusted.example" }},
		{"late acceptance", 1, func(m *IntegrationMessage) { m.Time = m.Data.Context.Deadline }},
		{"unsafe revision", 2, func(m *IntegrationMessage) { n := json.Number("9007199254740992"); m.Data.Revision = &n }},
		{"fractional revision", 2, func(m *IntegrationMessage) { n := json.Number("1.5"); m.Data.Revision = &n }},
		{"invalid outbound unicode", 0, func(m *IntegrationMessage) { m.ID = string([]byte{0xff}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message, err := DecodeIntegration(messages[tc.index])
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&message)
			if raw, err := EncodeIntegration(message); !errors.Is(err, ErrIntegration) || raw != nil {
				t.Fatal("invalid outbound message accepted", err)
			}
		})
	}
	message, err := DecodeIntegration(messages[2])
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"1e0", "9007199254740991"} {
		number := json.Number(value)
		message.Data.Revision = &number
		raw, err := EncodeIntegration(message)
		if err != nil || !bytes.Contains(raw, []byte(`"revision":`+value)) {
			t.Fatal("revision precision changed", err)
		}
	}
	message, err = DecodeIntegration(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	message.Time = "2026-10-22T11:00:00+01:00"
	if _, err := EncodeIntegration(message); err != nil {
		t.Fatal("equivalent offset rejected", err)
	}
	message, err = DecodeIntegration(integrationExchange(t, "cancelled")[2])
	if err != nil {
		t.Fatal(err)
	}
	message.Time = "2026-10-23T10:00:00Z"
	if _, err := EncodeIntegration(message); err != nil {
		t.Fatal("late cancellation rejected", err)
	}
	message, err = DecodeIntegration(messages[len(messages)-1])
	if err != nil {
		t.Fatal(err)
	}
	message.Time = "2026-10-23T10:00:00Z"
	if _, err := EncodeIntegration(message); err != nil {
		t.Fatal("late terminal evidence rejected", err)
	}
}

func TestIntegrationSchemaCannotLoadExternalResources(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	raw, err := json.Marshal(map[string]string{"$id": IntegrationSchemaURI, "$ref": server.URL + "/schema"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileIntegrationSchema(raw); err == nil || calls.Load() != 0 {
		t.Fatal("schema loader escaped embedded resource", err, calls.Load())
	}
}
