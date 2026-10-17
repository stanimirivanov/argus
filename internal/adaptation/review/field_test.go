package review

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/change"
)

func validFieldReviewInputs() (fieldrepair.Proposal, fieldvalidation.Evidence, []byte) {
	endpoint, _, _ := validReviewInputs()
	source := []byte("expect(json.orderId).toBe('order-1')")
	start := strings.Index(string(source), "orderId")
	edit := adaptation.TextEdit{Path: "tests/orders.spec.ts", BeforeSHA256: sha(source), StartByte: start,
		EndByte: start + len("orderId"), Original: "orderId", Replacement: "purchaseId", SemanticRole: "response-field-access"}
	proposal := fieldrepair.Proposal{APIVersion: fieldrepair.ProposalAPIVersion, PolicyVersion: fieldrepair.PolicyVersion,
		ProposalID: endpoint.ProposalID, ImpactAPIVersion: endpoint.ImpactAPIVersion, ImpactAnalyzerVersion: endpoint.ImpactAnalyzerVersion,
		Change: endpoint.Change, Test: endpoint.Test, Classification: "INVALIDATED", Decision: "PATCH_AND_VALIDATE",
		Rename: change.FieldRename{DocumentPath: "api/orders.yaml", BaseSHA256: strings.Repeat("1", 64), HeadSHA256: strings.Repeat("2", 64),
			Method: "GET", Path: "/orders", OperationID: "listOrders", Capabilities: []string{"list-orders"}, Direction: "response",
			StatusCode: "200", PreviousName: "orderId", Name: "purchaseId"},
		AdapterID: endpoint.AdapterID, AdapterVersion: endpoint.AdapterVersion, Edit: edit}
	candidate := []byte("expect(json.purchaseId).toBe('order-1')")
	negative := []byte("expect(json.__argus_negative_field_aaaaaaaaaaaa).toBe('order-1')")
	control := "__argus_negative_field_aaaaaaaaaaaa"
	sourceProof := fieldvalidation.Source{Path: edit.Path, OriginalSHA256: sha(source), CandidateSHA256: sha(candidate),
		NegativeSHA256: sha(negative), RestoredSHA256: sha(source), NegativeControlField: control}
	evidence := fieldvalidation.Evidence{APIVersion: fieldvalidation.EvidenceVersion, PolicyVersion: fieldvalidation.PolicyVersion,
		ValidationID: fieldvalidation.DeriveID(proposal.ProposalID, sourceProof.CandidateSHA256, sourceProof.NegativeSHA256, control),
		ProposalID:   proposal.ProposalID, ProposalPolicyVersion: proposal.PolicyVersion, Test: proposal.Test,
		AdapterID: proposal.AdapterID, AdapterVersion: proposal.AdapterVersion, Edit: edit, Source: sourceProof}
	for group, phase := range []fieldvalidation.Phase{fieldvalidation.Original, fieldvalidation.Candidate, fieldvalidation.NegativeControl} {
		outcome := adaptation.ValidationFailed
		hash := []string{sha(source), sha(candidate), sha(negative)}[group]
		if phase == fieldvalidation.Candidate {
			outcome = adaptation.ValidationPassed
		}
		for repeat := 1; repeat <= 2; repeat++ {
			minute := group*2 + repeat
			run := fieldvalidation.Run{Phase: phase, Repeat: repeat, SourceSHA256: hash,
				StartedAt:   time.Date(2026, 10, 17, 12, minute, 0, 0, time.UTC),
				CompletedAt: time.Date(2026, 10, 17, 12, minute, 30, 0, time.UTC), Outcome: outcome}
			if outcome == adaptation.ValidationFailed {
				run.Failure = &adaptation.ValidationFailure{Code: "assertion-failed", Message: "field mismatch"}
			}
			evidence.Runs = append(evidence.Runs, run)
		}
	}

	return proposal, evidence, source
}

func TestPublishFieldRequiresRepeatedProofAndCreatesDraft(t *testing.T) {
	proposal, evidence, source := validFieldReviewInputs()
	gateway := &fakeGateway{source: source, result: PublishedPullRequest{Number: 17,
		URL: "https://github.com/example/orders-tests/pull/17", HeadRevision: revision(strings.Repeat("d", 40)),
		Draft: true, State: "open", CreatedAt: time.Date(2026, 10, 17, 13, 0, 0, 0, time.UTC)}}
	publication, err := NewService(gateway, gateway).PublishField(t.Context(), proposal, evidence, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !publication.Draft || gateway.request.HeadBranch != "argus/field-repair-aaaaaaaaaaaa" ||
		string(gateway.request.Candidate) != "expect(json.purchaseId).toBe('order-1')" ||
		!strings.Contains(gateway.request.Body, "Repeated validation gates") {
		t.Fatalf("unsafe field draft: %+v", gateway.request)
	}
	evidence.Runs = evidence.Runs[:5]
	gateway.loaded, gateway.published = false, false
	_, err = NewService(gateway, gateway).PublishField(t.Context(), proposal, evidence, "main")
	if !errors.Is(err, adaptation.ErrInvalid) || gateway.loaded || gateway.published {
		t.Fatalf("partial proof reached provider: %v", err)
	}
}
