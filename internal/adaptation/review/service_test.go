package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestPublishCreatesCorrelatedDraftRequest(t *testing.T) {
	t.Parallel()
	proposal, evidence, source := validReviewInputs()
	gateway := &fakeGateway{source: source, result: PublishedPullRequest{
		Number: 17, URL: "https://github.com/example/orders-tests/pull/17",
		HeadRevision: revision(strings.Repeat("d", 40)), Draft: true, State: "open",
		CreatedAt: time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC),
	}}
	publication, err := NewService(gateway, gateway).Publish(t.Context(), proposal, evidence, "main")
	if err != nil {
		t.Fatalf("publish review: %v", err)
	}
	if publication.PullRequestNumber != 17 || !publication.Draft || publication.ReviewID == "" {
		t.Fatalf("unexpected publication: %+v", publication)
	}
	if gateway.request.HeadBranch != "argus/endpoint-repair-"+proposal.ProposalID[:12] ||
		string(gateway.request.Candidate) != "await request.get('/v2/orders')" ||
		!strings.Contains(gateway.request.Body, evidence.ValidationID) {
		t.Fatalf("unexpected publication request: %+v", gateway.request)
	}
}

func TestPublishRejectsMismatchedEvidenceBeforeProviderWrite(t *testing.T) {
	t.Parallel()
	proposal, evidence, source := validReviewInputs()
	evidence.ProposalID = strings.Repeat("e", 64)
	gateway := &fakeGateway{source: source}
	_, err := NewService(gateway, gateway).Publish(t.Context(), proposal, evidence, "main")
	if !errors.Is(err, adaptation.ErrInvalid) || gateway.loaded {
		t.Fatalf("publish mismatched evidence = %v, loaded=%v", err, gateway.loaded)
	}
}

func TestPublishRejectsChangedSourceAndCandidateDigest(t *testing.T) {
	t.Parallel()
	proposal, evidence, source := validReviewInputs()
	tests := map[string]func(*fakeGateway, *adaptation.ValidationEvidence){
		"source changed": func(gateway *fakeGateway, _ *adaptation.ValidationEvidence) {
			gateway.source = []byte("await request.get('/other')")
		},
		"candidate digest changed": func(_ *fakeGateway, evidence *adaptation.ValidationEvidence) {
			evidence.Source.CandidateSHA256 = strings.Repeat("f", 64)
			evidence.Runs[1].SourceSHA256 = evidence.Source.CandidateSHA256
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidateEvidence := evidence
			candidateEvidence.Runs = append([]adaptation.ValidationRun{}, evidence.Runs...)
			gateway := &fakeGateway{source: append([]byte{}, source...)}
			mutate(gateway, &candidateEvidence)
			_, err := NewService(gateway, gateway).Publish(t.Context(), proposal, candidateEvidence, "main")
			if !errors.Is(err, adaptation.ErrInvalid) || gateway.published {
				t.Fatalf("publish invalid source = %v, published=%v", err, gateway.published)
			}
		})
	}
}

type fakeGateway struct {
	source    []byte
	result    PublishedPullRequest
	request   PublicationRequest
	loaded    bool
	published bool
}

func (gateway *fakeGateway) LoadSource(
	_ context.Context,
	_ catalog.Repository,
	_ catalog.Revision,
	_ string,
) ([]byte, error) {
	gateway.loaded = true
	return append([]byte{}, gateway.source...), nil
}

func (gateway *fakeGateway) Publish(
	_ context.Context,
	request PublicationRequest,
) (PublishedPullRequest, error) {
	gateway.published = true
	gateway.request = request
	return gateway.result, nil
}

func validReviewInputs() (adaptation.Proposal, adaptation.ValidationEvidence, []byte) {
	source := []byte("await request.get('/v1/orders')")
	start := strings.Index(string(source), "/v1/orders")
	edit := adaptation.TextEdit{
		Path: "tests/orders.spec.ts", BeforeSHA256: sha(source), StartByte: start,
		EndByte: start + len("/v1/orders"), Original: "/v1/orders", Replacement: "/v2/orders",
		SemanticRole: "request-target",
	}
	test := adaptation.TestReference{
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "1296269",
			},
			Owner: "example", Name: "orders-tests",
		},
		Revision: revision(strings.Repeat("c", 40)), SuiteKey: "orders-api", TestKey: "list-orders",
		Name: "lists orders", Adapter: "playwright", Capabilities: []string{"list-orders"},
	}
	proposal := adaptation.Proposal{
		APIVersion: adaptation.ProposalAPIVersion, PolicyVersion: adaptation.PolicyVersion,
		ProposalID: strings.Repeat("a", 64), ImpactAPIVersion: change.ImpactAPIVersion,
		ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42",
				}, Owner: "example", Name: "orders",
			},
			PullRequestNumber: 42, BaseRevision: revision(strings.Repeat("1", 40)),
			HeadRevision: revision(strings.Repeat("2", 40)),
			ObservedAt:   time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
			Trigger:      change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"},
		},
		Test: test, Classification: adaptation.ClassificationInvalidated,
		Decision: adaptation.DecisionPatchAndValidate,
		Rename: adaptation.EndpointRename{
			Method: "GET", OperationID: "listOrders", PreviousPath: "/v1/orders", Path: "/v2/orders",
			Capabilities: []string{"list-orders"},
		},
		AdapterID: "playwright", AdapterVersion: "1.2.3", Edit: edit,
	}
	candidate := []byte("await request.get('/v2/orders')")
	negative := []byte("await request.get('/__argus_negative_control__/aaaaaaaaaaaa')")
	evidence := adaptation.ValidationEvidence{
		APIVersion: adaptation.ValidationEvidenceAPIVersion, PolicyVersion: adaptation.ValidationPolicyVersion,
		ValidationID: strings.Repeat("b", 64), ProposalID: proposal.ProposalID,
		ProposalPolicyVersion: proposal.PolicyVersion, Test: test,
		AdapterID: "playwright", AdapterVersion: "1.2.3", Edit: edit,
		Source: adaptation.ValidationSourceEvidence{
			Path: edit.Path, OriginalSHA256: sha(source), CandidateSHA256: sha(candidate),
			NegativeSHA256: sha(negative), RestoredSHA256: sha(source),
			NegativeControlPath: "/__argus_negative_control__/aaaaaaaaaaaa",
		},
		Runs: []adaptation.ValidationRun{
			validationRun(adaptation.ValidationOriginal, sha(source), adaptation.ValidationFailed, 0),
			validationRun(adaptation.ValidationCandidate, sha(candidate), adaptation.ValidationPassed, 2),
			validationRun(adaptation.ValidationNegativeControl, sha(negative), adaptation.ValidationFailed, 4),
		},
	}

	return proposal, evidence, source
}

func validationRun(
	phase adaptation.ValidationPhase,
	sourceSHA string,
	outcome adaptation.ValidationOutcome,
	minute int,
) adaptation.ValidationRun {
	run := adaptation.ValidationRun{
		Phase: phase, SourceSHA256: sourceSHA,
		StartedAt:   time.Date(2026, 9, 27, 12, minute, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 27, 12, minute+1, 0, 0, time.UTC), Outcome: outcome,
	}
	if outcome == adaptation.ValidationFailed {
		run.Failure = &adaptation.ValidationFailure{Code: "assertion-failed", Message: "expected response"}
	}

	return run
}

func revision(digest string) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: digest}
}

func sha(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
