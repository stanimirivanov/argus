package selectionio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func decisionFixture(t *testing.T) requestintent.DecisionSnapshot {
	t.Helper()
	evidence, policy, review, authority := requestReview(t, false)
	compiled, err := selectionio.Compile(evidence, policy, review, authority, observed.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := compiled.Intent()
	if err != nil {
		t.Fatal(err)
	}
	data := intent.Data()
	child := data.Children[0]
	reply, err := perfeng.DecodeIntegration(child.Payload)
	if err != nil {
		t.Fatal(err)
	}
	reply.Type, reply.Source, reply.ID = "dev.perfeng.integration.accepted.v1", data.Scope.Issuer, "accepted"
	reply.Time = child.RequestedAt.Add(time.Second).Format(time.RFC3339Nano)
	reply.Data.Run = &perfeng.IntegrationIdentity{Issuer: data.Scope.Issuer, ID: "run"}
	outcome, err := perfeng.DispatchOutcome(child.Payload, reply, data.Scope.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := perfeng.BindLifecycleAdmission(child.Payload, outcome, data.Scope.Issuer)
	if err != nil {
		t.Fatal(err)
	}

	return requestintent.DecisionSnapshot{Intent: intent, Child: child, Admission: admission}
}

func TestDecisionExportPreservesEmptyLateConflictAndIndependentOutcomes(t *testing.T) {
	snapshot := decisionFixture(t)
	scope := snapshot.Intent.Data().Scope
	project := func() []byte {
		raw, err := selectionio.ProjectDecision(t.Context(), snapshot, scope, snapshot.Intent.Fingerprint(), snapshot.Child.SHA256)
		if err != nil {
			t.Fatal(err)
		}

		return raw
	}
	var public contracts.PerformanceDecisionProjectionV1
	if json.Unmarshal(project(), &public) != nil || len(public.Analysis.Decisions) != 0 || public.Lifecycle != nil || public.Run.ID != "run" {
		t.Fatal("known empty projection lost")
	}
	first := requestintent.AnalysisDecision{EventID: "first", EvidenceFingerprint: strings.Repeat("a", 64), ReportSHA256: strings.Repeat("b", 64), PublishedAt: snapshot.Child.Deadline, Late: true,
		Rules: []requestintent.AnalysisRuleDecision{{RuleID: "latency", Metric: "duration", Statistic: "p95", Unit: "ms", Quality: requestintent.AnalysisOutcome{Status: "PASS"}, SLO: requestintent.AnalysisOutcome{Status: "FAIL", Reasons: []string{"over-budget"}}, Regression: requestintent.AnalysisOutcome{Status: "INCONCLUSIVE", Reasons: []string{"uncertain"}}}}}
	second := first
	second.EventID = "second"
	second.ReportSHA256 = strings.Repeat("c", 64)
	second.PublishedAt = second.PublishedAt.Add(time.Second)
	snapshot.Analysis = requestintent.AnalysisProjection{Conflict: true, Decisions: []requestintent.AnalysisDecision{first, second}}
	raw := project()
	if json.Unmarshal(raw, &public) != nil || !public.Analysis.Conflict || len(public.Analysis.Decisions) != 2 || !public.Analysis.Decisions[0].Late || public.Analysis.Decisions[0].Rules[0].SLO.Status != "FAIL" {
		t.Fatal("independent evidence lost")
	}
	for _, secret := range []string{`"Payload"`, `"Review"`, `"Bindings"`, `"position"`, `"reviewer"`} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("private field disclosed")
		}
	}
	public.Provenance.Change.SourceRepository.Name = strings.Repeat("x", 8<<20)
	if raw, err := contracts.EncodePerformanceDecisionProjectionV1(public); err == nil || raw != nil {
		t.Fatal("complete byte budget ignored")
	}
}

func TestDecisionExportRejectsUnapprovedAndCorruptInputsWithoutOutput(t *testing.T) {
	snapshot := decisionFixture(t)
	scope := snapshot.Intent.Data().Scope
	wrong := scope
	wrong.PrincipalID += "-other"
	for _, check := range []struct {
		scope           requestintent.Scope
		intent, request string
	}{
		{wrong, snapshot.Intent.Fingerprint(), snapshot.Child.SHA256},
		{scope, "wrong", snapshot.Child.SHA256}, {scope, snapshot.Intent.Fingerprint(), "wrong"},
	} {
		if raw, err := selectionio.ProjectDecision(t.Context(), snapshot, check.scope, check.intent, check.request); err == nil || raw != nil {
			t.Fatal("authorization mismatch disclosed output")
		}
	}
	bad := snapshot
	bad.Admission = requestintent.LifecycleAdmission{}
	if raw, err := selectionio.ProjectDecision(t.Context(), bad, scope, snapshot.Intent.Fingerprint(), snapshot.Child.SHA256); err == nil || raw != nil {
		t.Fatal("missing authoritative admission")
	}
	data := snapshot.Intent.Data()
	data.Evidence = []byte(`{}`)
	var err error
	bad.Intent, err = requestintent.New(data)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := selectionio.ProjectDecision(t.Context(), bad, scope, bad.Intent.Fingerprint(), snapshot.Child.SHA256); err == nil || raw != nil {
		t.Fatal("corrupt provenance")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if raw, err := selectionio.ProjectDecision(ctx, snapshot, scope, snapshot.Intent.Fingerprint(), snapshot.Child.SHA256); err == nil || raw != nil {
		t.Fatal("cancelled output")
	}
}
