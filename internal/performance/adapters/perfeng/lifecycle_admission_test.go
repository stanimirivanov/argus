package perfeng

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestLifecycleAdmissionBindsExactRetainedRunAndInitialSnapshot(t *testing.T) {
	t.Parallel()
	messages := integrationExchange(t, "completed")
	for _, raw := range messages[1:] {
		reply := lifecycleReply(t, raw)
		retained, err := DispatchOutcome(messages[0], reply, reply.Source)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := BindLifecycleAdmission(messages[0], retained, reply.Source)
		if err != nil || bound.Data().Run.ID != reply.Data.Run.ID || bound.Data().Evidence.Fingerprint() != retained.Fingerprint() || (bound.Data().Initial.Data().Evidence.Fingerprint() != "") != (reply.Data.Revision != nil) {
			t.Fatal("admission binding", err)
		}
		copied := bound.Data().Evidence.Data()
		copied.Payload[0] = '!'
		if bound.Data().Evidence.Data().Payload[0] != '{' {
			t.Fatal("aliased admission")
		}
		for _, invalid := range []requestintent.Outcome{requestintent.Outcome{}, changedAdmission(t, retained)} {
			if got, err := BindLifecycleAdmission(messages[0], invalid, reply.Source); err == nil || got.Data().Evidence.Fingerprint() != "" {
				t.Fatal("forged admission accepted", err)
			}
		}
		if _, err := BindLifecycleAdmission(messages[0], retained, "https://wrong.example"); err == nil {
			t.Fatal("wrong provider accepted")
		}
	}
	rejected := integrationExchange(t, "rejected")
	reply := lifecycleReply(t, rejected[1])
	outcome, err := DispatchOutcome(rejected[0], reply, reply.Source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindLifecycleAdmission(rejected[0], outcome, reply.Source); err == nil {
		t.Fatal("rejection bound to a run")
	}
}

func changedAdmission(t *testing.T, retained requestintent.Outcome) requestintent.Outcome {
	t.Helper()
	data := retained.Data()
	data.EventID += "-forged"
	changed, err := requestintent.NewOutcome(data)
	if err != nil {
		t.Fatal(err)
	}

	return changed
}
