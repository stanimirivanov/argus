package github_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestWebhookDecoderVerifiesBeforeNormalizing(t *testing.T) {
	t.Parallel()

	secret := []byte("It's a Secret to Everybody")
	body := webhookBody()
	decoder, err := githubadapter.NewWebhookDecoder(secret, "GitHub.COM")
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}

	delivery, err := decoder.Decode("pull_request", "delivery-42", sign(secret, body), body)
	if err != nil {
		t.Fatalf("decode webhook: %v", err)
	}
	if delivery.Repository.Identity.Host != "github.com" ||
		delivery.Repository.Identity.ProviderRepositoryID != "1296269" ||
		delivery.PullRequestNumber != 42 || delivery.Action != "synchronize" {
		t.Fatalf("unexpected delivery: %#v", delivery)
	}
}

func TestWebhookDecoderRejectsInvalidSignatureBeforeInvalidJSON(t *testing.T) {
	t.Parallel()

	decoder, err := githubadapter.NewWebhookDecoder([]byte("secret"), "github.com")
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	if _, err := decoder.Decode("pull_request", "delivery-1", "sha256=00", []byte("not-json")); !errors.Is(err, ingest.ErrUnauthenticated) {
		t.Fatalf("decode = %v, want ErrUnauthenticated", err)
	}
}

func TestWebhookSignatureMatchesGitHubDocumentedVector(t *testing.T) {
	t.Parallel()

	secret := []byte("It's a Secret to Everybody")
	body := []byte("Hello, World!")
	want := "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if got := sign(secret, body); got != want {
		t.Fatalf("signature = %q, want documented vector", got)
	}
}

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookBody() []byte {
	return []byte(`{
  "action": "synchronize",
  "number": 42,
  "repository": {"id": 1296269, "full_name": "octocat/hello-world"},
  "pull_request": {
    "updated_at": "2026-09-18T09:30:00Z",
    "base": {"sha": "0123456789abcdef0123456789abcdef01234567"},
    "head": {"sha": "89abcdef0123456789abcdef0123456789abcdef"}
  }
}`)
}
