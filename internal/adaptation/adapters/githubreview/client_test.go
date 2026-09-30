package githubreview

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestPublishCreatesDraftAndExactRetryReturnsIt(t *testing.T) {
	t.Parallel()
	fixture := newGitHubFixture(t)
	server := httptest.NewServer(fixture)
	defer server.Close()
	client := newTestClient(t, server)
	request := validPublicationRequest()

	first, err := client.Publish(t.Context(), request)
	if err != nil {
		t.Fatalf("publish review: %v", err)
	}
	second, err := client.Publish(t.Context(), request)
	if err != nil {
		t.Fatalf("retry review: %v", err)
	}
	if first.Number != 17 || first != second || !first.Draft || first.State != "open" {
		t.Fatalf("unexpected publications: first=%+v second=%+v", first, second)
	}
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	if fixture.branchCreates != 1 || fixture.fileUpdates != 1 || fixture.pullCreates != 1 {
		t.Fatalf("unexpected writes: branch=%d file=%d pull=%d", fixture.branchCreates, fixture.fileUpdates, fixture.pullCreates)
	}
}

func TestPublishRejectsDivergentDeterministicBranch(t *testing.T) {
	t.Parallel()
	fixture := newGitHubFixture(t)
	fixture.branch = true
	fixture.headSHA = strings.Repeat("e", 40)
	fixture.current = []byte("changed by somebody else")
	server := httptest.NewServer(fixture)
	defer server.Close()
	client := newTestClient(t, server)

	_, err := client.Publish(t.Context(), validPublicationRequest())
	if !errors.Is(err, adaptation.ErrReviewConflict) {
		t.Fatalf("publish divergent branch = %v, want conflict", err)
	}
}

func TestLoadSourceVerifiesStableRepositoryIdentity(t *testing.T) {
	t.Parallel()
	fixture := newGitHubFixture(t)
	fixture.repositoryID = 999
	server := httptest.NewServer(fixture)
	defer server.Close()
	client := newTestClient(t, server)
	request := validPublicationRequest()

	_, err := client.LoadSource(t.Context(), request.Repository, request.BaseRevision, request.Path)
	if !errors.Is(err, adaptation.ErrReviewConflict) {
		t.Fatalf("load transferred repository = %v, want conflict", err)
	}
}

type githubFixture struct {
	testing       *testing.T
	mutex         sync.Mutex
	repositoryID  int
	branch        bool
	pull          bool
	headSHA       string
	current       []byte
	branchCreates int
	fileUpdates   int
	pullCreates   int
}

func newGitHubFixture(t *testing.T) *githubFixture {
	return &githubFixture{
		testing: t, repositoryID: 1296269, headSHA: strings.Repeat("c", 40),
		current: []byte("await request.get('/v1/orders')"),
	}
}

func (fixture *githubFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) { //nolint:gocognit // Stateful provider fixture covers one complete publication protocol.
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	if request.Header.Get("Authorization") != "Bearer token" || request.Header.Get("X-GitHub-Api-Version") == "" {
		fixture.testing.Error("missing GitHub authentication or API version")
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/repos/example/orders-tests":
		fixture.writeJSON(response, map[string]any{
			"id": fixture.repositoryID, "name": "orders-tests", "owner": map[string]string{"login": "example"},
		})
	case request.Method == http.MethodGet && request.URL.Path == "/repos/example/orders-tests/pulls":
		if !fixture.pull {
			fixture.writeJSON(response, []any{})
			return
		}
		fixture.writeJSON(response, []any{fixture.pullResponse()})
	case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/git/ref/heads/"):
		if !fixture.branch {
			http.NotFound(response, request)
			return
		}
		fixture.writeJSON(response, map[string]any{"object": map[string]string{"sha": fixture.headSHA}})
	case request.Method == http.MethodPost && request.URL.Path == "/repos/example/orders-tests/git/refs":
		fixture.branch = true
		fixture.branchCreates++
		fixture.writeJSON(response, map[string]any{"object": map[string]string{"sha": fixture.headSHA}})
	case request.Method == http.MethodGet && request.URL.Path == "/repos/example/orders-tests/contents/tests/orders.spec.ts":
		if request.Header.Get("Accept") == "application/vnd.github.raw+json" {
			if _, err := response.Write(fixture.current); err != nil {
				fixture.testing.Errorf("write raw source: %v", err)
			}
			return
		}
		fixture.writeJSON(response, map[string]any{
			"type": "file", "sha": "blob-before", "size": len(fixture.current),
		})
	case request.Method == http.MethodPut && request.URL.Path == "/repos/example/orders-tests/contents/tests/orders.spec.ts":
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			fixture.testing.Errorf("decode update: %v", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(payload.Content)
		if err != nil {
			fixture.testing.Errorf("decode content: %v", err)
		}
		fixture.current = decoded
		fixture.headSHA = strings.Repeat("d", 40)
		fixture.fileUpdates++
		fixture.writeJSON(response, map[string]any{"commit": map[string]string{"sha": fixture.headSHA}})
	case request.Method == http.MethodPost && request.URL.Path == "/repos/example/orders-tests/pulls":
		var payload struct {
			Draft bool `json:"draft"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || !payload.Draft {
			fixture.testing.Errorf("pull request was not draft: err=%v payload=%+v", err, payload)
		}
		fixture.pull = true
		fixture.pullCreates++
		fixture.writeJSON(response, fixture.pullResponse())
	default:
		fixture.testing.Errorf("unexpected GitHub request: %s %s", request.Method, request.URL.String())
		http.NotFound(response, request)
	}
}

func (fixture *githubFixture) pullResponse() map[string]any {
	return map[string]any{
		"number": 17, "html_url": "https://github.com/example/orders-tests/pull/17",
		"state": "open", "draft": true, "created_at": "2026-09-27T14:00:00Z",
		"base": map[string]string{"ref": "main"},
		"head": map[string]string{"ref": "argus/endpoint-repair-aaaaaaaaaaaa", "sha": fixture.headSHA},
	}
}

func (fixture *githubFixture) writeJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		fixture.testing.Errorf("write JSON: %v", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Host: "github.com", Token: "token",
	})
	if err != nil {
		t.Fatalf("create GitHub review client: %v", err)
	}

	return client
}

func TestClientRejectsGitHubRedirectWithoutForwardingToken(t *testing.T) {
	t.Parallel()
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		targetRequests.Add(1)
		response.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	client := newTestClient(t, origin)
	publication := validPublicationRequest()
	_, err := client.LoadSource(t.Context(), publication.Repository, publication.BaseRevision, publication.Path)
	if !errors.Is(err, adaptation.ErrUnavailable) || targetRequests.Load() != 0 {
		t.Fatalf("LoadSource() = %v; redirected requests = %d", err, targetRequests.Load())
	}
}

func validPublicationRequest() review.PublicationRequest {
	candidate := []byte("await request.get('/v2/orders')")

	return review.PublicationRequest{
		ReviewID: strings.Repeat("a", 64),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "1296269",
			}, Owner: "example", Name: "orders-tests",
		},
		BaseRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40)},
		BaseBranch:   "main", HeadBranch: "argus/endpoint-repair-aaaaaaaaaaaa",
		Path: "tests/orders.spec.ts", OriginalSHA256: digest([]byte("await request.get('/v1/orders')")),
		Candidate: candidate, Title: "Repair orders test", Body: "Validated by Argus", CommitMessage: "Repair orders endpoint",
	}
}
