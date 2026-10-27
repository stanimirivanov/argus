package perfeng

import (
	"encoding/json"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func lifecycleReply(t *testing.T, raw []byte) IntegrationMessage {
	t.Helper()
	reply, err := DecodeIntegration(raw)
	if err != nil {
		t.Fatal(err)
	}

	return reply
}

func TestLifecyclePublishedExchangesProjectIndependentlyOfArrivalOrder(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"completed", "cancelled"} {
		messages := integrationExchange(t, name)
		var history []requestintent.LifecycleEvent
		for _, raw := range messages[1:] {
			reply := lifecycleReply(t, raw)
			if reply.Type != "dev.perfeng.integration.lifecycle.v1" {
				continue
			}
			event, err := LifecycleObservation(messages[0], reply, reply.Source)
			if err != nil {
				t.Fatal(err)
			}
			history = append(history, event)
		}
		for i := range history {
			history[0], history[i] = history[i], history[0]
			got, err := requestintent.ProjectLifecycle(history)
			if err != nil || string(got.Data().State) != map[string]string{"completed": "COMPLETED", "cancelled": "CANCELLED"}[name] {
				t.Fatal("exchange projection", name, err)
			}
		}
	}
}

func TestLifecycleConversionPreservesLateEvidenceAndExponentRevisions(t *testing.T) {
	messages := integrationExchange(t, "completed")
	reply := lifecycleReply(t, messages[3])
	revision := json.Number("1e0")
	reply.Data.Revision, reply.Time = &revision, reply.Data.Context.Deadline
	event, err := LifecycleObservation(messages[0], reply, reply.Source)
	if err != nil || !event.Late() || event.Data().Revision != 1 {
		t.Fatal("late/exponent evidence rejected", err)
	}
	retained := lifecycleReply(t, event.Data().Evidence.Data().Payload)
	if retained.Data.Revision.String() != "1e0" {
		t.Fatal("revision syntax lost")
	}
	duplicate := reply
	duplicate.ID = "redelivery"
	other, err := LifecycleObservation(messages[0], duplicate, duplicate.Source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requestintent.ProjectLifecycle([]requestintent.LifecycleEvent{event, other, event}); err != nil {
		t.Fatal("unchanged terminal snapshot rejected", err)
	}
	for _, change := range []func(*IntegrationMessage){
		func(m *IntegrationMessage) { m.Source = "https://wrong.example" },
		func(m *IntegrationMessage) { m.Data.Run.ID = "other"; m.Data.Context.Principal.ID = "other" },
		func(m *IntegrationMessage) { m.Data.Context.Workload.ID = "other" },
		func(m *IntegrationMessage) { m.Data.Revision = nil },
	} {
		invalid := lifecycleReply(t, messages[3])
		change(&invalid)
		if got, err := LifecycleObservation(messages[0], invalid, reply.Source); err == nil || got.Data().Evidence.Fingerprint() != "" {
			t.Fatal("untrusted/invalid evidence accepted", err)
		}
	}
	for _, raw := range messages[:2] {
		invalid := lifecycleReply(t, raw)
		if _, err := LifecycleObservation(messages[0], invalid, invalid.Source); err == nil {
			t.Fatal("non-lifecycle evidence projected")
		}
	}
}
