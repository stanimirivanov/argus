package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution/browser"
)

type artifactVerifier struct{ calls int }

func (verifier *artifactVerifier) Execute(context.Context, browser.Request) (browser.Result, error) {
	return browser.Result{}, nil
}
func (verifier *artifactVerifier) VerifyArtifacts(context.Context, browser.Result) error {
	verifier.calls++
	return nil
}

func TestScopedAdapterRejectsArtifactOutsideTrustedRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "trace.zip")
	if err := os.WriteFile(outside, []byte("trace"), 0600); err != nil {
		t.Fatal(err)
	}
	verifier := &artifactVerifier{}
	adapter := &scopedAdapter{Adapter: verifier, root: root}
	result := browser.Result{Artifacts: []browser.Artifact{{URI: fixtureURI(outside)}}}
	if err := adapter.VerifyArtifacts(t.Context(), result); err == nil || verifier.calls != 0 {
		t.Fatalf("accepted artifact outside root: err=%v verifier calls=%d", err, verifier.calls)
	}
	inside := filepath.Join(root, "trace.zip")
	data := []byte("trace")
	if err := os.WriteFile(inside, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	result.Artifacts[0].URI = fixtureURI(inside)
	result.Artifacts[0].SHA256 = hex.EncodeToString(sum[:])
	if err := adapter.VerifyArtifacts(t.Context(), result); err != nil || verifier.calls != 1 {
		t.Fatalf("rejected inside artifact: err=%v verifier calls=%d", err, verifier.calls)
	}
}

func fixtureURI(path string) string {
	value := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		value = "/" + value
	}

	return (&url.URL{Scheme: "file", Path: value}).String()
}
