package githubreview

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestObserveOutcomeCapturesCompleteReviewerDiff(t *testing.T) {
	t.Parallel()
	publication := validReviewPublication()
	finalSHA := strings.Repeat("9", 40)
	server := httptest.NewServer(reviewOutcomeHandler(t, publication, "closed", finalSHA, true, true))
	defer server.Close()
	observedAt := time.Date(2026, 9, 28, 10, 1, 0, 0, time.UTC)
	client := newOutcomeTestClient(t, server, observedAt)

	terminal, err := client.ObserveOutcome(t.Context(), publication)
	if err != nil {
		t.Fatalf("observe outcome: %v", err)
	}
	if terminal.FinalRevision.Digest != finalSHA || terminal.MergedAt == nil ||
		terminal.ObservedAt != observedAt || len(terminal.ReviewerEdits) != 1 {
		t.Fatalf("unexpected terminal review: %+v", terminal)
	}
	edit := terminal.ReviewerEdits[0]
	if edit.Path != "tests/orders.spec.ts" || edit.Kind != adaptation.ReviewFileModified || edit.Patch == "" {
		t.Fatalf("unexpected reviewer edit: %+v", edit)
	}
}

func TestObserveOutcomeRejectsOpenReview(t *testing.T) {
	t.Parallel()
	publication := validReviewPublication()
	server := httptest.NewServer(reviewOutcomeHandler(
		t, publication, "open", publication.HeadRevision.Digest, false, false,
	))
	defer server.Close()

	_, err := newOutcomeTestClient(t, server, time.Now()).ObserveOutcome(t.Context(), publication)
	if !errors.Is(err, adaptation.ErrReviewNotFinal) {
		t.Fatalf("observe open review = %v, want not final", err)
	}
}

func TestObserveOutcomeRejectsIncompleteReviewerPatch(t *testing.T) {
	t.Parallel()
	publication := validReviewPublication()
	finalSHA := strings.Repeat("9", 40)
	server := httptest.NewServer(reviewOutcomeHandler(t, publication, "closed", finalSHA, true, false))
	defer server.Close()

	_, err := newOutcomeTestClient(t, server, time.Now()).ObserveOutcome(t.Context(), publication)
	if !errors.Is(err, adaptation.ErrReviewEvidenceIncomplete) {
		t.Fatalf("observe incomplete diff = %v, want incomplete evidence", err)
	}
}

func TestOutcomeObservationUsesOnlyReadAuthority(t *testing.T) {
	t.Parallel()
	publication := validReviewPublication()
	provider := reviewOutcomeHandler(t, publication, "closed", publication.HeadRevision.Digest, false, false)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer read-token" {
			t.Errorf("observer used unexpected authority: %s %s", request.Method, request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusForbidden)
			return
		}
		provider.ServeHTTP(response, request)
	}))
	defer server.Close()
	observer, err := NewOutcomeObserver(ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Host: "github.com", Token: "read-token",
	})
	if err != nil {
		t.Fatalf("create observer: %v", err)
	}
	terminal, err := observer.ObserveOutcome(t.Context(), publication)
	if err != nil || terminal.FinalRevision != publication.HeadRevision {
		t.Fatalf("read terminal outcome: %+v, %v", terminal, err)
	}
}

func reviewOutcomeHandler(
	t *testing.T,
	publication adaptation.ReviewPublication,
	state string,
	finalSHA string,
	merged bool,
	includePatch bool,
) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var value any
		switch {
		case request.URL.Path == "/repos/example/orders-tests":
			value = map[string]any{
				"id": 1296269, "name": "orders-tests", "owner": map[string]string{"login": "example"},
			}
		case request.URL.Path == "/repos/example/orders-tests/pulls/17":
			value = map[string]any{
				"number": 17, "html_url": publication.PullRequestURL, "state": state,
				"closed_at": "2026-09-28T10:00:00Z", "merged": merged,
				"base": map[string]string{"ref": publication.BaseBranch},
				"head": map[string]string{"ref": publication.HeadBranch, "sha": finalSHA},
			}
			if merged {
				value.(map[string]any)["merged_at"] = "2026-09-28T09:59:00Z"
			}
		case strings.Contains(request.URL.Path, "/compare/"):
			file := map[string]any{
				"filename": "tests/orders.spec.ts", "status": "modified", "additions": 1, "deletions": 1,
			}
			if includePatch {
				file["patch"] = "@@ -1 +1 @@\n-/v2/orders\n+/v3/orders"
			}
			value = map[string]any{
				"status": "ahead", "ahead_by": 1,
				"base_commit":       map[string]string{"sha": publication.HeadRevision.Digest},
				"merge_base_commit": map[string]string{"sha": publication.HeadRevision.Digest},
				"files":             []any{file},
			}
		default:
			t.Errorf("unexpected GitHub request: %s", request.URL.String())
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(value); err != nil {
			t.Errorf("write GitHub response: %v", err)
		}
	})
}

func newOutcomeTestClient(t *testing.T, server *httptest.Server, now time.Time) *OutcomeObserver {
	t.Helper()
	client, err := NewOutcomeObserver(ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Host: "github.com", Token: "token",
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("create GitHub review client: %v", err)
	}

	return client
}

func validReviewPublication() adaptation.ReviewPublication {
	return adaptation.ReviewPublication{
		APIVersion: adaptation.ReviewAPIVersion,
		ReviewID:   strings.Repeat("e", 64), ProposalID: strings.Repeat("a", 64),
		ValidationID: strings.Repeat("d", 64),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "1296269",
			}, Owner: "example", Name: "orders-tests",
		},
		Provider: string(catalog.ProviderGitHub), BaseBranch: "main",
		BaseRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40),
		},
		HeadBranch: "argus/endpoint-repair-aaaaaaaaaaaa",
		HeadRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("f", 40),
		},
		PullRequestNumber: 17,
		PullRequestURL:    "https://github.com/example/orders-tests/pull/17",
		Draft:             true, State: "open", PublishedAt: time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC),
	}
}
