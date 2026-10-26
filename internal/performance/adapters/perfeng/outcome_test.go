package perfeng

import (
	"bytes"
	"errors"
	"testing"
)

func assertDispatchOutcome(t *testing.T, frozen []byte, reply IntegrationMessage) {
	t.Helper()
	outcome, err := DispatchOutcome(frozen, reply, reply.Source)
	if err != nil || outcome.Fingerprint() == "" {
		t.Fatal("valid outcome", err)
	}
	data := outcome.Data()
	canonical, err := EncodeIntegration(reply)
	if err != nil || !bytes.Equal(data.Payload, canonical) || data.EventID != reply.ID || data.Source != reply.Source || "dev.perfeng.integration."+data.Kind+".v1" != reply.Type || data.Request.ID != reply.Data.Context.Request.ID || data.Attempt.ID != reply.Data.Context.Attempt.ID {
		t.Fatal("conversion changed provider evidence", err)
	}
	data.Payload[0] = '!'
	if bytes.Equal(data.Payload, outcome.Data().Payload) {
		t.Fatal("aliased evidence")
	}
	if _, err := DispatchOutcome(frozen, reply, "https://wrong.example"); err == nil {
		t.Fatal("wrong provider accepted")
	}
	reply.Data.Context.Principal.ID += "-changed"
	if _, err := DispatchOutcome(frozen, reply, reply.Source); err == nil {
		t.Fatal("changed full context accepted")
	}
}

func TestDispatchOutcomeValidatesCorrelationAndOwnsEvidence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"completed", "rejected"} {
		messages := integrationExchange(t, name)
		for _, raw := range messages[1:] {
			reply, err := DecodeIntegration(raw)
			if err != nil {
				t.Fatal(err)
			}
			assertDispatchOutcome(t, messages[0], reply)
		}
	}
}

func TestDispatchOutcomeRejectsUntrustedOrMalformedMessages(t *testing.T) {
	t.Parallel()
	messages := integrationExchange(t, "completed")
	requested, err := DecodeIntegration(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DispatchOutcome(messages[0], requested, requested.Source); err == nil {
		t.Fatal("request settled dispatch")
	}
	for _, change := range []func(*IntegrationMessage){
		func(m *IntegrationMessage) { m.Source = "https://wrong.example" },
		func(m *IntegrationMessage) { m.Data.Run.Issuer = "https://wrong.example" },
		func(m *IntegrationMessage) { m.SpecVersion = "9.0" },
		func(m *IntegrationMessage) { m.Data.Context.Workload.SHA256 = string(bytes.Repeat([]byte{'0'}, 64)) },
	} {
		reply, err := DecodeIntegration(messages[1])
		if err != nil {
			t.Fatal(err)
		}
		change(&reply)
		if got, err := DispatchOutcome(messages[0], reply, "https://provider.example/identity"); !errors.Is(err, ErrIntegration) || got.Fingerprint() != "" {
			t.Fatal("bad evidence accepted", err)
		}
	}
	reply, err := DecodeIntegration(messages[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range [][]byte{[]byte(`{`), messages[1]} {
		if got, err := DispatchOutcome(original, reply, reply.Source); !errors.Is(err, ErrIntegration) || got.Fingerprint() != "" {
			t.Fatal("bad original accepted", err)
		}
	}
}
