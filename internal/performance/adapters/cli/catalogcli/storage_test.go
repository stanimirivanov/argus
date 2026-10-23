package catalogcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

func TestPublishWaitsForFetchAndDurableReadback(t *testing.T) {
	t.Parallel()
	args := []string{"-publish", "-endpoint", "https://provider.example/v1/performance-catalog", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal", "-producer-version", "1.0.0"}
	for _, failure := range []string{"", "fetch", "publish"} {
		var output bytes.Buffer
		fetched, published := false, false
		fetch := func(context.Context, string, string, perfeng.Expectations) ([]json.RawMessage, error) {
			fetched = true
			if failure == "fetch" {
				return nil, errors.New("fetch failed")
			}

			return []json.RawMessage{[]byte(`{"complete":true}`)}, nil
		}
		storage := Storage{Publish: func(_ context.Context, pages []json.RawMessage, expected perfeng.Expectations) (Publication, error) {
			published = true
			if !fetched || output.Len() != 0 || expected.PrincipalID != "principal" || len(pages) != 1 {
				t.Fatal("publication started before complete fetch or lost scope")
			}
			if failure == "publish" {
				return Publication{}, errors.New("publication failed")
			}

			return Publication{Created: true, Pages: pages}, nil
		}}
		err := RunWithStorage(t.Context(), args, "token", &output, fetch, storage)
		if failure != "" {
			if err == nil || output.Len() != 0 || (failure == "fetch" && published) {
				t.Fatalf("exposed failed publication: %v", err)
			}
		} else if err != nil || output.String() != "{\"created\":true,\"pages\":[{\"complete\":true}]}\n" {
			t.Fatalf("publication output: %s, %v", output.String(), err)
		}
	}
}

func TestStoredInspectionNeedsNoProviderCredentialOrNetwork(t *testing.T) {
	t.Parallel()
	args := []string{"-stored", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal"}
	var output bytes.Buffer
	storage := Storage{Current: func(_ context.Context, scope catalogsync.Scope) ([]json.RawMessage, error) {
		if scope != (catalogsync.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "principal"}) {
			t.Fatal("lost stored inspection scope")
		}
		return []json.RawMessage{[]byte(`{"historical":true}`)}, nil
	}}
	fetch := func(context.Context, string, string, perfeng.Expectations) ([]json.RawMessage, error) {
		t.Fatal("stored inspection contacted provider")
		return nil, nil
	}
	if err := RunWithStorage(t.Context(), args, "", &output, fetch, storage); err != nil || output.String() != "[{\"historical\":true}]\n" {
		t.Fatalf("stored inspection: %s, %v", output.String(), err)
	}
	output.Reset()
	if err := RunWithStorage(t.Context(), append(args, "-publish"), "", &output, fetch, storage); !errors.Is(err, commandline.ErrUsage) || output.Len() != 0 {
		t.Fatalf("accepted conflicting modes: %v", err)
	}
}
