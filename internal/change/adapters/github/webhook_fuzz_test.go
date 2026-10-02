package github_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

// FuzzWebhookDecode exercises authentication before interpreting untrusted JSON.
func FuzzWebhookDecode(f *testing.F) {
	f.Add(webhookBody())
	f.Add([]byte(`{"action":"synchronize"}`))
	f.Add([]byte("not-json"))

	secret := []byte("fuzz-webhook-secret")
	decoder, err := githubadapter.NewWebhookDecoder(secret, "github.com")
	if err != nil {
		f.Fatalf("create decoder: %v", err)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 64<<10 {
			t.Skip()
		}
		if _, err := decoder.Decode("pull_request", "delivery-1", sign(secret, body)+"0", body); !errors.Is(err, ingest.ErrUnauthenticated) {
			t.Fatalf("invalid signature was not rejected before payload: %v", err)
		}
		delivery, err := decoder.Decode("pull_request", "delivery-1", sign(secret, body), body)
		if err != nil {
			if !errors.Is(err, ingest.ErrInvalidDelivery) {
				t.Fatalf("authenticated payload returned unexpected error: %v", err)
			}
			return
		}
		digest := sha256.Sum256(body)
		if delivery.PayloadSHA256 != hex.EncodeToString(digest[:]) || delivery.Repository.Identity.Host != "github.com" {
			t.Fatalf("delivery lost authenticated bytes or configured host: %#v", delivery)
		}
	})
}
