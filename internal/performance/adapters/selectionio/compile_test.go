package selectionio_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
)

func requestReview(t *testing.T, multiple bool) (selectionio.Evidence, []byte, selectionio.RequestReview, selectionio.RequestAuthority) {
	t.Helper()
	evidence, policy := reviewed(t)
	if multiple {
		impact, stored, expected := sources(t, "v1")
		impact.Documents[0].Operations[0].Capabilities = append(impact.Documents[0].Operations[0].Capabilities, "second-capability")
		var err error
		evidence, err = selectionio.Inspect(impact, stored, expected)
		if err != nil {
			t.Fatal(err)
		}
		context, err := selectionio.Context(evidence)
		if err != nil {
			t.Fatal(err)
		}
		policy.ImpactSHA256 = context.Impact.SHA256
		policy.HeadCapabilities = append(policy.HeadCapabilities, "second-capability")
		mapping := policy.Mappings[0]
		mapping.ID, mapping.Capability = "second-mapping", "second-capability"
		policy.Mappings = append(policy.Mappings, mapping)
	}
	raw := encode(t, policy)
	selected, err := selectionio.Evaluate(evidence, raw, observed)
	if err != nil {
		t.Fatal(err)
	}
	d := selected.Decisions[0]
	ref := func(r contracts.PerformanceReference) perfeng.IntegrationReference {
		return perfeng.IntegrationReference(r)
	}
	service := ref(*d.Deployment.Reference)
	service.Issuer, service.ID = d.Deployment.Service.Issuer, d.Deployment.Service.ID
	principal := perfeng.IntegrationIdentity{Issuer: selected.Catalog.Scope.PrincipalIssuer, ID: selected.Catalog.Scope.PrincipalID}
	issuer := "https://argus.example/identity"
	context := perfeng.IntegrationContext{
		Request: perfeng.IntegrationIdentity{Issuer: issuer, ID: "request-1"}, Attempt: perfeng.IntegrationIdentity{Issuer: issuer, ID: "attempt-1"}, Principal: principal,
		RequestedAt: observed.Format(time.RFC3339Nano), Deadline: observed.Add(10 * time.Minute).Format(time.RFC3339Nano),
		Service: service, Capability: service, Deployment: ref(*d.Deployment.Reference), Environment: ref(d.Target.Environment), Workload: ref(d.Target.Workload),
		Profile: ref(d.Target.Profile), Policy: ref(d.Target.Policy), Evidence: []perfeng.IntegrationReference{ref(selected.Catalog.Reference)},
	}
	baseline := ref(*d.Target.Baseline)
	context.Baseline = &baseline
	context.Capability.ID = "provider-checkout-capability"
	context.Candidate.Source.Algorithm, context.Candidate.Source.Digest = d.Deployment.SourceRevision.Algorithm, d.Deployment.SourceRevision.Digest
	context.Candidate.ImageDigest = "sha256:" + *d.Deployment.ImageSHA256
	message := perfeng.IntegrationMessage{SpecVersion: "1.0", ID: "event-1", Source: issuer, Type: "dev.perfeng.integration.requested.v1", Time: context.RequestedAt, DataContentType: "application/json", DataSchema: perfeng.IntegrationSchemaURI, Data: perfeng.IntegrationData{Context: context}}
	review := selectionio.RequestReview{Selection: selected, Reviewer: perfeng.IntegrationIdentity{Issuer: "https://owners.example", ID: "checkout-owner"}, ApprovedAt: context.RequestedAt, ExpiresAt: observed.Add(20 * time.Minute).Format(time.RFC3339Nano)}
	for _, decision := range selected.Decisions {
		review.Bindings = append(review.Bindings, selectionio.RequestBinding{MappingID: *decision.MappingID, Message: message})
	}
	authority := selectionio.RequestAuthority{ReviewSHA256: digest(encode(t, review)), RequestIssuer: issuer, Reviewer: review.Reviewer, Principal: principal, OwnerRoutes: []contracts.PerformanceReference{*d.OwnerRoute}}

	return evidence, raw, review, authority
}

func TestCompileDeduplicatesAndOwnsExactReviewedRequests(t *testing.T) {
	t.Parallel()
	evidence, policy, review, authority := requestReview(t, true)
	compiled, err := selectionio.Compile(evidence, policy, review, authority, observed.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	s := compiled.Snapshot()
	if len(s.Children) != 1 || len(s.Children[0].Reasons) != 2 || s.Children[0].Reasons[1].Capability != "second-capability" || s.Children[0].Reasons[0].Reason != "owner-approval-required" {
		t.Fatal("identical targets lost reasons or were dispatched twice")
	}
	child := s.Children[0]
	if child.SHA256 != digest(child.Payload) || !bytes.Equal(s.Policy, policy) || !bytes.Equal(s.Selection, encode(t, review.Selection)) || !bytes.Equal(s.Review, encode(t, review)) {
		t.Fatal("lost exact payload or source/approval evidence")
	}
	message, err := perfeng.DecodeIntegration(child.Payload)
	if err != nil || !reflect.DeepEqual(message, review.Bindings[0].Message) {
		t.Fatalf("invalid frozen child: %v", err)
	}
	replayed, err := selectionio.Compile(evidence, policy, review, authority, observed.Add(2*time.Minute))
	if err != nil || !reflect.DeepEqual(s, replayed.Snapshot()) {
		t.Fatalf("replay changed identities or bytes: %v", err)
	}
	policy[0], s.Children[0].Payload[0], s.Review[0] = '!', '!', '!'
	s.Children[0].Reasons[0].Reason = "mutated"
	review.Bindings[0].Message.Data.Context.Evidence[0].ID = "mutated"
	if !reflect.DeepEqual(compiled.Snapshot(), replayed.Snapshot()) {
		t.Fatal("caller mutated immutable compilation")
	}
}

func TestCompileKeepsDistinctApprovedProviderTargets(t *testing.T) {
	t.Parallel()
	evidence, policy, review, authority := requestReview(t, true)
	m := &review.Bindings[1].Message
	m.ID, m.Data.Context.Request.ID, m.Data.Context.Attempt.ID = "event-2", "request-2", "attempt-2"
	m.Data.Context.Capability.ID = "other-reviewed-provider-capability"
	authority.ReviewSHA256 = digest(encode(t, review))
	compiled, err := selectionio.Compile(evidence, policy, review, authority, observed)
	if err != nil || len(compiled.Snapshot().Children) != 2 {
		t.Fatalf("lost distinct requests: %v", err)
	}
}

func TestCompileRejectsUnapprovedOrInconsistentRequestsAtomically(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*selectionio.RequestReview, *selectionio.RequestAuthority){
		"predated-evaluation": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Selection.EvaluatedAt = observed.Add(-time.Hour).Format(time.RFC3339Nano)
		},
		"deadline-at-now": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Deadline = observed.Add(time.Minute).Format(time.RFC3339Nano)
		},
		"duplicate-target-identities": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			m := &r.Bindings[1].Message
			m.ID, m.Data.Context.Request.ID, m.Data.Context.Attempt.ID = "event-2", "request-2", "attempt-2"
		},
		"duplicate-target-metadata": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			m := &r.Bindings[1].Message
			m.ID, m.Data.Context.Request.ID, m.Data.Context.Attempt.ID = "event-2", "request-2", "attempt-2"
			m.Data.Context.Deadline = observed.Add(5 * time.Minute).Format(time.RFC3339Nano)
			m.Data.Context.Evidence = append(m.Data.Context.Evidence, m.Data.Context.Service)
		},
		"reviewer": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) { r.Reviewer.ID = "outsider" },
		"owner-route": func(_ *selectionio.RequestReview, a *selectionio.RequestAuthority) {
			a.OwnerRoutes[0].Revision = "other"
		},
		"principal": func(_ *selectionio.RequestReview, a *selectionio.RequestAuthority) { a.Principal.ID = "other" },
		"issuer": func(_ *selectionio.RequestReview, a *selectionio.RequestAuthority) {
			a.RequestIssuer = "https://other.example"
		},
		"policy-pin": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Selection.PolicySHA256 = "wrong"
		},
		"catalog-pin": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Selection.Catalog.Fingerprint = "wrong"
		},
		"source-pin": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Selection.Change.HeadRevision.Digest = "wrong"
		},
		"future-review": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.ApprovedAt = observed.Add(time.Hour).Format(time.RFC3339Nano)
		},
		"expired-review": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.ExpiresAt = observed.Format(time.RFC3339Nano)
		},
		"extended-review": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.ExpiresAt = observed.Add(time.Hour).Format(time.RFC3339Nano)
		},
		"missing-binding": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) { r.Bindings = r.Bindings[:1] },
		"unknown-binding": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].MappingID = "unknown"
		},
		"duplicate-mapping": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].MappingID = r.Bindings[0].MappingID
		},
		"conflicting-event": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.ID = "event-2"
		},
		"conflicting-target": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Capability.ID = "other"
		},
		"deployment": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Deployment.ID = "other"
		},
		"service": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Service.ID = "other"
		},
		"workload": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Workload.ID = "other"
		},
		"image": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Candidate.ImageDigest = "sha256:" + string(bytes.Repeat([]byte{'a'}, 64))
		},
		"source": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Candidate.Source.Digest = "bad"
		},
		"missing-evidence": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Evidence = nil
		},
		"missing-baseline": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Baseline = nil
		},
		"deadline": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Data.Context.Deadline = observed.Add(time.Hour).Format(time.RFC3339Nano)
		},
		"not-request": func(r *selectionio.RequestReview, _ *selectionio.RequestAuthority) {
			r.Bindings[1].Message.Type = "dev.perfeng.integration.cancelled.v1"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			evidence, policy, review, authority := requestReview(t, true)
			mutate(&review, &authority)
			authority.ReviewSHA256 = digest(encode(t, review))
			compiled, err := selectionio.Compile(evidence, policy, review, authority, observed.Add(time.Minute))
			if !errors.Is(err, performance.ErrInvalid) || len(compiled.Snapshot().Children) != 0 {
				t.Fatalf("returned partial/unapproved requests: %v", err)
			}
		})
	}
}

func TestCompileRequiresIndependentApprovalAndCurrentEvidence(t *testing.T) {
	t.Parallel()
	evidence, policy, review, authority := requestReview(t, false)
	for _, test := range []struct {
		name      string
		policy    []byte
		authority selectionio.RequestAuthority
		at        time.Time
	}{
		{"no-approval", policy, selectionio.RequestAuthority{}, observed},
		{"tampered-approval", policy, func() selectionio.RequestAuthority { a := authority; a.ReviewSHA256 = "untrusted"; return a }(), observed},
		{"changed-policy-bytes", append([]byte(" "), policy...), authority, observed},
		{"stale-evidence", policy, authority, observed.Add(time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := selectionio.Compile(evidence, test.policy, review, test.authority, test.at)
			if !errors.Is(err, performance.ErrInvalid) || len(compiled.Snapshot().Children) != 0 {
				t.Fatalf("accepted invalid authority/evidence: %v", err)
			}
		})
	}
}
