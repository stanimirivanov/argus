package main

import (
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestAnalysisResourcesRefuseLinksBroadModesAndSpecialFiles(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := analysisDirectory(directory, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeAnalysisFile(root) }()
	path := filepath.Join(directory, "review.json")
	if err := os.WriteFile(path, []byte("review"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := analysisFile(root, "review.json", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	closeAnalysisFile(file)
	if err := os.Symlink(path, filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(directory, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "hard")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link", "fifo", "hard", "review.json"} {
		file, err := analysisFile(root, name, true, 10)
		if file != nil || !errors.Is(err, errAnalysisResource) {
			t.Fatal("unsafe resource opened", name, err)
		}
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := analysisDirectory(directory, true); !errors.Is(err, errAnalysisResource) {
		t.Fatal("broad root accepted", err)
	}
	if _, err := analysisFile(root, "review.json", true, 10); !errors.Is(err, errAnalysisResource) {
		t.Fatal("loosened root accepted", err)
	}
}

func TestAnalysisPreparationRequiresExplicitProtectedResources(t *testing.T) {
	disabled := performanceAnalysisConfig{}
	root, _, err := disabled.prepareAnalysis()
	if err != nil || root != nil {
		t.Fatal("disabled mode opened resources")
	}
	c, err := loadAnalysisValues(analysisEnvironment(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.prepareAnalysis(); !errors.Is(err, errAnalysisResource) {
		t.Fatal("absent review root accepted", err)
	}
	if err := os.Chmod(filepath.Dir(c.reviewDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(c.reviewDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	root, consumer, err := c.prepareAnalysis()
	if err != nil || consumer.Reports == nil {
		t.Fatal("protected startup failed", err)
	}
	closeAnalysisFile(root)
	c.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(c.caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.prepareAnalysis(); !errors.Is(err, errAnalysisResource) {
		t.Fatal("invalid CA accepted", err)
	}
}

func TestAnalysisCAFileVerifiedTLS(t *testing.T) {

	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	path := filepath.Join(directory, "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := analysisCAClient(path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("selected CA did not verify TLS", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
