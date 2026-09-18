package github_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestClientResolvesBoundedFilesAtDeliveryRevisions(t *testing.T) {
	t.Parallel()

	var pullReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" ||
			request.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("GitHub authentication or version header missing")
		}
		switch {
		case request.URL.Path == "/repos/octocat/hello-world/pulls/42":
			pullReads.Add(1)
			writeTestResponse(t, response, pullResponse(2, "89abcdef0123456789abcdef0123456789abcdef"))
		case request.URL.Path == "/repos/octocat/hello-world/pulls/42/files":
			if request.URL.Query().Get("per_page") != "100" || request.URL.Query().Get("page") != "1" {
				t.Errorf("unexpected pagination: %s", request.URL.RawQuery)
			}
			writeTestResponse(t, response, `[
  {"filename":"api/openapi.yaml","status":"modified","additions":2,"deletions":1,"patch":"@@ patch"},
  {"filename":"assets/logo.png","status":"modified","additions":0,"deletions":0}
]`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := githubadapter.NewClient(githubadapter.ClientOptions{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		Token:      "token",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	set, err := client.Resolve(t.Context(), validGitHubDelivery())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pullReads.Load() != 2 || len(set.Files) != 2 || set.FilesTruncated {
		t.Fatalf("unexpected resolved set: %#v", set)
	}
	if set.Files[1].PatchStatus != change.PatchUnavailable {
		t.Fatalf("binary patch status = %q", set.Files[1].PatchStatus)
	}
}

func TestClientLoadsRawDocumentAtImmutableRevision(t *testing.T) {
	t.Parallel()
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/repos/octocat/hello-world/contents/api/order%20spec.yaml" ||
			request.URL.Query().Get("ref") != digest {
			t.Errorf("unexpected contents request: %s", request.URL.String())
		}
		if request.Header.Get("Accept") != "application/vnd.github.raw+json" {
			t.Errorf("Accept = %q", request.Header.Get("Accept"))
		}
		if _, err := io.WriteString(response, "openapi: 3.1.0\n"); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	client, err := githubadapter.NewClient(githubadapter.ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Token: "token",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	repository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "1",
		},
		Owner: "octocat", Name: "hello-world",
	}
	document, err := client.LoadDocument(t.Context(), repository, catalog.Revision{
		Algorithm: catalog.RevisionGitSHA1, Digest: digest,
	}, "api/order spec.yaml")
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if string(document) != "openapi: 3.1.0\n" {
		t.Fatalf("document = %q", document)
	}
}

func TestClientRejectsHeadMovementDuringPagination(t *testing.T) {
	t.Parallel()

	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/files"):
			writeTestResponse(t, response, `[{"filename":"api/openapi.yaml","status":"modified","additions":1,"deletions":1,"patch":"x"}]`)
		default:
			head := "89abcdef0123456789abcdef0123456789abcdef"
			if reads.Add(1) == 2 {
				head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}
			writeTestResponse(t, response, pullResponse(1, head))
		}
	}))
	defer server.Close()

	client, err := githubadapter.NewClient(githubadapter.ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Token: "token",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := client.Resolve(t.Context(), validGitHubDelivery()); !errors.Is(err, change.ErrStale) {
		t.Fatalf("resolve moving PR = %v, want ErrStale", err)
	}
}

func TestClientRejectsFileCountMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/files") {
			writeTestResponse(t, response, `[
  {"filename":"one.go","status":"modified","additions":1,"deletions":0,"patch":"x"},
  {"filename":"two.go","status":"modified","additions":1,"deletions":0,"patch":"x"}
]`)

			return
		}
		writeTestResponse(t, response, pullResponse(1, "89abcdef0123456789abcdef0123456789abcdef"))
	}))
	defer server.Close()

	client, err := githubadapter.NewClient(githubadapter.ClientOptions{
		HTTPClient: server.Client(), BaseURL: server.URL, Token: "token",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := client.Resolve(t.Context(), validGitHubDelivery()); !errors.Is(err, change.ErrUnavailable) {
		t.Fatalf("resolve mismatched file count = %v, want ErrUnavailable", err)
	}
}

func writeTestResponse(t *testing.T, response http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(response, body); err != nil {
		t.Errorf("write test response: %v", err)
	}
}

func validGitHubDelivery() ingest.Delivery {
	return ingest.Delivery{
		Provider:      catalog.ProviderGitHub,
		ID:            "delivery-42",
		Event:         "pull_request",
		Action:        "synchronize",
		PayloadSHA256: "86ba79cae14d4d9e69fddd7751aeb4145b34a7a2e795f79e98e5bda55a5c9df1",
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "1296269",
			},
			Owner: "octocat",
			Name:  "hello-world",
		},
		PullRequestNumber: 42,
		BaseRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		HeadRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "89abcdef0123456789abcdef0123456789abcdef",
		},
		ObservedAt: time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC),
	}
}

func pullResponse(changedFiles int, head string) string {
	return fmt.Sprintf(`{
  "number": 42,
  "changed_files": %d,
  "base": {"sha":"0123456789abcdef0123456789abcdef01234567","repo":{"id":1296269}},
  "head": {"sha":%q}
}`, changedFiles, head)
}
