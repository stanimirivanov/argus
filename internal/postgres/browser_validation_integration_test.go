//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

type browserReviewGateway struct {
	sources  map[string][]byte
	request  review.PublicationRequest
	terminal outcome.TerminalReview
	writes   int
}

func (gateway *browserReviewGateway) LoadSource(_ context.Context, _ catalog.Repository, revision catalog.Revision, _ string) ([]byte, error) {
	return append([]byte{}, gateway.sources[revision.Digest]...), nil
}
func (gateway *browserReviewGateway) Publish(_ context.Context, request review.PublicationRequest) (review.PublishedPullRequest, error) {
	gateway.request = request
	gateway.writes++
	return review.PublishedPullRequest{Number: 17, URL: "https://github.com/example/browser-tests/pull/17",
		HeadRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("f", 40)},
		Draft:        true, State: "open", CreatedAt: time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC)}, nil
}
func (gateway *browserReviewGateway) ObserveOutcome(_ context.Context, _ adaptation.ReviewPublication) (outcome.TerminalReview, error) {
	return gateway.terminal, nil
}

func TestBrowserReviewPublishesValidatedCandidateAndCapturesBoundedCorrection(t *testing.T) {
	evidence := browserValidationFixture(t)
	document, err := adaptationcontract.ExportBrowserLocatorValidationV1(evidence, executioncontract.ExportBrowserAttempt)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err = adaptationcontract.ImportBrowserLocatorValidationV1(document, executioncontract.ImportBrowserAttempt)
	if err != nil {
		t.Fatal(err)
	}
	proposal := evidence.Proposal
	source := []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`)
	baseSHA := evidence.Failed.Request.TestRevision.Digest
	generatedSHA := strings.Repeat("f", 40)
	finalSHA := strings.Repeat("1", 40)
	gateway := &browserReviewGateway{sources: map[string][]byte{baseSHA: source}}
	stale := &browserReviewGateway{sources: map[string][]byte{baseSHA: []byte("changed test source")}}
	if _, err := review.NewService(stale, stale).PublishBrowser(t.Context(), evidence, "main"); !errors.Is(err, adaptation.ErrInvalid) || stale.writes != 0 {
		t.Fatalf("stale source published: %v writes=%d", err, stale.writes)
	}
	publication, err := review.NewService(gateway, gateway).PublishBrowser(t.Context(), evidence, "main")
	if err != nil {
		t.Fatal(err)
	}
	if gateway.writes != 1 || !publication.Draft || digestBrowserFixture(gateway.request.Candidate) != evidence.Source.CandidateSHA256 ||
		strings.Contains(gateway.request.Body, "file:///tmp/") || strings.Contains(gateway.request.Body, "Place order") {
		t.Fatalf("unsafe browser publication: %+v", gateway.request)
	}
	gateway.sources[generatedSHA] = gateway.request.Candidate
	final := strings.Replace(string(gateway.request.Candidate), proposal.Edit.Replacement,
		`getByRole("button", { name: "Confirm order" })`, 1)
	gateway.sources[finalSHA] = []byte(final)
	merged := time.Date(2026, 10, 19, 13, 20, 0, 0, time.UTC)
	gateway.terminal = outcome.TerminalReview{FinalRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: finalSHA},
		ClosedAt: merged, MergedAt: &merged, ObservedAt: merged.Add(time.Minute), ReviewerEdits: []adaptation.ReviewFileEdit{{
			Path: proposal.Edit.Path, Kind: adaptation.ReviewFileModified, Additions: 1, Deletions: 1,
			Patch: "@@ -1 +1 @@\n-old locator\n+corrected locator"}}}
	result, err := outcome.NewService(gateway).CaptureBrowser(t.Context(), evidence, publication, gateway, adaptation.ReviewReasonCorrected, "")
	if err != nil || result.Decision != adaptation.ReviewAcceptedWithEdits {
		t.Fatalf("capture corrected browser review: %v %+v", err, result)
	}
	outcomeDocument, err := adaptationcontract.ExportReviewOutcomeV1(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adaptationcontract.ImportReviewOutcomeV1(outcomeDocument); err != nil {
		t.Fatal(err)
	}
	gateway.sources[finalSHA] = []byte(strings.Replace(final, "toBeVisible()", "toBeHidden()", 1))
	if _, err := outcome.NewService(gateway).CaptureBrowser(t.Context(), evidence, publication, gateway, adaptation.ReviewReasonCorrected, ""); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("oracle-changing review accepted: %v", err)
	}
	rejected := evidence
	rejected.Status, rejected.Reason = browservalidation.Rejected, "original-passed"
	rejected.Runs = append([]browservalidation.Run{}, rejected.Runs[:2]...)
	for i := range rejected.Runs {
		rejected.Runs[i].Attempt.Result.Results = append([]execution.TestResult{}, rejected.Runs[i].Attempt.Result.Results...)
		rejected.Runs[i].Attempt.Result.Results[0].Outcome = execution.TestPassed
		rejected.Runs[i].Attempt.Result.Results[0].Failure = nil
		rejected.Runs[i].Attempt.Outcome = execution.DeriveAttemptOutcome(rejected.Runs[i].Attempt.Result.Results)
	}
	if err := browservalidation.ValidateEvidence(rejected); err != nil {
		t.Fatalf("trustworthy browser rejection fixture: %v", err)
	}
	if _, err := review.NewService(gateway, gateway).PublishBrowser(t.Context(), rejected, "main"); !errors.Is(err, adaptation.ErrInvalid) || gateway.writes != 1 {
		t.Fatalf("rejected proof published: %v writes=%d", err, gateway.writes)
	}
}

func TestBrowserValidationRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	evidence := browserValidationFixture(t)
	if _, err := store.FindBrowserValidation(t.Context(), evidence.ValidationID); !errors.Is(err, browservalidation.ErrNotFound) {
		t.Fatalf("find missing validation: %v", err)
	}
	created, err := store.SaveBrowserValidation(t.Context(), evidence)
	if err != nil || !created {
		t.Fatalf("save validation: created=%t err=%v", created, err)
	}
	created, err = store.SaveBrowserValidation(t.Context(), evidence)
	if err != nil || created {
		t.Fatalf("exact retry: created=%t err=%v", created, err)
	}
	got, err := store.FindBrowserValidation(t.Context(), evidence.ValidationID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, canonicalBrowserValidation(evidence)) {
		t.Fatal("browser validation round trip changed evidence")
	}
	conflict := evidence
	conflict.Runs = append([]browservalidation.Run{}, evidence.Runs...)
	conflict.Runs[0].Attempt = browser.CanonicalAttempt(conflict.Runs[0].Attempt)
	conflict.Runs[0].Attempt.Result.Artifacts[0].URI = "file:///tmp/changed-trace.zip"
	if err := browservalidation.ValidateEvidence(conflict); err != nil {
		t.Fatalf("invalid conflict fixture: %v", err)
	}
	if _, err := store.SaveBrowserValidation(t.Context(), conflict); !errors.Is(err, browservalidation.ErrConflict) {
		t.Fatalf("conflicting validation retry: %v", err)
	}
	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindBrowserValidation(t.Context(), evidence.ValidationID); err != nil {
		t.Fatalf("find after restart: %v", err)
	}
}

func browserValidationFixture(t *testing.T) browservalidation.Evidence {
	t.Helper()
	source := []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`)
	old := `getByRole("button", { name: "Checkout" })`
	start := strings.Index(string(source), old)
	proposal := browserlocator.Proposal{APIVersion: browserlocator.ProposalVersion, PolicyVersion: browserlocator.PolicyVersion,
		BaselineID: "baseline-1", FailedID: "failed-1", BaselineSHA256: strings.Repeat("a", 64), FailedSHA256: strings.Repeat("b", 64),
		SuiteKey: "shop", TestKey: "checkout", OriginalTarget: browserlocator.Node{Role: "button", Name: "Checkout", TestID: "action", Visible: true},
		CurrentTarget: browserlocator.Node{Role: "button", Name: "Place order", TestID: "action", Visible: true},
		Edit: adaptation.TextEdit{Path: "tests/shop.spec.mjs", BeforeSHA256: digestBrowserFixture(source), StartByte: start, EndByte: start + len(old),
			Original: old, Replacement: `getByRole("button", { name: "Place order" })`, SemanticRole: "browser-locator"}}
	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	proposal.ProposalID = digestBrowserFixture(data)
	control, err := browserlocator.NegativeControlLocator(proposal)
	if err != nil {
		t.Fatal(err)
	}
	candidate := append(append(append([]byte{}, source[:start]...), proposal.Edit.Replacement...), source[start+len(old):]...)
	negative := append(append(append([]byte{}, source[:start]...), control...), source[start+len(old):]...)
	sourceProof := browservalidation.Source{Path: proposal.Edit.Path, OriginalSHA256: digestBrowserFixture(source),
		CandidateSHA256: digestBrowserFixture(candidate), NegativeSHA256: digestBrowserFixture(negative),
		RestoredSHA256: digestBrowserFixture(source), NegativeLocator: control}
	id := browservalidation.DeriveID(proposal.ProposalID, proposal.FailedID, sourceProof.CandidateSHA256, sourceProof.NegativeSHA256)
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "baseline-1",
		Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("d", 64)}, Stage: execution.StageFullSuite,
		TestRepository: repo, TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("e", 40)},
		Adapter: "playwright", Project: "chromium-desktop", Environment: "staging",
		Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout [argus:shop:checkout]"}}}
	baseline := browserFixtureAttempt(request, execution.TestPassed, "", proposal.BaselineSHA256, 0)
	request.AttemptID = "failed-1"
	failed := browserFixtureAttempt(request, execution.TestFailed, "locator-not-found", proposal.FailedSHA256, 1)
	evidence := browservalidation.Evidence{APIVersion: browservalidation.EvidenceVersion, PolicyVersion: browservalidation.PolicyVersion,
		ValidationID: id, Status: browservalidation.Validated, Proposal: proposal, Baseline: baseline, Failed: failed, Source: sourceProof}
	for i := 0; i < 6; i++ {
		phase := []browservalidation.Phase{browservalidation.Original, browservalidation.Candidate, browservalidation.NegativeControl}[i/2]
		sha := []string{sourceProof.OriginalSHA256, sourceProof.CandidateSHA256, sourceProof.NegativeSHA256}[i/2]
		outcome := execution.TestFailed
		code := "locator-not-found"
		if phase == browservalidation.Candidate {
			outcome, code = execution.TestPassed, ""
		}
		request.AttemptID = fmt.Sprintf("browser-validation-%s-%s-%d", id[:16], phase, i%2+1)
		evidence.Runs = append(evidence.Runs, browservalidation.Run{Phase: phase, Repeat: i%2 + 1, SourceSHA256: sha,
			Attempt: browserFixtureAttempt(request, outcome, code, strings.Repeat("c", 64), i+2)})
	}
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	return evidence
}

func browserFixtureAttempt(request browser.Request, outcome execution.TestOutcome, code, snapshotSHA string, minute int) browser.Attempt {
	start := time.Date(2026, 10, 19, 12, minute, 0, 0, time.UTC)
	var failure *execution.Failure
	if code != "" {
		failure = &execution.Failure{Code: code, Message: "locator missing"}
	}
	artifacts := []browser.Artifact{}
	for _, kind := range []string{"trace", "screenshot", "accessibility"} {
		sha := strings.Repeat("c", 64)
		if kind == "accessibility" {
			sha = snapshotSHA
		}
		artifacts = append(artifacts, browser.Artifact{Key: kind, SuiteKey: "shop", TestKey: "checkout", Kind: kind,
			URI: "file:///tmp/" + kind, SHA256: sha})
	}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: "playwright", AdapterVersion: "1.58.2",
		Project: request.Project, Environment: request.Environment, StartedAt: start, CompletedAt: start.Add(time.Second),
		Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: outcome, Duration: time.Second, Failure: failure}}, Artifacts: artifacts}
	return browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}
}

func digestBrowserFixture(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
